package server

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Series"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Series", "book.pdf"), []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeEPUB(t, filepath.Join(dir, "novel.epub"))
	a := New(Config{CachePath: filepath.Join(t.TempDir(), "cache.json"), Users: []User{{Username: "reader", Password: "secret"}}, Libraries: []Library{{ID: "main", Name: "Main", Path: dir}}})
	return a, dir
}

func request(t *testing.T, a *App, path string, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if auth {
		r.SetBasicAuth("reader", "secret")
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}

func TestAuthentication(t *testing.T) {
	a, _ := testApp(t)
	w := request(t, a, "/opds", false)
	if w.Code != http.StatusUnauthorized || w.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("status=%d headers=%v", w.Code, w.Header())
	}
}

func TestOPDSRootAndFolder(t *testing.T) {
	a, _ := testApp(t)
	w := request(t, a, "/opds", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "/opds/main") {
		t.Fatalf("root: %d %s", w.Code, w.Body.String())
	}
	w = request(t, a, "/opds/main", true)
	body := w.Body.String()
	if !strings.Contains(body, "Series") || !strings.Contains(body, "EPUB Title") || !strings.Contains(body, "Jane Doe") {
		t.Fatalf("library: %s", body)
	}
	w = request(t, a, "/opds/main?path=Series", true)
	if !strings.Contains(w.Body.String(), "book.pdf") || !strings.Contains(w.Body.String(), "/download/main/Series/book.pdf") {
		t.Fatalf("folder: %s", w.Body.String())
	}
}

func TestDownloadAndTraversal(t *testing.T) {
	a, root := testApp(t)
	w := request(t, a, "/download/main/Series/book.pdf", true)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("download: %d %v", w.Code, w.Header())
	}
	w = request(t, a, "/opds/main?path=..", true)
	if w.Code != 400 {
		t.Fatalf("traversal status=%d", w.Code)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.pdf"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	w = request(t, a, "/download/main/outside/secret.pdf", true)
	if w.Code != 400 {
		t.Fatalf("symlink escape status=%d", w.Code)
	}
}

func TestMetadataCachePersists(t *testing.T) {
	a, _ := testApp(t)
	_ = request(t, a, "/opds/main", true)
	if _, err := os.Stat(a.config.CachePath); err != nil {
		t.Fatal(err)
	}
}

func writeEPUB(t *testing.T, path string) {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	files := map[string]string{"META-INF/container.xml": `<container><rootfiles><rootfile full-path="content.opf"/></rootfiles></container>`, "content.opf": `<package><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>EPUB Title</dc:title><dc:creator>Jane Doe</dc:creator></metadata></package>`}
	for name, body := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(w, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
