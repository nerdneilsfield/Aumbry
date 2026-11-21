package schema

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lestrrat-go/helium"
	"github.com/lestrrat-go/helium/xpath1"
)

// find evaluates a constant XPath expression. The expressions in this package are fixed
// strings, so a compile or evaluation error is a programming error.
func find(tree *helium.Document, expr string) []helium.Node {
	nodes, err := xpath1.Find(context.Background(), tree, expr)
	if err != nil {
		panic(fmt.Sprintf("schema: xpath %q: %v", expr, err))
	}
	return nodes
}

func text(n helium.Node) string { return strings.TrimSpace(string(n.Content())) }

func texts(tree *helium.Document, expr string) []string {
	var out []string
	for _, n := range find(tree, expr) {
		out = append(out, text(n))
	}
	return out
}

func parseTree(input []byte) (*helium.Document, *ValidationError) {
	tree, err := helium.NewParser().Parse(context.Background(), input)
	if err != nil {
		issue := &ValidationError{Code: "INVALID_XML", Field: "document", Message: err.Error()}
		var pe helium.ErrParseError
		if errors.As(err, &pe) {
			issue.Line = pe.LineNumber
			if pe.Err != nil {
				issue.Message = pe.Err.Error()
			}
		}
		return nil, issue
	}
	// The protocol has no use for DTDs; refusing them rules out entity tricks outright.
	if dtd := tree.IntSubset(); dtd != nil {
		return nil, &ValidationError{Line: dtd.Line(), Code: "INVALID_XML", Field: "document", Message: "DOCTYPE declarations are not allowed"}
	}
	return tree, nil
}

func (d *Document) readMeta() {
	if root := d.tree.DocumentElement(); root != nil {
		d.Version, _ = root.GetAttribute("version")
	}
	field := func(name string) string {
		if values := texts(d.tree, "/document/meta/"+name+"[1]"); len(values) > 0 {
			return values[0]
		}
		return ""
	}
	d.Meta = Metadata{
		Title: field("title"), Type: field("type"), Date: field("date"), Summary: field("summary"),
		SourceAgent: field("source-agent"), CanonicalURL: field("canonical-url"),
		Tags: texts(d.tree, "/document/meta/tags/tag"), RelatedIDs: texts(d.tree, "/document/meta/related-ids/id"),
	}
}

// RewriteAssets returns the XML with every <figure src> replaced via mapping,
// which is keyed by normalized path. Everything else, CDATA sections included, is
// carried over by the XML tree round trip; empty elements are written in the self-closing form
// and an XML declaration is only written if the input had one.
func (d *Document) RewriteAssets(mapping map[string]string) ([]byte, error) {
	// Work on a fresh tree so the Document stays unchanged and can be rewritten again.
	tree, issue := parseTree(d.input)
	if issue != nil {
		return nil, errors.New(issue.Message)
	}
	for _, n := range find(tree, figureXPath) {
		fig := n.(*helium.Element)
		src, _ := fig.GetAttribute("src")
		normalized, _ := NormalizeAssetPath(src)
		replacement, ok := mapping[normalized]
		if !ok {
			return nil, fmt.Errorf("no replacement for figure src %q", src)
		}
		if err := fig.SetAttribute("src", replacement); err != nil {
			return nil, err
		}
	}
	var out strings.Builder
	if err := helium.NewWriter().XMLDeclaration(tree.Version() != "").WriteTo(&out, tree); err != nil {
		return nil, err
	}
	return []byte(out.String()), nil
}

// PlainText returns the document text and searchable attributes for full-text indexing.
func (d *Document) PlainText() string {
	var b strings.Builder
	for _, n := range find(d.tree, "//@title | //@caption | //@name | //@authors | //text()") {
		b.Write(n.Content())
		b.WriteByte(' ')
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
