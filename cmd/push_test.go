package cmd

import (
	"bytes"
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

	"github.com/nerdneilsfield/aumbry/internal/client"
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

func TestPushV11FlagsAndPreflight(t *testing.T) {
	t.Run("flag conflicts fail without touching network", func(t *testing.T) {
		isolate(t)
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("network must not be contacted on flag conflict")
		}))
		defer srv.Close()
		t.Setenv("AUMBRY_SERVER", srv.URL)
		t.Setenv("AUMBRY_TOKEN", "tok")

		if _, err := runCLI(t, "push", "../examples/minimal-report.xml", "--draft", "--dir", "D1"); err == nil || !strings.Contains(err.Error(), "--draft") {
			t.Errorf("--draft --dir: got err=%v", err)
		}
		if _, err := runCLI(t, "push", "../examples/minimal-report.xml", "--create-only", "--id", "ITEM1", "--key", "k"); err == nil || !strings.Contains(err.Error(), "--create-only") {
			t.Errorf("--create-only --id: got err=%v", err)
		}
		if _, err := runCLI(t, "push", "../examples/minimal-report.xml", "--create-only"); err == nil || !strings.Contains(err.Error(), "--key") {
			t.Errorf("--create-only without --key: got err=%v", err)
		}
	})

	t.Run("draft ignores default_dirs and preflights agent/dirs against v1.0 404", func(t *testing.T) {
		isolate(t)
		v10 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/agent/dirs" {
				t.Fatalf("unexpected v1.0 call to %s before preflight failure", r.URL.Path)
			}
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		}))
		defer v10.Close()
		mustRun(t, "config", "set", "--server", v10.URL, "--token", "tok", "--dir", "DEF1")
		if _, err := runCLI(t, "push", "../examples/minimal-report.xml", "--draft"); err == nil || !strings.Contains(err.Error(), "404") {
			t.Fatalf("expected 404 preflight error on v1.0 server, got %v", err)
		}
		if _, err := runCLI(t, "push", "../examples/minimal-report.xml", "--create-only", "--key", "k:1"); err == nil || !strings.Contains(err.Error(), "404") {
			t.Fatalf("expected 404 preflight error for create-only, got %v", err)
		}
	})

	t.Run("draft sends draft=1 and full_url while ignoring AUMBRY_DIRS", func(t *testing.T) {
		isolate(t)
		var capturedDirs []string
		var draftVal string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if strings.HasSuffix(r.URL.Path, "/api/v1/agent/dirs") {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_ = r.ParseMultipartForm(1 << 20)
			if r.MultipartForm != nil {
				capturedDirs = r.MultipartForm.Value["dirs"]
			}
			draftVal = r.FormValue("draft")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"abcd12345678","type":"report","url":"/r/abcd12345678","dirs":[],"assets":0,"created":true,"draft":true,"published":false}`))
		}))
		defer srv.Close()
		mustRun(t, "config", "set", "--server", srv.URL+"/sub", "--token", "tok", "--dir", "DEFAULT_DIR")
		t.Setenv("AUMBRY_DIRS", "ENV_DIR")

		out := mustRun(t, "push", "../examples/minimal-report.xml", "--draft", "--key", "k:d")
		if draftVal != "1" || len(capturedDirs) != 0 {
			t.Fatalf("draft=%q dirs=%v, want draft=1 and no dirs", draftVal, capturedDirs)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatal(err)
		}
		if res["full_url"] != srv.URL+"/sub/r/abcd12345678" || res["draft"] != true || res["published"] != false {
			t.Fatalf("response = %v", res)
		}
	})

	t.Run("409 already_published and 422 UNSUPPORTED_VERSION hints", func(t *testing.T) {
		isolate(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/api/v1/agent/dirs" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"already_published","message":"published"}`))
		}))
		defer srv.Close()
		t.Setenv("AUMBRY_SERVER", srv.URL)
		t.Setenv("AUMBRY_TOKEN", "tok")
		if _, err := runCLI(t, "push", "../examples/minimal-report.xml", "--draft", "--key", "k:pub"); err == nil || !strings.Contains(err.Error(), "unpublish") {
			t.Fatalf("already_published error = %v", err)
		}

		v10Reject := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"valid":false,"errors":[{"code":"UNSUPPORTED_VERSION","field":"document.version","message":"version must be 1.0"}]}`))
		}))
		defer v10Reject.Close()
		t.Setenv("AUMBRY_SERVER", v10Reject.URL)

		var outBuf, errBuf bytes.Buffer
		root := NewRootCmd()
		root.SetOut(&outBuf)
		root.SetErr(&errBuf)
		root.SetArgs([]string{"push", "../examples/minimal-report.xml"})
		err := root.Execute()
		var failed *ValidationFailedError
		if !errors.As(err, &failed) || !strings.Contains(errBuf.String(), "upgrade") {
			t.Fatalf("err=%v stderr=%q stdout=%q", err, errBuf.String(), outBuf.String())
		}
	})
}

func TestPushRejectsEscapingSymlinkAtValidationAndOpen(t *testing.T) {
	isolate(t)
	base := t.TempDir()
	outside := filepath.Join(base, "secret.png")
	if err := os.WriteFile(outside, []byte("secret-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	docDir := filepath.Join(base, "doc")
	imgDir := filepath.Join(docDir, "imgs")
	if err := os.MkdirAll(imgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	leakPath := filepath.Join(imgDir, "leak.png")
	if err := os.Symlink(outside, leakPath); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	xmlFile := filepath.Join(docDir, "doc.xml")
	xmlBody := `<document version="1.1"><meta><title>T</title><type>report</type><date>2026-11-20</date><summary>S</summary><source-agent>a</source-agent></meta><body><figure src="./imgs/leak.png"/></body></document>`
	if err := os.WriteFile(xmlFile, []byte(xmlBody), 0o600); err != nil {
		t.Fatal(err)
	}

	called := false
	var seenBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		data, _ := io.ReadAll(r.Body)
		seenBody += string(data)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("AUMBRY_SERVER", srv.URL)
	t.Setenv("AUMBRY_TOKEN", "tok")

	out, err := runCLI(t, "push", xmlFile)
	var failed *ValidationFailedError
	if !errors.As(err, &failed) || !strings.Contains(out, "MISSING_ASSET") || called {
		t.Fatalf("push accepted symlink escape or hit network: called=%v err=%v out=%s", called, err, out)
	}

	// Simulate post-validation TOCTOU symlink replacement directly at client.Push upload time.
	_, pushErr := client.Push(t.Context(), client.PushRequest{
		Server:   srv.URL,
		Token:    "tok",
		XML:      []byte(xmlBody),
		AssetDir: docDir,
		Assets:   []client.Asset{{Src: "imgs/leak.png", Path: leakPath}},
	})
	if pushErr == nil || strings.Contains(seenBody, "secret-bytes") {
		t.Fatalf("client.Push uploaded escaping symlink: err=%v leaked=%v", pushErr, strings.Contains(seenBody, "secret-bytes"))
	}
}
