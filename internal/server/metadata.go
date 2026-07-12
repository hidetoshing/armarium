package server

import (
	"archive/zip"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Metadata struct {
	Title      string    `json:"title"`
	Author     string    `json:"author,omitempty"`
	MediaType  string    `json:"media_type"`
	Size       int64     `json:"size"`
	Modified   time.Time `json:"modified"`
	ImageCount int       `json:"image_count,omitempty"`
}

type cacheEntry struct {
	Size     int64    `json:"size"`
	ModUnix  int64    `json:"mod_unix"`
	Metadata Metadata `json:"metadata"`
}

type metadataCache struct {
	path        string
	mu          sync.Mutex
	directories map[string]int64
	entries     map[string]cacheEntry
}

type cacheFile struct {
	Version     int                   `json:"version"`
	Directories map[string]int64      `json:"directories"`
	Entries     map[string]cacheEntry `json:"entries"`
}

const cacheVersion = 3

func newMetadataCache(path string) *metadataCache {
	c := &metadataCache{path: path, directories: map[string]int64{}, entries: map[string]cacheEntry{}}
	b, err := os.ReadFile(path)
	if err == nil {
		var stored cacheFile
		if json.Unmarshal(b, &stored) == nil {
			switch stored.Version {
			case cacheVersion:
				if stored.Directories != nil {
					c.directories = stored.Directories
				}
				if stored.Entries != nil {
					c.entries = stored.Entries
				}
			case 0:
				// version 1では書籍エントリーがJSONのルートに格納されていた。
				_ = json.Unmarshal(b, &c.entries)
			}
		}
	}
	return c
}

func (c *metadataCache) refreshDirectory(dir string, info os.FileInfo, items []os.DirEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	modUnix := info.ModTime().UnixNano()
	if c.directories[dir] == modUnix {
		return
	}

	present := make(map[string]bool)
	for _, item := range items {
		if item.IsDir() || !item.Type().IsRegular() || !supported(item.Name()) {
			continue
		}
		itemInfo, err := item.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, item.Name())
		present[path] = true
		entry, ok := c.entries[path]
		if ok && entry.Size == itemInfo.Size() && entry.ModUnix == itemInfo.ModTime().UnixNano() {
			continue
		}
		metadata := extractMetadata(path, itemInfo)
		c.entries[path] = cacheEntry{Size: itemInfo.Size(), ModUnix: itemInfo.ModTime().UnixNano(), Metadata: metadata}
	}

	for path := range c.entries {
		if filepath.Dir(path) == dir && !present[path] {
			delete(c.entries, path)
		}
	}
	c.directories[dir] = modUnix
	c.persist()
}

func (c *metadataCache) get(path string, info os.FileInfo) Metadata {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.entries[path]; ok && v.Size == info.Size() && v.ModUnix == info.ModTime().UnixNano() {
		return v.Metadata
	}
	m := extractMetadata(path, info)
	c.entries[path] = cacheEntry{Size: info.Size(), ModUnix: info.ModTime().UnixNano(), Metadata: m}
	c.persist()
	return m
}

func (c *metadataCache) persist() {
	b, err := json.Marshal(cacheFile{Version: cacheVersion, Directories: c.directories, Entries: c.entries})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return
	}
	tmp := c.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, c.path)
	}
}

func extractMetadata(path string, info os.FileInfo) Metadata {
	ext := strings.ToLower(filepath.Ext(path))
	m := Metadata{Title: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), MediaType: mediaType(ext), Size: info.Size(), Modified: info.ModTime()}
	if ext == ".epub" {
		readEPUB(path, &m)
	} else if ext == ".cbz" || ext == ".zip" {
		m.ImageCount = countImages(path)
	}
	return m
}

func mediaType(ext string) string {
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".epub":
		return "application/epub+zip"
	case ".cbz", ".zip":
		return "application/vnd.comicbook+zip"
	default:
		return "application/zip"
	}
}

func countImages(path string) int {
	z, err := zip.OpenReader(path)
	if err != nil {
		return 0
	}
	defer z.Close()
	n := 0
	for _, f := range z.File {
		switch strings.ToLower(filepath.Ext(f.Name)) {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif":
			n++
		}
	}
	return n
}

func readEPUB(path string, m *Metadata) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return
	}
	defer z.Close()
	var container struct {
		Rootfiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	var opfPath string
	for _, f := range z.File {
		if f.Name == "META-INF/container.xml" {
			r, e := f.Open()
			if e == nil {
				_ = xml.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&container)
				r.Close()
			}
			break
		}
	}
	if len(container.Rootfiles) > 0 {
		opfPath = container.Rootfiles[0].FullPath
	}
	for _, f := range z.File {
		if f.Name == opfPath {
			var p struct {
				Title   string `xml:"metadata>title"`
				Creator string `xml:"metadata>creator"`
			}
			r, e := f.Open()
			if e == nil {
				_ = xml.NewDecoder(io.LimitReader(r, 4<<20)).Decode(&p)
				r.Close()
				if strings.TrimSpace(p.Title) != "" {
					m.Title = strings.TrimSpace(p.Title)
				}
				m.Author = strings.TrimSpace(p.Creator)
			}
			return
		}
	}
}

func supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf", ".epub", ".cbz", ".zip":
		return true
	}
	return false
}
