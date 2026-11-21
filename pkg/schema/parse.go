package schema

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

func lineAt(input []byte, offset int64) int {
	if offset > int64(len(input)) {
		offset = int64(len(input))
	}
	return bytes.Count(input[:offset], []byte{'\n'}) + 1
}

// parseTree builds the element tree. Syntax errors are reported as INVALID_XML.
func parseTree(input []byte) (*node, *ValidationError) {
	decoder := xml.NewDecoder(bytes.NewReader(input))
	invalid := func(message string) (*node, *ValidationError) {
		return nil, &ValidationError{
			Line: lineAt(input, decoder.InputOffset()), Code: "INVALID_XML", Field: "document", Message: message,
		}
	}
	var root *node
	var stack []*node
	for {
		before := decoder.InputOffset()
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return invalid(err.Error())
		}
		switch t := token.(type) {
		case xml.StartElement:
			n := &node{
				name: t.Name.Local, attrs: t.Attr, line: lineAt(input, before),
				start: before, end: decoder.InputOffset(),
			}
			n.selfClosing = bytes.HasSuffix(input[n.start:n.end], []byte("/>"))
			if len(stack) == 0 {
				if root != nil {
					return invalid("multiple root elements")
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.kids = append(parent.kids, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text += string(t)
			}
		}
	}
	if root == nil {
		return invalid("document is empty")
	}
	return root, nil
}

// RewriteAssets returns the XML with every <figure src> replaced via mapping,
// which is keyed by normalized path. All other bytes are preserved.
func (d *Document) RewriteAssets(mapping map[string]string) ([]byte, error) {
	var figures []*node
	var collect func(*node)
	collect = func(n *node) {
		if n.name == "figure" {
			figures = append(figures, n)
		}
		for _, k := range n.kids {
			collect(k)
		}
	}
	collect(d.root)

	var out bytes.Buffer
	var pos int64
	for _, fig := range figures {
		src, _ := fig.attr("src")
		normalized, _ := NormalizeAssetPath(src)
		replacement, ok := mapping[normalized]
		if !ok {
			return nil, fmt.Errorf("no replacement for figure src %q", src)
		}
		out.Write(d.input[pos:fig.start])
		out.WriteString("<figure")
		for _, a := range fig.attrs {
			value := a.Value
			if a.Name.Local == "src" {
				value = replacement
			}
			var escaped strings.Builder
			if err := xml.EscapeText(&escaped, []byte(value)); err != nil {
				return nil, err
			}
			fmt.Fprintf(&out, ` %s="%s"`, a.Name.Local, escaped.String())
		}
		if fig.selfClosing {
			out.WriteString("/>")
		} else {
			out.WriteString(">")
		}
		pos = fig.end
	}
	out.Write(d.input[pos:])
	return out.Bytes(), nil
}

// PlainText returns the document text and searchable attributes for full-text indexing.
func (d *Document) PlainText() string {
	var b strings.Builder
	var walk func(*node)
	walk = func(n *node) {
		for _, name := range []string{"title", "caption", "name", "authors"} {
			if v, ok := n.attr(name); ok {
				b.WriteString(v)
				b.WriteByte(' ')
			}
		}
		b.WriteString(n.text)
		b.WriteByte(' ')
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(d.root)
	return strings.Join(strings.Fields(b.String()), " ")
}
