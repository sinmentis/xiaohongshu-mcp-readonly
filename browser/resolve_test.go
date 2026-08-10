package browser

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveBrowserUsesExplicitPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chromium")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0755))
	t.Setenv("XHS_BROWSER_BIN", path)

	resolution, err := ResolveBrowser()
	require.NoError(t, err)
	assert.Equal(t, path, resolution.Path)
	assert.Equal(t, "XHS_BROWSER_BIN", resolution.Source)
	assert.False(t, resolution.SourceFingerprint)
}

func TestResolveBrowserRejectsInvalidExplicitPath(t *testing.T) {
	t.Setenv("XHS_BROWSER_BIN", filepath.Join(t.TempDir(), "missing"))

	_, err := ResolveBrowser()
	assert.ErrorContains(t, err, "invalid XHS_BROWSER_BIN")
}

func TestResolveBrowserFallsBackWhenBundledDownloadFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chromium")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0755))

	resolution, err := resolveBrowser(
		"",
		func() (string, error) {
			return "", errors.New("offline")
		},
		func() (string, bool) {
			return path, true
		},
		func() string {
			return ""
		},
	)

	require.NoError(t, err)
	assert.Equal(t, path, resolution.Path)
	assert.Equal(t, "system Chromium", resolution.Source)
	assert.False(t, resolution.SourceFingerprint)
}

func TestFindPlaywrightBrowserUsesLatestVersion(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	oldPath := playwrightBrowserPath(cacheDir, "1200")
	newPath := playwrightBrowserPath(cacheDir, "1300")
	require.NoError(t, os.MkdirAll(filepath.Dir(oldPath), 0700))
	require.NoError(t, os.MkdirAll(filepath.Dir(newPath), 0700))
	require.NoError(t, os.WriteFile(oldPath, []byte("#!/bin/sh\n"), 0755))
	require.NoError(t, os.WriteFile(newPath, []byte("#!/bin/sh\n"), 0755))

	assert.Equal(t, newPath, findPlaywrightBrowser())
}

func playwrightBrowserPath(cacheDir, version string) string {
	return filepath.Join(cacheDir, "ms-playwright", "chromium-"+version, "chrome-linux", "chrome")
}
