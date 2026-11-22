package api

import (
	"slices"
	"strings"
	"unicode"
)

const (
	// Rune counts kept before the first hit and from its start, so a snippet is about 60 runes.
	snippetBefore = 20
	snippetAfter  = 40
)

// originalSnippet builds a snippet of the first text in texts that contains one of terms. items_fts_cjk
// stores bigram-expanded text, so FTS5's snippet() would show the expansion; this locates the terms in
// the original text instead and marks them with markStart/markEnd. Matching ignores case, like the
// trigram index. It returns "" when no text contains a term.
func originalSnippet(texts, terms []string) string {
	for _, text := range texts {
		if s := snippetOf([]rune(text), terms); s != "" {
			return s
		}
	}
	return ""
}

func snippetOf(text []rune, terms []string) string {
	folded := foldRunes(text)
	type span struct{ start, end int }
	var spans []span
	for _, term := range terms {
		needle := foldRunes([]rune(term))
		for from := 0; len(needle) > 0 && from+len(needle) <= len(folded); {
			i := runeIndex(folded[from:], needle)
			if i < 0 {
				break
			}
			spans = append(spans, span{from + i, from + i + len(needle)})
			from += i + len(needle)
		}
	}
	if len(spans) == 0 {
		return ""
	}
	slices.SortFunc(spans, func(a, b span) int { return a.start - b.start })
	merged := spans[:1]
	for _, sp := range spans[1:] {
		if last := &merged[len(merged)-1]; sp.start <= last.end {
			last.end = max(last.end, sp.end)
		} else {
			merged = append(merged, sp)
		}
	}
	spans = merged
	lo := max(0, spans[0].start-snippetBefore)
	hi := min(len(text), spans[0].start+snippetAfter)

	var b strings.Builder
	if lo > 0 {
		b.WriteString("…")
	}
	pos := lo
	for _, sp := range spans {
		start, end := max(sp.start, pos), min(sp.end, hi)
		if start >= end {
			continue
		}
		b.WriteString(string(text[pos:start]))
		b.WriteString(markStart)
		b.WriteString(string(text[start:end]))
		b.WriteString(markEnd)
		pos = end
	}
	b.WriteString(string(text[pos:hi]))
	if hi < len(text) {
		b.WriteString("…")
	}
	// Collapse newlines and runs of spaces in the original text.
	return strings.Join(strings.Fields(b.String()), " ")
}

// foldRunes lowercases rune by rune, so indexes stay aligned with the original text.
func foldRunes(in []rune) []rune {
	out := make([]rune, len(in))
	for i, r := range in {
		out[i] = unicode.ToLower(r)
	}
	return out
}

// runeIndex returns the index of the first occurrence of needle in hay, or -1.
func runeIndex(hay, needle []rune) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if slices.Equal(hay[i:i+len(needle)], needle) {
			return i
		}
	}
	return -1
}
