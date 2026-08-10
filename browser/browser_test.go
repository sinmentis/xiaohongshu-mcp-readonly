package browser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskProxyCredentials(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "空字符串", input: "", want: ""},
		{name: "无认证信息原样返回", input: "http://127.0.0.1:8080", want: "http://127.0.0.1:8080"},
		{name: "用户名和密码脱敏", input: "http://user:pass@host:8080", want: "http://***:***@host:8080"},
		{name: "仅用户名脱敏", input: "http://user@host:8080", want: "http://***@host:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, maskProxyCredentials(tt.input))
		})
	}
}

func TestValidateProxyURL(t *testing.T) {
	for _, proxyURL := range []string{
		"http://127.0.0.1:8080",
		"https://proxy.example.com:8443",
		"socks5://127.0.0.1:1080",
	} {
		t.Run(proxyURL, func(t *testing.T) {
			got, err := validateProxyURL(proxyURL)
			assert.NoError(t, err)
			assert.Equal(t, proxyURL, got)
		})
	}

	for _, proxyURL := range []string{
		"127.0.0.1:8080",
		"ftp://127.0.0.1:21",
		"http://user:password@127.0.0.1:8080",
	} {
		t.Run(proxyURL, func(t *testing.T) {
			_, err := validateProxyURL(proxyURL)
			assert.Error(t, err)
		})
	}
}

func TestOptions(t *testing.T) {
	cfg := &browserConfig{}
	WithSite("rednote")(cfg)
	WithFingerprintSeed(98759)(cfg)
	WithProxy("http://127.0.0.1:8080")(cfg)
	WithBrowserBinary("/usr/bin/chromium", false)(cfg)

	assert.Equal(t, "rednote", cfg.site)
	assert.Equal(t, 98759, cfg.fingerprintSeed)
	assert.Equal(t, "http://127.0.0.1:8080", cfg.proxy)
	assert.Equal(t, "/usr/bin/chromium", cfg.browserBin)
	assert.False(t, cfg.sourceFingerprint)
}

func TestOptionsDefaults(t *testing.T) {
	cfg := &browserConfig{}

	assert.Empty(t, cfg.site)
	assert.Zero(t, cfg.fingerprintSeed)
	assert.Empty(t, cfg.proxy)
	assert.Empty(t, cfg.browserBin)
	assert.False(t, cfg.sourceFingerprint)
}

func TestFallbackViewportUsesStableSeed(t *testing.T) {
	assert.Equal(t, fallbackViewport(98759), fallbackViewport(98759))
	assert.Contains(t, fallbackViewports, fallbackViewport(98759))
	assert.Equal(t, fallbackViewports[0], fallbackViewport(0))
}
