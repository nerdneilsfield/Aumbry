package schema

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Options controls the checks that depend on where the document is validated.
type Options struct {
	// HasAsset reports whether the normalized local image path exists.
	// When nil, figure existence is not checked.
	HasAsset func(normalized string) bool
}

var (
	idPattern       = regexp.MustCompile(`^[A-Za-z0-9_-]{12}$`)
	imageExtensions = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".svg": true}
)

// NormalizeAssetPath cleans a figure src and reports whether it is a plain relative path
// that stays inside the document directory.
func NormalizeAssetPath(src string) (string, bool) {
	if src == "" || strings.Contains(src, `\`) || strings.Contains(src, ":") || strings.HasPrefix(src, "/") {
		return "", false
	}
	cleaned := path.Clean(src)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned == "." {
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

// DirAssetChecker resolves figure paths against dir on the local filesystem.
func DirAssetChecker(dir string) func(string) bool {
	return func(normalized string) bool {
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(normalized)))
		return err == nil && info.Mode().IsRegular()
	}
}

// Parse parses and validates input. The returned document is nil only when the XML is not well formed.
func Parse(input []byte, opts Options) (*Document, []ValidationError) {
	root, fatal := parseTree(input)
	if fatal != nil {
		return nil, []ValidationError{*fatal}
	}
	v := &validator{opts: opts, doc: &Document{input: input, root: root}, seen: map[string]bool{}}
	v.document(root)
	if v.issues == nil {
		v.issues = []ValidationError{}
	}
	return v.doc, v.issues
}

type validator struct {
	opts   Options
	doc    *Document
	issues []ValidationError
	seen   map[string]bool
}

func (v *validator) add(n *node, code, field, format string, args ...any) {
	v.issues = append(v.issues, ValidationError{
		Line: n.line, Code: code, Field: field, Message: fmt.Sprintf(format, args...),
	})
}

func (v *validator) document(root *node) {
	if root.name != "document" {
		v.add(root, "INVALID_ROOT", "document", "root element must be <document>, got <%s>", root.name)
		return
	}
	if version, _ := root.attr("version"); version != "1.0" {
		v.add(root, "UNSUPPORTED_VERSION", "document.version", `version must be "1.0"`)
	}
	v.noText(root, "document")
	meta, body := root.child("meta"), root.child("body")
	for _, k := range root.kids {
		if k != meta && k != body {
			v.add(k, "UNKNOWN_TAG", "document", "unexpected <%s> inside <document>", k.name)
		}
	}
	if meta == nil {
		v.add(root, "MISSING_META", "document.meta", "<meta> is required")
	} else {
		v.meta(meta)
	}
	if body == nil {
		v.add(root, "MISSING_BODY", "document.body", "<body> is required")
	} else {
		v.container(body, "body", map[string]bool{"section": true, "sources": true})
	}
}

func (v *validator) meta(meta *node) {
	m := &v.doc.Meta
	text := func(name string) string {
		if k := meta.child(name); k != nil {
			return strings.TrimSpace(k.text)
		}
		return ""
	}
	m.Title, m.Type, m.Date = text("title"), text("type"), text("date")
	m.Summary, m.SourceAgent, m.CanonicalURL = text("summary"), text("source-agent"), text("canonical-url")
	for _, f := range []struct{ name, value string }{
		{"title", m.Title}, {"type", m.Type}, {"date", m.Date}, {"summary", m.Summary}, {"source-agent", m.SourceAgent},
	} {
		if f.value == "" {
			v.add(meta, "REQUIRED_FIELD", "meta."+f.name, "meta.%s is required", f.name)
		}
	}
	if m.Type != "" && m.Type != "report" && m.Type != "post" {
		v.add(meta.child("type"), "INVALID_TYPE", "meta.type", "type must be report or post")
	}
	if m.Date != "" {
		if _, err := time.Parse("2006-01-02", m.Date); err != nil {
			v.add(meta.child("date"), "INVALID_DATE", "meta.date", "date must use YYYY-MM-DD format")
		}
	}
	if m.CanonicalURL != "" && !httpURL(m.CanonicalURL) {
		v.add(meta.child("canonical-url"), "INVALID_URL", "meta.canonical-url", "canonical-url must be an http or https URL")
	}
	if tags := meta.child("tags"); tags != nil {
		for _, k := range tags.kids {
			if tag := strings.TrimSpace(k.text); k.name == "tag" && tag != "" {
				m.Tags = append(m.Tags, tag)
			} else {
				v.add(k, "INVALID_TAG", "meta.tags", "<tags> may only contain non-empty <tag> elements")
			}
		}
	}
	if related := meta.child("related-ids"); related != nil {
		for _, k := range related.kids {
			if id := strings.TrimSpace(k.text); k.name == "id" && idPattern.MatchString(id) {
				m.RelatedIDs = append(m.RelatedIDs, id)
			} else {
				v.add(k, "INVALID_ID", "meta.related-ids", "related-ids may only contain <id> elements holding a 12-character item id")
			}
		}
	}
}

// children allowed per container element.
var allowedChildren = map[string]map[string]bool{
	"section":  {"md": true, "figure": true, "paper-card": true, "repo-card": true, "compare-table": true, "callout": true, "collapse": true, "sources": true},
	"collapse": {"md": true, "figure": true},
}

func (v *validator) container(n *node, field string, allowed map[string]bool) {
	v.noText(n, field)
	for _, k := range n.kids {
		if !allowed[k.name] {
			v.add(k, "UNKNOWN_TAG", field, "<%s> is not allowed inside <%s>", k.name, n.name)
			continue
		}
		v.block(k)
	}
}

func (v *validator) block(n *node) {
	switch n.name {
	case "section":
		v.requireAttrs(n, "section", "title")
		v.container(n, "section", allowedChildren["section"])
	case "collapse":
		v.requireAttrs(n, "collapse", "title")
		v.container(n, "collapse", allowedChildren["collapse"])
	case "md":
		v.leaf(n, "md")
	case "figure":
		v.figure(n)
	case "paper-card":
		v.leaf(n, "paper-card")
		v.requireAttrs(n, "paper-card", "title", "url")
		v.urlAttr(n, "paper-card", "url")
		if id, ok := n.attr("related-id"); ok && !idPattern.MatchString(id) {
			v.add(n, "INVALID_ID", "paper-card.related-id", "related-id must be a 12-character item id")
		}
	case "repo-card":
		v.leaf(n, "repo-card")
		v.requireAttrs(n, "repo-card", "name", "url")
		v.urlAttr(n, "repo-card", "url")
	case "callout":
		v.leaf(n, "callout")
		switch kind, _ := n.attr("type"); kind {
		case "info", "warn", "insight":
		default:
			v.add(n, "INVALID_ATTR", "callout.type", "callout type must be info, warn or insight")
		}
	case "compare-table":
		v.table(n)
	case "sources":
		v.sources(n)
	}
}

// leaf checks an element whose content is text only.
func (v *validator) leaf(n *node, field string) {
	for _, k := range n.kids {
		v.add(k, "UNKNOWN_TAG", field, "<%s> cannot contain child elements", n.name)
	}
}

func (v *validator) noText(n *node, field string) {
	if strings.TrimSpace(n.text) != "" {
		v.add(n, "UNEXPECTED_TEXT", field, "<%s> cannot contain text directly; wrap it in <md>", n.name)
	}
}

func (v *validator) requireAttrs(n *node, field string, names ...string) {
	for _, name := range names {
		if value, _ := n.attr(name); strings.TrimSpace(value) == "" {
			v.add(n, "MISSING_ATTR", field+"."+name, "<%s> requires the %q attribute", n.name, name)
		}
	}
}

func (v *validator) urlAttr(n *node, field, name string) {
	if value, ok := n.attr(name); ok && value != "" && !httpURL(value) {
		v.add(n, "INVALID_URL", field+"."+name, "%s must be an http or https URL", name)
	}
}

func httpURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func (v *validator) figure(n *node) {
	v.leaf(n, "figure")
	src, _ := n.attr("src")
	if strings.TrimSpace(src) == "" {
		v.add(n, "MISSING_ATTR", "figure.src", `<figure> requires the "src" attribute`)
		return
	}
	normalized, ok := NormalizeAssetPath(src)
	if !ok {
		v.add(n, "INVALID_ASSET_PATH", "figure.src", "src must be a relative path inside the document directory: %s", src)
		return
	}
	if !imageExtensions[strings.ToLower(path.Ext(normalized))] {
		v.add(n, "INVALID_ASSET_TYPE", "figure.src", "image must be png, jpg, jpeg, webp or svg: %s", src)
		return
	}
	if v.opts.HasAsset != nil && !v.opts.HasAsset(normalized) {
		v.add(n, "MISSING_ASSET", "figure.src", "local image file not found: %s", src)
		return
	}
	if !v.seen[normalized] {
		v.seen[normalized] = true
		v.doc.Assets = append(v.doc.Assets, normalized)
	}
}

func (v *validator) table(n *node) {
	v.noText(n, "compare-table")
	columns := n.child("columns")
	if columns == nil || len(columns.kids) == 0 {
		v.add(n, "TABLE_NO_COLUMNS", "compare-table.columns", "<compare-table> requires <columns> with at least one <col>")
		return
	}
	for _, k := range columns.kids {
		if k.name != "col" {
			v.add(k, "UNKNOWN_TAG", "compare-table.columns", "<columns> may only contain <col>")
		}
	}
	rows := 0
	for _, k := range n.kids {
		switch k.name {
		case "columns":
		case "row":
			rows++
			cells := 0
			for _, c := range k.kids {
				if c.name == "cell" {
					cells++
				} else {
					v.add(c, "UNKNOWN_TAG", "compare-table.row", "<row> may only contain <cell>")
				}
			}
			if cells != len(columns.kids) {
				v.add(k, "TABLE_COL_MISMATCH", fmt.Sprintf("compare-table.row[%d]", rows),
					"expected %d cells to match columns, got %d", len(columns.kids), cells)
			}
		default:
			v.add(k, "UNKNOWN_TAG", "compare-table", "<%s> is not allowed inside <compare-table>", k.name)
		}
	}
	if rows == 0 {
		v.add(n, "TABLE_NO_ROWS", "compare-table.row", "<compare-table> requires at least one <row>")
	}
}

func (v *validator) sources(n *node) {
	v.noText(n, "sources")
	count := 0
	for _, k := range n.kids {
		if k.name != "source" {
			v.add(k, "UNKNOWN_TAG", "sources", "<sources> may only contain <source>")
			continue
		}
		count++
		v.requireAttrs(k, "source", "title", "url")
		v.urlAttr(k, "source", "url")
	}
	if count == 0 {
		v.add(n, "MISSING_SOURCE", "sources", "<sources> requires at least one <source>")
	}
}
