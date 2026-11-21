package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var output bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&output)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.HasPrefix(output.String(), "aumbry dev\n") {
		t.Fatalf("unexpected output: %q", output.String())
	}
}
