package schema

import (
	"cmp"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/lestrrat-go/helium"
	"github.com/lestrrat-go/helium/schematron"
)

// Options controls the checks that depend on where the document is validated.
type Options struct {
	// HasAsset reports whether the normalized local asset path exists.
	// When nil, local asset existence is not checked.
	HasAsset func(normalized string) bool
	// StoredAssets marks input whose figure/audio src values were already rewritten to stored asset
	// URLs (as in the XML kept in storage). Local asset paths are then neither normalized nor
	// checked, and Document.Assets/AssetRefs stay empty.
	StoredAssets bool
}

// protocolSource is the declarative rule set; see the comment at its top for the message format.
//
//go:embed protocol.sch
var protocolSource []byte

var protocol = sync.OnceValue(func() *schematron.Schema {
	ctx := context.Background()
	doc, err := helium.NewParser().Parse(ctx, protocolSource)
	if err != nil {
		panic("schema: protocol.sch: " + err.Error())
	}
	compiled, err := schematron.NewCompiler().Compile(ctx, doc)
	if err != nil {
		panic("schema: protocol.sch: " + err.Error())
	}
	return compiled
})

const (
	assetXPath = "//figure[@src] | //audio[@src] | //attachment[@src]"
	urlXPath   = "/document/meta/canonical-url | /document/body//paper-card[@url or @code-url] | /document/body//*[self::repo-card or self::source][@url] | /document/body//*[self::ref or self::quote or self::news-item][@url]"
)

var (
	imageExtensions = []string{".png", ".jpg", ".jpeg", ".webp", ".svg"}
	audioExtensions = []string{".mp3", ".wav", ".m4a"}
	pdfExtensions   = []string{".pdf"}
)

// NormalizeAssetPath cleans a figure src and reports whether it is a plain relative path
// that stays inside the document directory.
func NormalizeAssetPath(src string) (string, bool) {
	if u, err := url.Parse(src); err == nil && u.Scheme != "" {
		return "", false
	}
	cleaned := path.Clean(src)
	if !fs.ValidPath(cleaned) || cleaned == "." || !filepath.IsLocal(cleaned) {
		return "", false
	}
	return cleaned, true
}

// ValidateFile reads an XML file and validates it. Figure files are resolved relative to the file's directory.
func ValidateFile(file string) (ValidationResult, error) {
	input, err := os.ReadFile(file)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("read XML file %q: %w", file, err)
	}
	_, issues := Parse(input, Options{HasAsset: DirAssetChecker(filepath.Dir(file))})
	return ValidationResult{Valid: len(issues) == 0, File: file, Errors: issues}, nil
}

// DirAssetChecker resolves figure paths against dir on the local filesystem, confining symlinks via os.OpenRoot.
func DirAssetChecker(dir string) func(string) bool {
	return func(normalized string) bool {
		root, err := os.OpenRoot(dir)
		if err != nil {
			return false
		}
		defer func() { _ = root.Close() }()
		info, err := root.Stat(filepath.FromSlash(normalized))
		return err == nil && info.Mode().IsRegular()
	}
}

// Parse parses and validates input. The returned document is nil only when the XML is not well formed.
func Parse(input []byte, opts Options) (*Document, []ValidationError) {
	tree, fatal := parseTree(input)
	if fatal != nil {
		return nil, []ValidationError{*fatal}
	}
	d := &Document{input: input, tree: tree}
	d.readMeta()
	issues := protocolIssues(tree)
	issues = append(issues, d.checkBlocks()...)
	issues = append(issues, d.checkURLs()...)
	issues = append(issues, d.checkAssets(opts)...)
	slices.SortStableFunc(issues, func(a, b ValidationError) int { return cmp.Compare(a.Line, b.Line) })
	if issues == nil {
		issues = []ValidationError{}
	}
	return d, issues
}

// protocolIssues runs protocol.sch and decodes its "CODE|field|message" results.
func protocolIssues(tree *helium.Document) []ValidationError {
	ctx := context.Background()
	collector := helium.NewErrorCollector(ctx, helium.ErrorLevelNone)
	_ = schematron.NewValidator(protocol()).ErrorHandler(collector).Validate(ctx, tree)
	var issues []ValidationError
	for _, err := range collector.Errors() {
		var ve *schematron.ValidationError
		if !errors.As(err, &ve) {
			continue
		}
		code, rest, _ := strings.Cut(ve.Message, "|")
		field, rest, _ := strings.Cut(rest, "|")
		message, hint, _ := strings.Cut(rest, "|")
		issues = append(issues, ValidationError{
			Line:    ve.Line,
			Code:    code,
			Field:   field,
			Message: strings.TrimSpace(message),
			Hint:    strings.TrimSpace(hint),
		})
	}
	return issues
}

// checkURLs verifies that every URL attribute and canonical-url element uses http(s).
func (d *Document) checkURLs() []ValidationError {
	var issues []ValidationError
	for _, n := range find(d.tree, urlXPath) {
		el, ok := n.(*helium.Element)
		if !ok {
			continue
		}
		if el.Name() == "canonical-url" {
			if issue := validateHTTPURL(el.Line(), "meta.canonical-url", text(el)); issue != nil {
				issues = append(issues, *issue)
			}
			continue
		}
		attrs := []string{"url"}
		if el.Name() == "paper-card" {
			attrs = []string{"url", "code-url"}
		}
		for _, attr := range attrs {
			raw, has := el.GetAttribute(attr)
			if !has {
				continue
			}
			if issue := validateHTTPURL(el.Line(), el.Name()+"."+attr, raw); issue != nil {
				issues = append(issues, *issue)
			}
		}
	}
	return issues
}

func validateHTTPURL(line int, field, raw string) *ValidationError {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return &ValidationError{
			Line: line, Code: "INVALID_URL", Field: field, Message: "must be an http or https URL",
		}
	}
	return nil
}

// checkAssets validates figure, audio, and attachment src values (plus v1.2 remote image URLs)
// and collects Assets, AssetRefs, and RemoteFigures.
func (d *Document) checkAssets(opts Options) []ValidationError {
	if opts.StoredAssets {
		return nil
	}
	var issues []ValidationError
	seen := map[string]bool{}
	seenRemote := map[string]bool{}
	for _, n := range find(d.tree, assetXPath) {
		el := n.(*helium.Element)
		elemName := el.Name()
		if (elemName == "audio" || elemName == "attachment") && d.Version == "1.1" {
			continue // reported by protocol.sch v12-features
		}
		src, _ := el.GetAttribute("src")
		if strings.TrimSpace(src) == "" {
			continue // reported by protocol.sch / checkAudio / checkAttachment
		}
		if elemName == "figure" && IsRemoteURLCandidate(src) {
			if d.Version != "1.2" {
				msg := "src must be a relative path inside the document directory: " + src
				hint := ""
				if u, err := url.Parse(src); err == nil && (strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")) {
					msg = fmt.Sprintf("remote figure src %q requires <document version=\"1.2\">", src)
					hint = `upgrade the root element to <document version="1.2"> or use a relative local image path`
				}
				issues = append(issues, ValidationError{
					Line: n.Line(), Code: "INVALID_ASSET_PATH", Field: "figure.src", Message: msg, Hint: hint,
				})
				continue
			}
			code, msg, hint, ok := ValidateRemoteImageURL(src)
			if !ok {
				issues = append(issues, ValidationError{
					Line: n.Line(), Code: code, Field: "figure.src", Message: msg, Hint: hint,
				})
				continue
			}
			if !seenRemote[src] {
				seenRemote[src] = true
				d.RemoteFigures = append(d.RemoteFigures, RemoteFigureRef{Line: n.Line(), URL: src})
			}
			continue
		}
		kind := AssetKindImage
		field := "figure.src"
		exts := imageExtensions
		typeErrMsg := "image must be png, jpg, jpeg, webp or svg: %s"
		missingMsg := "local image file not found: %s"
		switch elemName {
		case "audio":
			kind = AssetKindAudio
			field = "audio.src"
			exts = audioExtensions
			typeErrMsg = "audio must be mp3, wav or m4a: %s"
			missingMsg = "local audio file not found: %s"
		case "attachment":
			kind = AssetKindPDF
			field = "attachment.src"
			exts = pdfExtensions
			typeErrMsg = "attachment must be pdf: %s"
			missingMsg = "local PDF file not found: %s"
		}
		add := func(code, format string) {
			issues = append(issues, ValidationError{
				Line: n.Line(), Code: code, Field: field, Message: fmt.Sprintf(format, src),
			})
		}
		normalized, ok := NormalizeAssetPath(src)
		switch {
		case !ok:
			add("INVALID_ASSET_PATH", "src must be a relative path inside the document directory: %s")
		case !slices.Contains(exts, strings.ToLower(path.Ext(normalized))):
			add("INVALID_ASSET_TYPE", typeErrMsg)
		case opts.HasAsset != nil && !opts.HasAsset(normalized):
			add("MISSING_ASSET", missingMsg)
		case !seen[normalized]:
			seen[normalized] = true
			d.Assets = append(d.Assets, normalized)
			d.AssetRefs = append(d.AssetRefs, AssetRef{Path: normalized, Kind: kind})
		}
	}
	return issues
}
