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
	// HasAsset reports whether the normalized local image path exists.
	// When nil, figure existence is not checked.
	HasAsset func(normalized string) bool
	// StoredAssets marks input whose figure src values were already rewritten to stored asset
	// URLs (as in the XML kept in storage). Figure src values are then neither normalized nor
	// checked, and Document.Assets stays empty.
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
	figureXPath = "//figure[@src]"
	urlXPath    = "/document/meta/canonical-url | /document/body//paper-card[@url or @code-url] | /document/body//*[self::repo-card or self::source][@url] | /document/body//*[self::ref or self::quote or self::news-item][@url]"
)

var imageExtensions = []string{".png", ".jpg", ".jpeg", ".webp", ".svg"}

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
	issues = append(issues, d.checkFigures(opts)...)
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

// checkFigures validates figure src values against the local filesystem rules and collects Assets.
func (d *Document) checkFigures(opts Options) []ValidationError {
	if opts.StoredAssets {
		return nil
	}
	var issues []ValidationError
	seen := map[string]bool{}
	for _, n := range find(d.tree, figureXPath) {
		src, _ := n.(*helium.Element).GetAttribute("src")
		if strings.TrimSpace(src) == "" {
			continue // reported by protocol.sch
		}
		add := func(code, format string) {
			issues = append(issues, ValidationError{
				Line: n.Line(), Code: code, Field: "figure.src", Message: fmt.Sprintf(format, src),
			})
		}
		normalized, ok := NormalizeAssetPath(src)
		switch {
		case !ok:
			add("INVALID_ASSET_PATH", "src must be a relative path inside the document directory: %s")
		case !slices.Contains(imageExtensions, strings.ToLower(path.Ext(normalized))):
			add("INVALID_ASSET_TYPE", "image must be png, jpg, jpeg, webp or svg: %s")
		case opts.HasAsset != nil && !opts.HasAsset(normalized):
			add("MISSING_ASSET", "local image file not found: %s")
		case !seen[normalized]:
			seen[normalized] = true
			d.Assets = append(d.Assets, normalized)
		}
	}
	return issues
}
