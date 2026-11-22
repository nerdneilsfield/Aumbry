// Package schema defines the shared Aumbry XML protocol, validation and text extraction.
package schema

import "github.com/lestrrat-go/helium"

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
	Version string
	Meta    Metadata
	// Assets lists the normalized local paths referenced by <figure src>, in document order, without duplicates.
	Assets []string

	input []byte
	tree  *helium.Document
}
