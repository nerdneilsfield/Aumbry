// Package schema defines the shared Aumbry XML protocol, validation and text extraction.
package schema

import "github.com/lestrrat-go/helium"

// AssetKind classifies a local binary asset referenced by a document.
type AssetKind string

const (
	// AssetKindImage represents a local image referenced by <figure src>.
	AssetKindImage AssetKind = "image"
	// AssetKindAudio represents a local audio file referenced by <audio src>.
	AssetKindAudio AssetKind = "audio"
)

// AssetRef pairs a deduplicated normalized local asset path with its element category.
type AssetRef struct {
	Path string
	Kind AssetKind
}

// OutgoingLink describes a directed link from a document to another item in Aumbry.
type OutgoingLink struct {
	TargetItemID string
	Kind         string
}

// AudioChapter represents a <chapter> entry inside an <audio> block.
type AudioChapter struct {
	Start  string `json:"start"`
	Title  string `json:"title"`
	Target string `json:"target,omitempty"`
}

// AudioInfo holds the primary <audio> projection stored with an item for feed generation.
type AudioInfo struct {
	Src      string
	Hash     string
	Title    string
	Chapters []AudioChapter
}

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
	// Assets lists the normalized local paths referenced by <figure src> and <audio src>,
	// in document order, without duplicates.
	Assets []string
	// AssetRefs lists the typed normalized local asset references in document order, without duplicates.
	AssetRefs []AssetRef

	input []byte
	tree  *helium.Document
}
