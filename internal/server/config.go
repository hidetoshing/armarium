package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Listen    string    `json:"listen"`
	CachePath string    `json:"cache_path"`
	Users     []User    `json:"users"`
	Libraries []Library `json:"libraries"`
}

type User struct {
	Username       string `json:"username"`
	Password       string `json:"password,omitempty"`
	PasswordSHA256 string `json:"password_sha256,omitempty"`
}

type Library struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func loadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("設定ファイルの読み込み: %w", err)
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("設定ファイルの解析: %w", err)
	}
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.CachePath == "" {
		c.CachePath = "/data/cache/metadata.json"
	}
	if len(c.Users) == 0 || len(c.Libraries) == 0 {
		return Config{}, fmt.Errorf("users と libraries は1件以上必要です")
	}
	ids := map[string]bool{}
	for _, u := range c.Users {
		if u.Username == "" || (u.Password == "" && u.PasswordSHA256 == "") {
			return Config{}, fmt.Errorf("各ユーザーには username と password または password_sha256 が必要です")
		}
	}
	for _, l := range c.Libraries {
		if l.ID == "" || l.Name == "" || !filepath.IsAbs(l.Path) || ids[l.ID] || strings.ContainsAny(l.ID, "/\\") {
			return Config{}, fmt.Errorf("ライブラリ %q の id/name/path が不正です", l.ID)
		}
		ids[l.ID] = true
	}
	return c, nil
}
