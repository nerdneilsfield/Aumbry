package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nerdneilsfield/aumbry/internal/cliconfig"
	"github.com/nerdneilsfield/aumbry/pkg/schema"
)

// runCLI executes the root command with an isolated config directory and clean AUMBRY_* env.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func isolate(t *testing.T) string {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	for _, name := range []string{cliconfig.EnvServer, cliconfig.EnvToken, cliconfig.EnvDirs} {
		t.Setenv(name, "")
	}
	return xdg
}

func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runCLI(t, args...)
	if err != nil {
		t.Fatalf("aumbry %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func TestConfigSetShowUse(t *testing.T) {
	xdg := isolate(t)
	mustRun(t, "config", "set", "--server", "https://a.example", "--token", "amb_live_secret1234", "--dir", "D1", "--dir", "D2")
	mustRun(t, "config", "set", "--profile", "other", "--server", "https://b.example")
	// Only the given flags change; --dir replaces the list.
	mustRun(t, "config", "set", "--dir", "D3")

	path := filepath.Join(xdg, "aumbry", "config.yaml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("config mode = %o, want 600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("config dir mode = %o, want 700", dirInfo.Mode().Perm())
	}

	out := mustRun(t, "config", "show")
	for _, want := range []string{"profile: default", "server: https://a.example", "token: '****1234'", "- D3"} {
		if !strings.Contains(out, want) && !strings.Contains(out, strings.ReplaceAll(want, "'", "")) {
			t.Errorf("show output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret") || strings.Contains(out, "D1") {
		t.Errorf("show leaked the token or kept replaced dirs:\n%s", out)
	}

	mustRun(t, "config", "use", "other")
	if out := mustRun(t, "config", "show"); !strings.Contains(out, "profile: other") || !strings.Contains(out, "b.example") {
		t.Errorf("use did not switch profile:\n%s", out)
	}
	if _, err := runCLI(t, "config", "use", "nope"); err == nil {
		t.Error("use of an unknown profile must fail")
	}
	if _, err := runCLI(t, "config", "show", "--profile", "nope"); err == nil {
		t.Error("show of an unknown profile must fail")
	}
}

func TestConfigPrecedence(t *testing.T) {
	isolate(t)
	mustRun(t, "config", "set", "--server", "https://file.example", "--token", "file-token-0001", "--dir", "FILE")
	mustRun(t, "config", "set", "--profile", "alt", "--server", "https://alt.example", "--token", "alt-token-0002")

	file, err := cliconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	// file only
	name, p, err := file.Resolve("")
	if err != nil || name != "default" || p.Server != "https://file.example" || p.DefaultDirs[0] != "FILE" {
		t.Fatalf("file layer: %s %+v %v", name, p, err)
	}
	// env beats file
	t.Setenv(cliconfig.EnvServer, "https://env.example")
	t.Setenv(cliconfig.EnvToken, "env-token-0003")
	t.Setenv(cliconfig.EnvDirs, "E1, E2,,")
	_, p, err = file.Resolve("")
	if err != nil || p.Server != "https://env.example" || p.Token != "env-token-0003" || strings.Join(p.DefaultDirs, "|") != "E1|E2" {
		t.Fatalf("env layer: %+v %v", p, err)
	}
	// --profile flag beats active_profile (env still applies on top of the chosen profile)
	name, p, err = file.Resolve("alt")
	if err != nil || name != "alt" || p.Server != "https://env.example" {
		t.Fatalf("flag layer: %s %+v %v", name, p, err)
	}
	t.Setenv(cliconfig.EnvServer, "")
	if _, p, _ = file.Resolve("alt"); p.Server != "https://alt.example" {
		t.Fatalf("flag profile without env: %+v", p)
	}
	// show reflects the env override through the command
	t.Setenv(cliconfig.EnvServer, "https://env.example")
	if out := mustRun(t, "config", "show"); !strings.Contains(out, "env.example") {
		t.Errorf("show ignored env override:\n%s", out)
	}
}

func TestMaskToken(t *testing.T) {
	for token, want := range map[string]string{"": "", "abc": "****", "abcd": "****", "amb_live_xyz12345": "****2345"} {
		if got := cliconfig.MaskToken(token); got != want {
			t.Errorf("MaskToken(%q) = %q, want %q", token, got, want)
		}
	}
}

func TestValidateExit(t *testing.T) {
	isolate(t)
	out, err := runCLI(t, "validate", "../examples/full-report.xml")
	if err != nil || !strings.Contains(out, `"valid": true`) {
		t.Fatalf("valid doc: err=%v out=%s", err, out)
	}

	bad := filepath.Join(t.TempDir(), "bad.xml")
	if err = os.WriteFile(bad, []byte(`<report></report>`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = runCLI(t, "validate", bad)
	var failed *ValidationFailedError
	if !errors.As(err, &failed) {
		t.Fatalf("want ValidationFailedError, got %v", err)
	}
	if !strings.Contains(out, `"valid": false`) || !strings.Contains(out, `"errors"`) {
		t.Errorf("invalid JSON not on stdout: %s", out)
	}
	out, err = runCLI(t, "validate", bad, "--format", "text")
	if !errors.As(err, &failed) || strings.Contains(out, "{") {
		t.Errorf("text format: err=%v out=%q", err, out)
	}
}

func TestValidateHintAndSchemaCommand(t *testing.T) {
	isolate(t)
	dir := t.TempDir()

	// #44: INVALID_XML with bare & inside <md> prints CDATA hint in both json and text output.
	bareAmp := filepath.Join(dir, "amp.xml")
	xmlAmp := `<document version="1.1"><meta><title>T</title><type>report</type><date>2026-11-20</date><summary>S</summary><source-agent>a</source-agent></meta><body><md>A & B</md></body></document>`
	if err := os.WriteFile(bareAmp, []byte(xmlAmp), 0o600); err != nil {
		t.Fatal(err)
	}
	var failed *ValidationFailedError
	outJSON, err := runCLI(t, "validate", bareAmp, "--format", "json")
	if !errors.As(err, &failed) || !strings.Contains(outJSON, `"hint"`) || !strings.Contains(outJSON, "CDATA") {
		t.Fatalf("json hint missing: err=%v out=%s", err, outJSON)
	}
	outText, err := runCLI(t, "validate", bareAmp, "--format", "text")
	if !errors.As(err, &failed) || !strings.Contains(outText, "hint:") || !strings.Contains(outText, "CDATA") {
		t.Fatalf("text hint missing: err=%v out=%s", err, outText)
	}

	// Versions outside 1.1/1.2 exit via ValidationFailedError with UNSUPPORTED_VERSION.
	oldVersion := filepath.Join(dir, "version10.xml")
	xmlOld := `<document version="1.0"><meta><title>T</title><type>report</type><date>2026-11-20</date><summary>S</summary><source-agent>a</source-agent></meta><body><md>x</md></body></document>`
	if err = os.WriteFile(oldVersion, []byte(xmlOld), 0o600); err != nil {
		t.Fatal(err)
	}
	outOld, err := runCLI(t, "validate", oldVersion)
	if !errors.As(err, &failed) || !strings.Contains(outOld, `"UNSUPPORTED_VERSION"`) {
		t.Fatalf("version 1.0 validate: err=%v out=%s", err, outOld)
	}

	// #25 + #69: aumbry schema --format json and md.
	jsonOut := mustRun(t, "schema", "--format", "json")
	var spec schema.ProtocolSpec
	if err := json.Unmarshal([]byte(jsonOut), &spec); err != nil {
		t.Fatalf("unmarshal schema json: %v\n%s", err, jsonOut)
	}
	if spec.Version != "1.2" {
		t.Fatalf("schema json version = %q, want 1.2", spec.Version)
	}
	if len(spec.Elements) == 0 || len(spec.ErrorCodes) == 0 {
		t.Fatalf("schema json missing elements/error_codes: %+v", spec)
	}

	mdOut := mustRun(t, "schema")
	if !strings.Contains(mdOut, "<tldr>") {
		t.Fatalf("schema md unexpected output:\n%s", mdOut)
	}

	if _, err := runCLI(t, "schema", "--format", "xml"); err == nil {
		t.Fatal("expected unsupported schema format error")
	}
}

func TestV2M2ValidateAndPushRemoteAndPDF(t *testing.T) {
	t.Run("validate rejects loopback remote figure offline with REMOTE_ASSET_BLOCKED", func(t *testing.T) {
		isolate(t)
		docPath := filepath.Join(t.TempDir(), "ssrf.xml")
		xml := `<?xml version="1.0" encoding="UTF-8"?><document version="1.2">
<meta><title>SSRF</title><type>post</type><date>2026-11-23</date><summary>S</summary><source-agent>a</source-agent></meta>
<body><figure src="http://127.0.0.1:18080/a.png" caption="bad"/></body></document>`
		if err := os.WriteFile(docPath, []byte(xml), 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := runCLI(t, "validate", docPath)
		var failed *ValidationFailedError
		if !errors.As(err, &failed) || !strings.Contains(out, `"REMOTE_ASSET_BLOCKED"`) {
			t.Fatalf("offline validate SSRF = err=%v out=%s", err, out)
		}
	})

	t.Run("push skips remote figure URL upload while uploading local PDF attachment", func(t *testing.T) {
		isolate(t)
		dir := t.TempDir()
		pdfPath := filepath.Join(dir, "paper.pdf")
		if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n1 0 obj<<>>endobj\n%%EOF\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		xmlPath := filepath.Join(dir, "doc.xml")
		xml := `<?xml version="1.0" encoding="UTF-8"?><document version="1.2">
<meta><title>RemoteAndPDF</title><type>post</type><date>2026-11-23</date><summary>S</summary><source-agent>a</source-agent></meta>
<body>
  <figure src="https://example.org/pic.png" caption="remote"/>
  <attachment id="p" type="pdf" src="paper.pdf" title="Paper"/>
</body></document>`
		if err := os.WriteFile(xmlPath, []byte(xml), 0o600); err != nil {
			t.Fatal(err)
		}

		server, got := captureIngest(t, http.StatusCreated, `{"id":"m2cli0000001","type":"post","url":"/p/m2cli0000001","dirs":["D1"],"assets":2,"created":true}`)
		t.Setenv(cliconfig.EnvServer, server.URL)
		t.Setenv(cliconfig.EnvToken, "tok-1234")

		out := mustRun(t, "push", xmlPath, "--dir", "D1")
		if !strings.Contains(out, `"m2cli0000001"`) {
			t.Fatalf("unexpected push output:\n%s", out)
		}
		if len(got.files["assets[]"]) != 1 || got.files["assets[]"][0] != "paper.pdf" {
			t.Fatalf("uploaded asset files = %v, want [paper.pdf]", got.files["assets[]"])
		}
		if len(got.fields["asset_srcs"]) != 1 || got.fields["asset_srcs"][0] != "paper.pdf" {
			t.Fatalf("uploaded asset_srcs = %v, want [paper.pdf]", got.fields["asset_srcs"])
		}
		if !strings.Contains(string(got.bodies["xml"]), `https://example.org/pic.png`) {
			t.Fatalf("uploaded XML dropped remote URL:\n%s", got.bodies["xml"])
		}
	})
}
