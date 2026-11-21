package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type captured struct {
	auth   string
	fields map[string][]string
	files  map[string][]string // field -> filenames
	bodies map[string][]byte   // field -> first file content
}

func captureIngest(t *testing.T, status int, response string) (*httptest.Server, *captured) {
	t.Helper()
	got := &captured{fields: map[string][]string{}, files: map[string][]string{}, bodies: map[string][]byte{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/ingest" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		got.auth = r.Header.Get("Authorization")
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/form-data" {
			t.Errorf("content type %q: %v", r.Header.Get("Content-Type"), err)
			return
		}
		reader := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("read part: %v", err)
				break
			}
			data, _ := io.ReadAll(part)
			if part.FileName() != "" {
				got.files[part.FormName()] = append(got.files[part.FormName()], part.FileName())
				if _, ok := got.bodies[part.FormName()]; !ok {
					got.bodies[part.FormName()] = data
				}
			} else {
				got.fields[part.FormName()] = append(got.fields[part.FormName()], string(data))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)
	return server, got
}

func TestPushSuccess(t *testing.T) {
	isolate(t)
	server, got := captureIngest(t, http.StatusCreated, `{"id":"abc","type":"report","url":"/r/abc","dirs":["D1"],"assets":1,"created":true}`)
	mustRun(t, "config", "set", "--server", server.URL+"/", "--token", "tok-1234", "--dir", "DEFAULT")

	out := mustRun(t, "push", "../examples/full-report.xml", "--dir", "D1", "--dir", "D2", "--key", "k:1", "--id", "ITEM")
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil || result["id"] != "abc" {
		t.Fatalf("stdout is not the server JSON: %v\n%s", err, out)
	}
	if !strings.Contains(out, "\n  \"id\"") {
		t.Errorf("response not indented:\n%s", out)
	}

	if got.auth != "Bearer tok-1234" {
		t.Errorf("Authorization = %q", got.auth)
	}
	if strings.Join(got.fields["dirs"], ",") != "D1,D2" {
		t.Errorf("dirs = %v (--dir must override default_dirs)", got.fields["dirs"])
	}
	if got.fields["external_key"][0] != "k:1" || got.fields["item_id"][0] != "ITEM" {
		t.Errorf("key/id fields: %v", got.fields)
	}
	want, err := os.ReadFile("../examples/full-report.xml")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.bodies["xml"]) != string(want) {
		t.Error("xml part differs from the source file")
	}
	srcs := got.fields["asset_srcs"]
	if len(srcs) != 1 || srcs[0] != "imgs/world_model_arch.svg" || len(got.files["assets[]"]) != 1 || got.files["assets[]"][0] != "world_model_arch.svg" {
		t.Errorf("asset_srcs=%v assets[]=%v", srcs, got.files["assets[]"])
	}
	svg, _ := os.ReadFile("../examples/imgs/world_model_arch.svg")
	if string(got.bodies["assets[]"]) != string(svg) {
		t.Error("asset content differs")
	}
}

func TestPushDefaultDirs(t *testing.T) {
	isolate(t)
	server, got := captureIngest(t, http.StatusOK, `{"id":"x"}`)
	mustRun(t, "config", "set", "--server", server.URL, "--token", "tok", "--dir", "A", "--dir", "B")
	mustRun(t, "push", "../examples/minimal-report.xml")
	if strings.Join(got.fields["dirs"], ",") != "A,B" {
		t.Errorf("dirs = %v", got.fields["dirs"])
	}
}

func TestPushServerRejections(t *testing.T) {
	isolate(t)
	server, _ := captureIngest(t, http.StatusUnprocessableEntity, `{"valid":false,"errors":[{"code":"X","message":"bad"}]}`)
	t.Setenv("AUMBRY_SERVER", server.URL)
	t.Setenv("AUMBRY_TOKEN", "tok")
	out, err := runCLI(t, "push", "../examples/minimal-report.xml")
	var failed *ValidationFailedError
	if !errors.As(err, &failed) || !strings.Contains(out, `"valid": false`) {
		t.Errorf("422: err=%v out=%s", err, out)
	}

	unauthorized, _ := captureIngest(t, http.StatusUnauthorized, `{"error":"unauthorized","message":"invalid token"}`)
	t.Setenv("AUMBRY_SERVER", unauthorized.URL)
	out, err = runCLI(t, "push", "../examples/minimal-report.xml")
	if err == nil || err.Error() != "invalid token (HTTP 401)" || out != "" {
		t.Errorf("401: err=%v out=%q", err, out)
	}
}

func TestPushLocalFailuresSkipNetwork(t *testing.T) {
	isolate(t)
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("network must not be used")
	}))
	defer server.Close()
	t.Setenv("AUMBRY_SERVER", server.URL)
	t.Setenv("AUMBRY_TOKEN", "tok")

	bad := filepath.Join(t.TempDir(), "bad.xml")
	if err := os.WriteFile(bad, []byte(`<report></report>`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, "push", bad)
	var failed *ValidationFailedError
	if !errors.As(err, &failed) || !strings.Contains(out, `"valid": false`) {
		t.Errorf("invalid doc: err=%v out=%s", err, out)
	}

	t.Setenv("AUMBRY_TOKEN", "")
	if _, err := runCLI(t, "push", "../examples/minimal-report.xml"); err == nil || !strings.Contains(err.Error(), "AUMBRY_TOKEN") {
		t.Errorf("missing token error should name the fix: %v", err)
	}
}
