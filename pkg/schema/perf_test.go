package schema

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/helium"
	"github.com/lestrrat-go/helium/schematron"
)

// TestProtocolPerfNear10MB benchmarks our split (Schematron + Go DOM walk) against a
// Schematron ancestor::section rule on an ~9MB v1.1 document (PRD 6.9).
func TestProtocolPerfNear10MB(t *testing.T) {
	payload := strings.Repeat("x", 1500)
	var b strings.Builder
	b.Grow(9 * 1024 * 1024)
	b.WriteString(`<document version="1.1">` + metaBlock + `<body>`)
	for i := 0; b.Len() < 8_500_000; i++ {
		fmt.Fprintf(&b, `<section id="sec-%d" title="S %d"><section title="Inner"><claim level="evidence">%s</claim><compare-table><columns><col>A</col><col>B</col></columns><row><cell tag="evidence">1</cell><cell tag="approx">2</cell></row></compare-table></section></section>`, i, i, payload)
	}
	b.WriteString(`</body></document>`)
	raw := []byte(b.String())
	if len(raw) < 8*1024*1024 {
		t.Fatalf("expected >8MB fixture, got %d bytes", len(raw))
	}

	// Ensure compiled before timing.
	_ = protocol()
	_ = chartSchemas()

	tParseStart := time.Now()
	tree, fatal := parseTree(raw)
	if fatal != nil {
		t.Fatal(fatal)
	}
	tParse := time.Since(tParseStart)

	tSchStart := time.Now()
	schIssues := protocolIssues(tree)
	tSch := time.Since(tSchStart)

	d := &Document{Version: "1.1", input: raw, tree: tree}
	tGoStart := time.Now()
	v11Issues := d.checkV11()
	tGo := time.Since(tGoStart)

	if len(schIssues) != 0 || len(v11Issues) != 0 {
		t.Fatalf("unexpected issues: sch=%v v11=%v", schIssues, v11Issues)
	}

	// Compare with Schematron ancestor::section predicate on the same tree.
	ancestorSch := `<?xml version="1.0" encoding="UTF-8"?>
<schema xmlns="http://purl.oclc.org/dsdl/schematron" queryBinding="xpath3">
  <pattern id="depth">
    <rule context="/document[@version='1.1']/body//section">
      <assert test="count(ancestor::section) le 3">SECTION_TOO_DEEP|section|too deep</assert>
    </rule>
  </pattern>
</schema>`
	schDoc, err := helium.NewParser().Parse(context.Background(), []byte(ancestorSch))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := schematron.NewCompiler().Compile(context.Background(), schDoc)
	if err != nil {
		t.Fatal(err)
	}
	tAncStart := time.Now()
	_ = schematron.NewValidator(compiled).Validate(context.Background(), tree)
	tAnc := time.Since(tAncStart)

	t.Logf("docSize=%.2fMB xmlParse=%s schematron=%s goV11=%s schematronAncestorOnly=%s",
		float64(len(raw))/(1024*1024), tParse, tSch, tGo, tAnc)
}
