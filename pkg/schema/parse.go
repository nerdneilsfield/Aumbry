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
		if hint := detectMissingCDATAHint(input, issue.Line); hint != "" {
			issue.Hint = hint
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
	for _, n := range find(tree, d.figureXPath()) {
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

var cdataTextElements = map[string]bool{
	"md": true, "callout": true, "mermaid": true, "chart": true,
	"equation": true, "code-block": true, "claim": true, "quote": true,
	"paper-card": true, "repo-card": true, "detail": true,
}

// detectMissingCDATAHint checks whether an XML parse error occurs inside (or on the closing
// line of) a CDATA-bearing text element containing unescaped '&' or raw '<' without <![CDATA[.
func detectMissingCDATAHint(input []byte, errLine int) string {
	s := string(input)
	type frame struct {
		name         string
		openLine     int
		sawUnescaped bool
	}
	var cur *frame
	line := 1
	n := len(s)

	hasValidEntityAt := func(i int) bool {
		if i >= n || s[i] != '&' {
			return false
		}
		rest := s[i+1:]
		semi := strings.IndexByte(rest, ';')
		if semi <= 0 || semi > 12 {
			return false
		}
		ent := rest[:semi]
		switch ent {
		case "amp", "lt", "gt", "apos", "quot":
			return true
		}
		if strings.HasPrefix(ent, "#x") || strings.HasPrefix(ent, "#X") {
			hex := ent[2:]
			if len(hex) == 0 {
				return false
			}
			for _, r := range hex {
				if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
					return false
				}
			}
			return true
		}
		if strings.HasPrefix(ent, "#") {
			dec := ent[1:]
			if len(dec) == 0 {
				return false
			}
			for _, r := range dec {
				if r < '0' || r > '9' {
					return false
				}
			}
			return true
		}
		return false
	}

	skipTagEnd := func(i int) (end int, selfClosing bool, ok bool) {
		var quote byte
		for i < n {
			c := s[i]
			if c == '\n' {
				line++
			}
			if quote != 0 {
				if c == quote {
					quote = 0
				}
				i++
				continue
			}
			if c == '"' || c == '\'' {
				quote = c
				i++
				continue
			}
			if c == '>' {
				prev := i - 1
				for prev >= 0 && (s[prev] == ' ' || s[prev] == '\t' || s[prev] == '\r' || s[prev] == '\n') {
					prev--
				}
				return i + 1, prev >= 0 && s[prev] == '/', true
			}
			i++
		}
		return n, false, false
	}

	countLines := func(from, to int) {
		for k := from; k < to; k++ {
			if s[k] == '\n' {
				line++
			}
		}
	}

	for i := 0; i < n; {
		if errLine > 0 && line > errLine {
			break
		}
		if s[i] == '\n' {
			line++
			i++
			continue
		}
		if strings.HasPrefix(s[i:], "<![CDATA[") {
			end := strings.Index(s[i+9:], "]]>")
			if end < 0 {
				break
			}
			countLines(i, i+9+end+3)
			i += 9 + end + 3
			continue
		}
		if strings.HasPrefix(s[i:], "<!--") {
			end := strings.Index(s[i+4:], "-->")
			if end < 0 {
				break
			}
			countLines(i, i+4+end+3)
			i += 4 + end + 3
			continue
		}
		if strings.HasPrefix(s[i:], "<?") {
			end := strings.Index(s[i+2:], "?>")
			if end < 0 {
				break
			}
			countLines(i, i+2+end+2)
			i += 2 + end + 2
			continue
		}

		if cur == nil {
			if s[i] == '<' && i+1 < n && s[i+1] != '/' && s[i+1] != '!' {
				j := i + 1
				for j < n && s[j] != ' ' && s[j] != '\t' && s[j] != '\r' && s[j] != '\n' && s[j] != '>' && s[j] != '/' {
					j++
				}
				name := s[i+1 : j]
				end, selfClosing, ok := skipTagEnd(j)
				if !ok {
					break
				}
				if !selfClosing && cdataTextElements[name] {
					cur = &frame{name: name, openLine: line}
				}
				i = end
				continue
			}
			i++
			continue
		}

		// Inside a CDATA-bearing element.
		if strings.HasPrefix(s[i:], "</"+cur.name) {
			j := i + 2 + len(cur.name)
			for j < n && (s[j] == ' ' || s[j] == '\t' || s[j] == '\r' || s[j] == '\n') {
				if s[j] == '\n' {
					line++
				}
				j++
			}
			if j < n && s[j] == '>' {
				closeLine := line
				if cur.sawUnescaped && (errLine <= 0 || (errLine >= cur.openLine && errLine <= closeLine)) {
					return fmt.Sprintf("wrap content inside <%s> with <![CDATA[ ... ]]>", cur.name)
				}
				cur = nil
				i = j + 1
				continue
			}
		}

		if s[i] == '&' && !hasValidEntityAt(i) {
			cur.sawUnescaped = true
			if errLine <= 0 || errLine == line {
				return fmt.Sprintf("wrap content inside <%s> with <![CDATA[ ... ]]>", cur.name)
			}
		} else if s[i] == '<' {
			cur.sawUnescaped = true
		}
		i++
	}

	if cur != nil && cur.sawUnescaped {
		return fmt.Sprintf("wrap content inside <%s> with <![CDATA[ ... ]]>", cur.name)
	}
	return ""
}

// PlainText returns the document text and searchable attributes for full-text indexing.
// v1.0 documents retain the full-tree text projection; v1.1 documents exclude code, formula,
// diagram, chart JSON, quiz options, and next-steps buttons while indexing v1.1 attributes.
func (d *Document) PlainText() string {
	var b strings.Builder
	if d.Version == "1.1" {
		if root := d.tree.DocumentElement(); root != nil {
			appendV11IndexedText(&b, root)
		}
		return strings.Join(strings.Fields(b.String()), " ")
	}
	for _, n := range find(d.tree, "//@title | //@caption | //@name | //@authors | //text()") {
		b.Write(n.Content())
		b.WriteByte(' ')
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func appendV11IndexedText(b *strings.Builder, el *helium.Element) {
	name := el.Name()
	if name == "next-steps" || name == "option" {
		return
	}
	writeAttr := func(attr string) {
		if v, ok := el.GetAttribute(attr); ok && strings.TrimSpace(v) != "" {
			b.WriteString(v)
			b.WriteByte(' ')
		}
	}
	if name == "equation" || name == "mermaid" || name == "chart" || name == "code-block" {
		writeAttr("caption")
		if name == "code-block" {
			writeAttr("file")
		}
		return
	}

	for _, attr := range []string{"title", "caption", "name", "authors"} {
		writeAttr(attr)
	}
	switch name {
	case "metric":
		writeAttr("label")
		writeAttr("value")
	case "excluded":
		writeAttr("reason")
	case "quote":
		writeAttr("source")
		writeAttr("author")
	case "check":
		writeAttr("detail")
	case "news-item":
		writeAttr("source")
	}

	for cur := el.FirstChild(); cur != nil; cur = cur.NextSibling() {
		switch cur.Type() {
		case helium.TextNode, helium.CDATASectionNode:
			b.Write(cur.Content())
			b.WriteByte(' ')
		case helium.ElementNode:
			if child, ok := cur.(*helium.Element); ok {
				appendV11IndexedText(b, child)
			}
		}
	}
}
