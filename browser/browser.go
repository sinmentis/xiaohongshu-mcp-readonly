package browser

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/sinmentis/xiaohongshu-mcp-readonly/cookies"
	"github.com/sirupsen/logrus"
	"github.com/xpzouying/headless_browser"
)

type Browser = headless_browser.Browser

var fallbackViewports = []string{
	"1365,768",
	"1440,900",
	"1536,864",
	"1920,1080",
}

type browserConfig struct {
	site              string
	fingerprintSeed   int
	proxy             string
	browserBin        string
	sourceFingerprint bool
}

type Option func(*browserConfig)

func WithSite(site string) Option {
	return func(c *browserConfig) {
		c.site = site
	}
}

func WithProxy(proxy string) Option {
	return func(c *browserConfig) {
		c.proxy = proxy
	}
}

func WithFingerprintSeed(seed int) Option {
	return func(c *browserConfig) {
		c.fingerprintSeed = seed
	}
}

func WithBrowserBinary(path string, sourceFingerprint bool) Option {
	return func(c *browserConfig) {
		c.browserBin = path
		c.sourceFingerprint = sourceFingerprint
	}
}

func maskProxyCredentials(proxyURL string) string {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return "<redacted>"
	}
	if u.User == nil {
		return proxyURL
	}
	credential := "***"
	if _, hasPassword := u.User.Password(); hasPassword {
		credential = "***:***"
	}
	return strings.Replace(proxyURL, u.User.String()+"@", credential+"@", 1)
}

func validateProxyURL(proxyURL string) (string, error) {
	u, err := url.Parse(proxyURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("代理地址无效")
	}
	switch u.Scheme {
	case "http", "https", "socks5":
	default:
		return "", fmt.Errorf("代理协议必须是 http、https 或 socks5")
	}
	if u.User != nil {
		return "", fmt.Errorf("不支持在代理 URL 中嵌入凭据")
	}
	return u.String(), nil
}

func NewBrowser(headless bool, options ...Option) *Browser {
	cfg := &browserConfig{}
	for _, option := range options {
		option(cfg)
	}

	if cfg.browserBin == "" {
		resolution, err := ResolveBrowser()
		if err != nil {
			panic(fmt.Sprintf("浏览器不可用，拒绝启动: %v", err))
		}
		cfg.browserBin = resolution.Path
		cfg.sourceFingerprint = resolution.SourceFingerprint
	}

	extraFlags := make(map[string]string)
	opts := []headless_browser.Option{
		headless_browser.WithHeadless(headless),
		headless_browser.WithChromeBinPath(cfg.browserBin),
	}

	if cfg.sourceFingerprint {
		opts = append(opts,
			headless_browser.WithFingerprint(""),
			headless_browser.WithStealthJS(false),
			headless_browser.WithLanguage("zh-CN"),
		)
		extraFlags["fingerprint-brand"] = "Chrome"
	} else {
		opts = append(opts, headless_browser.WithStealthJS(true))
		extraFlags["lang"] = "zh-CN"
		if cfg.fingerprintSeed > 0 {
			viewport := fallbackViewport(cfg.fingerprintSeed)
			extraFlags["window-size"] = viewport
			logrus.Infof(
				"fallback browser identity pinned: seed=%d viewport=%s",
				cfg.fingerprintSeed,
				viewport,
			)
		}
	}

	if cfg.proxy != "" {
		proxyURL, err := validateProxyURL(cfg.proxy)
		if err != nil {
			panic(fmt.Sprintf("代理配置无效: %v", err))
		}
		opts = append(opts, headless_browser.WithProxy(proxyURL))
		logrus.Infof("Using proxy: %s", maskProxyCredentials(proxyURL))
	}

	if cfg.sourceFingerprint && cfg.fingerprintSeed > 0 {
		opts = append(opts, headless_browser.WithFingerprintSeed(cfg.fingerprintSeed))
		logrus.Infof("fingerprint seed pinned: %d", cfg.fingerprintSeed)
	}

	opts = append(opts, headless_browser.WithExtraFlags(extraFlags))

	cookieLoader := cookies.NewLoadCookie(cookies.GetCookiesFilePathForSite(cfg.site))
	if data, err := cookieLoader.LoadCookies(); err == nil {
		opts = append(opts, headless_browser.WithCookies(string(data)))
		logrus.Debug("loaded cookies from file successfully")
	} else {
		logrus.Warnf("failed to load cookies: %v", err)
	}

	return headless_browser.New(opts...)
}

func fallbackViewport(seed int) string {
	if seed <= 0 {
		return fallbackViewports[0]
	}
	return fallbackViewports[seed%len(fallbackViewports)]
}
