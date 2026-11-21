package schema

import (
	"strings"
	"testing"
)

func TestValidateXML(t *testing.T) {
	tests := []struct {
		name       string
		xml        string
		wantErrors int
	}{
		{
			name: "valid document",
			xml: `<document version="1.0"><meta><title>Daily</title><type>report</type>` +
				`<date>2026-11-20</date><summary>Summary</summary>` +
				`<source-agent>agent</source-agent></meta><body/></document>`,
		},
		{
			name:       "invalid date and type",
			xml:        `<document version="1.0"><meta><title>T</title><type>note</type><date>20-11-2026</date><summary>S</summary><source-agent>A</source-agent></meta><body/></document>`,
			wantErrors: 2,
		},
		{
			name:       "malformed XML",
			xml:        `<document>`,
			wantErrors: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			issues := ValidateXML([]byte(test.xml))
			if len(issues) != test.wantErrors {
				t.Fatalf("ValidateXML() returned %d errors, want %d: %#v", len(issues), test.wantErrors, issues)
			}
		})
	}
}

func TestExtractPlainText(t *testing.T) {
	got, err := ExtractPlainText(strings.NewReader(`<body><md><![CDATA[hello world]]></md></body>`))
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello world" {
		t.Fatalf("ExtractPlainText() = %q, want %q", got, "hello world")
	}
}
