// Package cjk holds the single bigram expansion shared by indexing (ingest, reindex) and
// querying, so the text written to items_fts_cjk and the phrases matched against it are always
// split the same way.
package cjk

import (
	"strings"
	"unicode"
)

// IsCJK reports whether r belongs to a script that is indexed as overlapping two-character
// groups: Han, Hiragana, Katakana and Hangul. The prolonged sound mark U+30FC is in the Common
// script but is part of Katakana words, so it counts as well.
func IsCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) || r == 'ー'
}

// Segment is a maximal run of text that is either entirely CJK or entirely not.
type Segment struct {
	Text string
	CJK  bool
}

// Segments splits s into alternating CJK and non-CJK runs. Punctuation, whitespace, Latin
// letters, digits and emoji are all non-CJK, so any of them ends a CJK run.
func Segments(s string) []Segment {
	var out []Segment
	start := 0
	var cur bool
	for i, r := range s {
		if c := IsCJK(r); i > 0 && c != cur {
			out = append(out, Segment{Text: s[start:i], CJK: cur})
			start = i
		}
		cur = IsCJK(r)
	}
	if start < len(s) {
		out = append(out, Segment{Text: s[start:], CJK: cur})
	}
	return out
}

// Bigrams returns the overlapping two-rune groups of a CJK run, joined by spaces. A run shorter
// than two runes has none.
func Bigrams(run string) string {
	runes := []rune(run)
	if len(runes) < 2 {
		return ""
	}
	var b strings.Builder
	for i := 0; i+1 < len(runes); i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(runes[i])
		b.WriteRune(runes[i+1])
	}
	return b.String()
}

// Expand returns the text stored in items_fts_cjk: CJK runs become their bigrams and every other
// run is kept as is, with a space between runs so the unicode61 tokenizer splits them.
func Expand(s string) string {
	var parts []string
	for _, seg := range Segments(s) {
		if !seg.CJK {
			parts = append(parts, seg.Text)
		} else if g := Bigrams(seg.Text); g != "" {
			parts = append(parts, g)
		}
	}
	return strings.Join(parts, " ")
}
