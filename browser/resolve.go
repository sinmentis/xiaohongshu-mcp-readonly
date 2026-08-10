package browser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/sirupsen/logrus"
)

// Resolution describes the selected browser and its capabilities.
type Resolution struct {
	Path              string
	Source            string
	SourceFingerprint bool
}

// ResolveBrowser prefers an explicit path, then the bundled browser, system
// Chromium, and an existing Playwright Chromium installation.
func ResolveBrowser() (Resolution, error) {
	var bundled func() (string, error)
	if _, _, ok := platformAsset(); ok {
		bundled = EnsureBrowser
	}
	return resolveBrowser(
		os.Getenv("XHS_BROWSER_BIN"),
		bundled,
		launcher.LookPath,
		findPlaywrightBrowser,
	)
}

func resolveBrowser(
	explicit string,
	bundled func() (string, error),
	system func() (string, bool),
	playwright func() string,
) (Resolution, error) {
	if explicit != "" {
		path, err := resolveExecutable(explicit)
		if err != nil {
			return Resolution{}, fmt.Errorf("invalid XHS_BROWSER_BIN: %w", err)
		}
		return Resolution{Path: path, Source: "XHS_BROWSER_BIN"}, nil
	}

	var bundledErr error
	if bundled != nil {
		path, err := bundled()
		if err == nil {
			return Resolution{
				Path:              path,
				Source:            "bundled CloakBrowser",
				SourceFingerprint: true,
			}, nil
		}
		bundledErr = err
		logrus.Warnf("Bundled browser unavailable; trying local Chromium: %v", err)
	}

	if path, ok := system(); ok {
		return Resolution{Path: path, Source: "system Chromium"}, nil
	}

	if path := playwright(); path != "" {
		return Resolution{Path: path, Source: "Playwright Chromium"}, nil
	}

	if bundledErr != nil {
		return Resolution{}, fmt.Errorf(
			"bundled browser unavailable and no local Chromium was found: %w",
			bundledErr,
		)
	}
	return Resolution{}, fmt.Errorf(
		"no compatible browser found; install Chromium or set XHS_BROWSER_BIN",
	)
}

func resolveExecutable(path string) (string, error) {
	if filepath.Base(path) == path {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return "", err
		}
		path = resolved
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf("%s is not executable", path)
	}

	return path, nil
}

func findPlaywrightBrowser() string {
	if runtime.GOOS != "linux" {
		return ""
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}

	matches, err := filepath.Glob(filepath.Join(
		cacheDir,
		"ms-playwright",
		"chromium-*",
		"chrome-linux",
		"chrome",
	))
	if err != nil {
		return ""
	}

	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	for _, path := range matches {
		if resolved, err := resolveExecutable(path); err == nil {
			return resolved
		}
	}

	return ""
}
