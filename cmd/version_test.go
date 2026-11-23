package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nerdneilsfield/aumbry/internal/buildinfo"
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

func TestVersionCommandMetadataAndJSON(t *testing.T) {
	origVer, origCommit, origDate := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.Date = origVer, origCommit, origDate
	})

	buildinfo.Version = "0.2.0"
	buildinfo.Commit = "5ba6832d3d90301d70dce14eabd75256fb52b5a4"
	buildinfo.Date = "2026-11-23T12:18:00Z"

	var textBuf bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&textBuf)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("text execute: %v", err)
	}
	if !strings.Contains(textBuf.String(), "aumbry 0.2.0\ncommit: 5ba6832\nbuilt: 2026-11-23T12:18:00Z\n") {
		t.Fatalf("unexpected text output: %q", textBuf.String())
	}

	var jsonBuf bytes.Buffer
	rootJSON := NewRootCmd()
	rootJSON.SetOut(&jsonBuf)
	rootJSON.SetArgs([]string{"version", "--json"})
	if err := rootJSON.Execute(); err != nil {
		t.Fatalf("json execute: %v", err)
	}
	var got buildinfo.Info
	if err := json.Unmarshal(jsonBuf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal json: %v (%s)", err, jsonBuf.String())
	}
	if got.Version != "0.2.0" || got.Commit != "5ba6832" || got.BuildTime != "2026-11-23T12:18:00Z" {
		t.Fatalf("json output = %+v", got)
	}
}
