// Package schema defines the shared Aumbry XML protocol and validation types.
package schema

import "encoding/xml"

// Document is the envelope shared by the CLI validator and server ingest path.
type Document struct {
	XMLName xml.Name `xml:"document"`
	Version string   `xml:"version,attr"`
	Meta    Metadata `xml:"meta"`
	Body    Body     `xml:"body"`
}

// Body preserves the XML content for the rich-text renderer.
type Body struct {
	InnerXML string `xml:",innerxml"`
}

// Metadata contains the required document fields and optional links.
type Metadata struct {
	Title        string   `xml:"title"`
	Type         string   `xml:"type"`
	Date         string   `xml:"date"`
	Summary      string   `xml:"summary"`
	SourceAgent  string   `xml:"source-agent"`
	CanonicalURL string   `xml:"canonical-url"`
	Tags         []string `xml:"tags>tag"`
	RelatedIDs   []string `xml:"related-ids>id"`
}

// ValidationError is a machine-readable validation issue.
type ValidationError struct {
	Line    int    `json:"line,omitempty"`
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationResult is the stable output shape used by the CLI.
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	File   string            `json:"file,omitempty"`
	Errors []ValidationError `json:"errors"`
}
