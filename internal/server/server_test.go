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
	writeImageZIP(t, filepath.Join(dir, "comic.cbz"))
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
	if !strings.Contains(body, "Series") || !strings.Contains(body, "<title>EPUB Title.epub</title>") || !strings.Contains(body, "Jane Doe") ||
		!strings.Contains(body, "<title>images.cbz</title>") ||
		!strings.Contains(body, `xmlns:dc="http://purl.org/dc/elements/1.1/"`) ||
		!strings.Contains(body, `xmlns:dcterms="http://purl.org/dc/terms/"`) ||
		!strings.Contains(body, "<dc:format>application/vnd.comicbook+zip</dc:format>") ||
		!strings.Contains(body, "<dcterms:extent>1 page</dcterms:extent>") ||
		strings.Contains(body, "<content>") ||
		!strings.Contains(body, `href="/download/main/novel.epub" type="application/epub+zip" title="novel.epub" length="`) ||
		!strings.Contains(body, `href="/download/main/images.zip" type="application/vnd.comicbook+zip" title="images.cbz" length="`) {
		t.Fatalf("library: %s", body)
	}
	w = request(t, a, "/opds/main?path=Series", true)
	if !strings.Contains(w.Body.String(), "book.pdf") || !strings.Contains(w.Body.String(), "/download/main/Series/book.pdf") {
		t.Fatalf("folder: %s", w.Body.String())
	}
}

func TestPublicationExtentUsesImagePageCount(t *testing.T) {
	tests := []struct {
		name     string
		metadata Metadata
		want     string
	}{
		{name: "EPUBには出力しない", metadata: Metadata{MediaType: "application/epub+zip", Size: 123}, want: ""},
		{name: "0ページは出力しない", metadata: Metadata{MediaType: "application/vnd.comicbook+zip", ImageCount: 0}, want: ""},
		{name: "1ページ", metadata: Metadata{MediaType: "application/vnd.comicbook+zip", ImageCount: 1}, want: "1 page"},
		{name: "複数ページ", metadata: Metadata{MediaType: "application/vnd.comicbook+zip", ImageCount: 12}, want: "12 pages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publicationExtent(tt.metadata); got != tt.want {
				t.Fatalf("publication extent: got=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestPublicationTitleUsesDeliveredExtension(t *testing.T) {
	tests := []struct {
		name  string
		title string
		path  string
		want  string
	}{
		{name: "EPUB", title: "作品名", path: "book.epub", want: "作品名.epub"},
		{name: "PDF", title: "資料", path: "book.PDF", want: "資料.pdf"},
		{name: "CBZ", title: "漫画", path: "book.cbz", want: "漫画.cbz"},
		{name: "ZIPはCBZ", title: "画像集", path: "book.zip", want: "画像集.cbz"},
		{name: "既存拡張子", title: "作品名.EPUB", path: "book.epub", want: "作品名.EPUB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publicationTitle(tt.title, tt.path); got != tt.want {
				t.Fatalf("publication title: got=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestDownloadAndTraversal(t *testing.T) {
	a, root := testApp(t)
	w := request(t, a, "/download/main/Series/book.pdf", true)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("download: %d %v", w.Code, w.Header())
	}
	if got := w.Header().Get("Content-Disposition"); got != `attachment; filename="download.pdf"; filename*=UTF-8''book.pdf` {
		t.Fatalf("pdf content disposition: %q", got)
	}
	w = request(t, a, "/download/main/novel.epub", true)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/epub+zip" {
		t.Fatalf("epub download: %d %v", w.Code, w.Header())
	}
	if got := w.Header().Get("Content-Disposition"); got != `attachment; filename="download.epub"; filename*=UTF-8''novel.epub` {
		t.Fatalf("epub content disposition: %q", got)
	}
	w = request(t, a, "/download/main/images.zip", true)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/vnd.comicbook+zip" {
		t.Fatalf("image zip download: %d %v", w.Code, w.Header())
	}
	if got := w.Header().Get("Content-Disposition"); got != `attachment; filename="download.cbz"; filename*=UTF-8''images.cbz` {
		t.Fatalf("image zip content disposition: %q", got)
	}
	w = request(t, a, "/download/main/comic.cbz", true)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/vnd.comicbook+zip" {
		t.Fatalf("cbz download: %d %v", w.Code, w.Header())
	}
	if got := w.Header().Get("Content-Disposition"); got != `attachment; filename="download.cbz"; filename*=UTF-8''comic.cbz` {
		t.Fatalf("cbz content disposition: %q", got)
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

func TestContentDispositionKeepsUTF8Filename(t *testing.T) {
	got := contentDisposition(filepath.Join(t.TempDir(), "日本 語.epub"))
	want := `attachment; filename="download.epub"; filename*=UTF-8''%E6%97%A5%E6%9C%AC%20%E8%AA%9E.epub`
	if got != want {
		t.Fatalf("content disposition: got=%q want=%q", got, want)
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
