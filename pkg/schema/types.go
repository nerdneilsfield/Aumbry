// Package schema defines the shared Aumbry XML protocol, validation and text extraction.
package schema

import "encoding/xml"

// Metadata is the parsed <meta> block of a document.
type Metadata struct {
	Title        string
	Type         string
	Date         string
	Summary      string
	SourceAgent  string
	CanonicalURL string
	Tags         []string
	RelatedIDs   []string
}

// Document is a parsed Aumbry document. It is only returned together with the
// validation issues found while parsing it; callers must treat any issue as fatal.
type Document struct {
	Meta Metadata
	// Assets lists the normalized local paths referenced by <figure src>, in document order, without duplicates.
	Assets []string

	input []byte
	root  *node
}

// node is one XML element with the source position needed for error lines and rewriting.
type node struct {
	name        string
	attrs       []xml.Attr
	kids        []*node
	text        string
	line        int
	start, end  int64 // byte range of the start tag
	selfClosing bool
}

func (n *node) attr(name string) (string, bool) {
	for _, a := range n.attrs {
		if a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

func (n *node) child(name string) *node {
	for _, k := range n.kids {
		if k.name == name {
			return k
		}
	}
	return nil
}
