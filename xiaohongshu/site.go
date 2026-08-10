package xiaohongshu

import (
	"fmt"
	"net/url"
	"sync"
)

const (
	SiteXiaohongshu = "xiaohongshu"
	SiteRednote     = "rednote"
)

// SiteConfig owns site URLs and locale behavior.
type SiteConfig struct {
	Name       string
	Base       string
	Home       string
	PublishURL string
	ForceZhCN  bool
}

var sites = map[string]SiteConfig{
	SiteXiaohongshu: {
		Name:       SiteXiaohongshu,
		Base:       "https://www.xiaohongshu.com",
		Home:       "https://www.xiaohongshu.com/explore",
		PublishURL: "https://creator.xiaohongshu.com/publish/publish?source=official",
	},
	SiteRednote: {
		Name:       SiteRednote,
		Base:       "https://www.rednote.com",
		Home:       "https://www.rednote.com/explore",
		PublishURL: "https://creator.rednote.com/publish/publish?source=official",
		ForceZhCN:  true,
	},
}

var (
	siteMu      sync.RWMutex
	currentSite = sites[SiteXiaohongshu]
)

func SetSite(name string) error {
	site, ok := sites[name]
	if !ok {
		return fmt.Errorf(
			"unknown site %q; supported values are %s and %s",
			name,
			SiteXiaohongshu,
			SiteRednote,
		)
	}

	siteMu.Lock()
	currentSite = site
	siteMu.Unlock()
	return nil
}

func Site() SiteConfig {
	siteMu.RLock()
	defer siteMu.RUnlock()
	return currentSite
}

func (s SiteConfig) MatchesURL(rawURL string) bool {
	actual, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	base, err := url.Parse(s.Base)
	if err != nil {
		return false
	}
	return actual.Scheme == base.Scheme && actual.Hostname() == base.Hostname()
}
