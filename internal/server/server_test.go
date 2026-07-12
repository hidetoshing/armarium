package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	writeImageZIP(t, filepath.Join(dir, "images.zip"))
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
	if !strings.Contains(body, "Series") || !strings.Contains(body, "EPUB Title") || !strings.Contains(body, "Jane Doe") ||
		!strings.Contains(body, `href="/download/main/images.zip" type="application/vnd.comicbook+zip"`) {
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
	w = request(t, a, "/download/main/images.zip", true)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/vnd.comicbook+zip" {
		t.Fatalf("image zip download: %d %v", w.Code, w.Header())
	}
	if got := w.Header().Get("Content-Disposition"); got != "attachment; filename*=UTF-8''images.cbz" {
		t.Fatalf("image zip content disposition: %q", got)
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

func TestDirectoryChangeRefreshesDirectChildren(t *testing.T) {
	a, root := testApp(t)
	series := filepath.Join(root, "Series")
	_ = request(t, a, "/opds/main?path=Series", true)

	oldPath := filepath.Join(series, "book.pdf")
	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(series, "new.pdf")
	if err := os.WriteFile(newPath, []byte("%PDF-1.7"), 0o644); err != nil {
		t.Fatal(err)
	}
	forceDirectoryTimestamp(t, series)

	w := request(t, a, "/opds/main?path=Series", true)
	if strings.Contains(w.Body.String(), "book.pdf") || !strings.Contains(w.Body.String(), "new.pdf") {
		t.Fatalf("folder: %s", w.Body.String())
	}
	if _, ok := a.cache.entries[oldPath]; ok {
		t.Fatal("削除済みファイルのキャッシュが残っています")
	}
	if _, ok := a.cache.entries[newPath]; !ok {
		t.Fatal("新規ファイルがキャッシュされていません")
	}
}

func TestDirectoryChangeKeepsDescendantCache(t *testing.T) {
	a, root := testApp(t)
	series := filepath.Join(root, "Series")
	subdir := filepath.Join(series, "Sub")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	nestedPath := filepath.Join(subdir, "nested.pdf")
	if err := os.WriteFile(nestedPath, []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = request(t, a, "/opds/main?path=Series%2FSub", true)
	if _, ok := a.cache.entries[nestedPath]; !ok {
		t.Fatal("子ディレクトリの書籍がキャッシュされていません")
	}

	if err := os.WriteFile(filepath.Join(series, "added.pdf"), []byte("%PDF-1.7"), 0o644); err != nil {
		t.Fatal(err)
	}
	forceDirectoryTimestamp(t, series)
	_ = request(t, a, "/opds/main?path=Series", true)
	if _, ok := a.cache.entries[nestedPath]; !ok {
		t.Fatal("直下の差分更新で子ディレクトリのキャッシュが削除されました")
	}
}

func TestMetadataCacheLoadsVersion1AndMigrates(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(t.TempDir(), "cache.json")
	bookPath := filepath.Join(dir, "book.pdf")
	if err := os.WriteFile(bookPath, []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(bookPath)
	if err != nil {
		t.Fatal(err)
	}
	old := map[string]cacheEntry{bookPath: {Size: info.Size(), ModUnix: info.ModTime().UnixNano(), Metadata: extractMetadata(bookPath, info)}}
	b, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, b, 0o600); err != nil {
		t.Fatal(err)
	}

	a := New(Config{CachePath: cachePath, Users: []User{{Username: "reader", Password: "secret"}}, Libraries: []Library{{ID: "main", Name: "Main", Path: dir}}})
	_ = request(t, a, "/opds/main", true)
	b, err = os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	var stored cacheFile
	if err := json.Unmarshal(b, &stored); err != nil || stored.Version != cacheVersion || stored.Entries[bookPath].Metadata.Title != "book" {
		t.Fatalf("移行後のキャッシュが不正です: err=%v cache=%+v", err, stored)
	}
}

func TestOldMetadataCacheDoesNotKeepZIPMediaType(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(t.TempDir(), "cache.json")
	bookPath := filepath.Join(dir, "images.zip")
	writeImageZIP(t, bookPath)
	info, err := os.Stat(bookPath)
	if err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	oldMetadata := extractMetadata(bookPath, info)
	oldMetadata.MediaType = "application/zip"
	old := cacheFile{
		Version:     cacheVersion - 1,
		Directories: map[string]int64{dir: dirInfo.ModTime().UnixNano()},
		Entries:     map[string]cacheEntry{bookPath: {Size: info.Size(), ModUnix: info.ModTime().UnixNano(), Metadata: oldMetadata}},
	}
	b, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, b, 0o600); err != nil {
		t.Fatal(err)
	}

	a := New(Config{CachePath: cachePath, Users: []User{{Username: "reader", Password: "secret"}}, Libraries: []Library{{ID: "main", Name: "Main", Path: dir}}})
	w := request(t, a, "/opds/main", true)
	if !strings.Contains(w.Body.String(), `href="/download/main/images.zip" type="application/vnd.comicbook+zip"`) {
		t.Fatalf("古いキャッシュのMIMEタイプが残っています: %s", w.Body.String())
	}
}

func forceDirectoryTimestamp(t *testing.T, path string) {
	t.Helper()
	stamp := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
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

func writeImageZIP(t *testing.T, path string) {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, err := z.Create("001.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("image")); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
