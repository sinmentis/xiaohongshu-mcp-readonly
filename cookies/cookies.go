package cookies

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
)

// RawMessage preserves cookie fields while wrapping legacy arrays in the v2 envelope.
type sessionFile struct {
	Version int             `json:"version"`
	Seed    int             `json:"seed,omitempty"`
	SavedAt string          `json:"saved_at,omitempty"`
	Cookies json.RawMessage `json:"cookies"`
}

const localCookiesPath = "cookies.json"

type Cookier interface {
	LoadCookies() ([]byte, error)
	SaveCookies(data []byte) error
	LoadSeed() int
	SaveSeed(seed int) error
}

type localCookie struct {
	path string
}

func NewLoadCookie(path string) Cookier {
	if path == "" {
		panic("path is required")
	}

	return &localCookie{
		path: path,
	}
}

// LoadCookies accepts both legacy arrays and the v2 session envelope.
func (c *localCookie) LoadCookies() ([]byte, error) {

	data, err := os.ReadFile(c.path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read cookies from tmp file")
	}

	var f sessionFile
	if err := json.Unmarshal(data, &f); err == nil && len(f.Cookies) > 0 {
		return f.Cookies, nil
	}

	return data, nil
}

func (c *localCookie) LoadSeed() int {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return 0
	}

	var f sessionFile
	if err := json.Unmarshal(data, &f); err != nil {
		return 0
	}
	return f.Seed
}

func (c *localCookie) SaveCookies(data []byte) error {
	return c.write(data, c.LoadSeed())
}

func (c *localCookie) SaveSeed(seed int) error {
	cks, err := c.LoadCookies()
	if err != nil {
		cks = nil
	}
	return c.write(cks, seed)
}

func (c *localCookie) write(cks []byte, seed int) error {
	if len(cks) == 0 {
		cks = []byte("[]")
	}

	data, err := json.MarshalIndent(sessionFile{
		Version: 2,
		Seed:    seed,
		SavedAt: time.Now().Format(time.RFC3339),
		Cookies: json.RawMessage(cks),
	}, "", "  ")
	if err != nil {
		return errors.Wrap(err, "marshal session file failed")
	}

	if dir := filepath.Dir(c.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return errors.Wrap(err, "create cookies dir failed")
		}
	}

	if err := os.WriteFile(c.path, data, 0600); err != nil {
		return err
	}
	return os.Chmod(c.path, 0600)
}

func GetCookiesFilePath() string {
	if path := os.Getenv("COOKIES_PATH"); path != "" {
		return path
	}
	return localCookiesPath
}

func GetCookiesFilePathForSite(site string) string {
	base := GetCookiesFilePath()
	if site == "" || site == "xiaohongshu" {
		return base
	}
	return filepath.Join(filepath.Dir(base), fmt.Sprintf("cookies-%s.json", site))
}
