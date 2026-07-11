package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type App struct {
	config  Config
	cache   *metadataCache
	handler http.Handler
}

func NewFromFile(path string) (*App, error) {
	c, err := loadConfig(path)
	if err != nil {
		return nil, err
	}
	return New(c), nil
}
func New(c Config) *App {
	a := &App{config: c, cache: newMetadataCache(c.CachePath)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", a.home)
	mux.HandleFunc("GET /opds", a.opdsRoot)
	mux.HandleFunc("GET /opds/{library}", a.opdsLibrary)
	mux.HandleFunc("GET /download/{library}/{path...}", a.download)
	a.handler = a.auth(mux)
	return a
}
func (a *App) Address() string       { return a.config.Listen }
func (a *App) ListenAndServe() error { return http.ListenAndServe(a.config.Listen, a.handler) }
func (a *App) Handler() http.Handler { return a.handler }

func (a *App) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		valid := false
		if ok {
			sum := sha256.Sum256([]byte(p))
			for _, user := range a.config.Users {
				if subtle.ConstantTimeCompare([]byte(u), []byte(user.Username)) != 1 {
					continue
				}
				expected := user.PasswordSHA256
				if expected == "" {
					s := sha256.Sum256([]byte(user.Password))
					expected = hex.EncodeToString(s[:])
				}
				if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(strings.ToLower(expected))) == 1 {
					valid = true
				}
			}
		}
		if !valid {
			w.Header().Set("WWW-Authenticate", `Basic realm="armarium", charset="UTF-8"`)
			http.Error(w, "認証が必要です", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><html lang="ja"><meta charset="utf-8"><title>Armarium</title><h1>Armarium</h1><p><a href="/opds">OPDSカタログ</a></p>`)
}

type feed struct {
	XMLName xml.Name `xml:"feed"`
	Xmlns   string   `xml:"xmlns,attr"`
	ID      string   `xml:"id"`
	Title   string   `xml:"title"`
	Updated string   `xml:"updated"`
	Entries []entry  `xml:"entry"`
}
type entry struct {
	ID      string  `xml:"id"`
	Title   string  `xml:"title"`
	Updated string  `xml:"updated"`
	Author  *author `xml:"author,omitempty"`
	Content string  `xml:"content,omitempty"`
	Links   []link  `xml:"link"`
}
type author struct {
	Name string `xml:"name"`
}
type link struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
	Type string `xml:"type,attr,omitempty"`
}

const opdsType = "application/atom+xml;profile=opds-catalog;kind=navigation"

func writeFeed(w http.ResponseWriter, f feed) {
	w.Header().Set("Content-Type", opdsType+"; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(f)
}
func now() string { return time.Now().UTC().Format(time.RFC3339) }
func (a *App) opdsRoot(w http.ResponseWriter, r *http.Request) {
	f := feed{Xmlns: "http://www.w3.org/2005/Atom", ID: "urn:armarium:root", Title: "Armarium", Updated: now()}
	for _, l := range a.config.Libraries {
		f.Entries = append(f.Entries, entry{ID: "urn:armarium:library:" + l.ID, Title: l.Name, Updated: now(), Links: []link{{Rel: "subsection", Href: "/opds/" + url.PathEscape(l.ID), Type: opdsType}}})
	}
	writeFeed(w, f)
}

func (a *App) library(id string) (Library, bool) {
	for _, l := range a.config.Libraries {
		if l.ID == id {
			return l, true
		}
	}
	return Library{}, false
}
func safePath(root, rel string) (string, bool) {
	rel, err := url.PathUnescape(rel)
	if err != nil {
		return "", false
	}
	rel = filepath.Clean(filepath.FromSlash(rel))
	if rel == "." {
		rel = ""
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	p := filepath.Join(root, rel)
	rr, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	pp, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", false
	}
	rr, _ = filepath.Abs(rr)
	pp, _ = filepath.Abs(pp)
	return p, pp == rr || strings.HasPrefix(pp, rr+string(filepath.Separator))
}
func escapedPath(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func (a *App) opdsLibrary(w http.ResponseWriter, r *http.Request) {
	l, ok := a.library(r.PathValue("library"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	rel := r.URL.Query().Get("path")
	dir, ok := safePath(l.Path, rel)
	if !ok {
		http.Error(w, "不正なパスです", 400)
		return
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f := feed{Xmlns: "http://www.w3.org/2005/Atom", ID: "urn:armarium:" + l.ID + ":" + rel, Title: l.Name + func() string {
		if rel != "" {
			return " / " + rel
		}
		return ""
	}(), Updated: now()}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name()) < strings.ToLower(items[j].Name()) })
	for _, item := range items {
		child := filepath.Join(rel, item.Name())
		info, e := item.Info()
		if e != nil {
			continue
		}
		if item.IsDir() {
			href := "/opds/" + url.PathEscape(l.ID) + "?path=" + url.QueryEscape(filepath.ToSlash(child))
			f.Entries = append(f.Entries, entry{ID: "urn:armarium:dir:" + l.ID + ":" + filepath.ToSlash(child), Title: item.Name(), Updated: info.ModTime().UTC().Format(time.RFC3339), Links: []link{{Rel: "subsection", Href: href, Type: opdsType}}})
			continue
		}
		if !supported(item.Name()) {
			continue
		}
		m := a.cache.get(filepath.Join(dir, item.Name()), info)
		dl := "/download/" + url.PathEscape(l.ID) + "/" + escapedPath(child)
		e2 := entry{ID: "urn:armarium:book:" + l.ID + ":" + filepath.ToSlash(child), Title: m.Title, Updated: m.Modified.UTC().Format(time.RFC3339), Content: fmt.Sprintf("%d bytes", m.Size), Links: []link{{Rel: "http://opds-spec.org/acquisition", Href: dl, Type: m.MediaType}}}
		if m.Author != "" {
			e2.Author = &author{Name: m.Author}
		}
		f.Entries = append(f.Entries, e2)
	}
	writeFeed(w, f)
}

func (a *App) download(w http.ResponseWriter, r *http.Request) {
	l, ok := a.library(r.PathValue("library"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	p, ok := safePath(l.Path, r.PathValue("path"))
	if !ok || !supported(p) {
		http.Error(w, "不正なファイルです", 400)
		return
	}
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mediaType(strings.ToLower(filepath.Ext(p))))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(filepath.Base(p))))
	http.ServeFile(w, r, p)
}
