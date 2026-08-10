package main

import (
	"flag"
	"time"

	"github.com/sinmentis/xiaohongshu-mcp-readonly/browser"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/configs"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/cookies"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
	"github.com/sirupsen/logrus"
)

// version 构建版本号，发布时通过 -ldflags "-X main.version=vX.Y.Z" 注入。
var version = "dev"

func main() {
	var (
		headless      bool
		port          string
		minInterval   time.Duration
		requestJitter time.Duration
		maxComments   int
		maxReplies    int
		site          string
	)
	flag.BoolVar(&headless, "headless", true, "run the browser headlessly")
	flag.StringVar(&port, "port", "127.0.0.1:18060", "local loopback listen address")
	flag.DurationVar(&minInterval, "min-request-interval", 30*time.Second, "minimum delay between completed site operations")
	flag.DurationVar(&requestJitter, "request-jitter", 15*time.Second, "maximum additional random delay per operation")
	flag.IntVar(&maxComments, "max-comments", 50, "maximum top-level comments per note request")
	flag.IntVar(&maxReplies, "max-replies", 10, "maximum reply-thread threshold")
	flag.StringVar(&site, "site", xiaohongshu.SiteXiaohongshu, "site: xiaohongshu | rednote")
	flag.Parse()

	if err := xiaohongshu.SetSite(site); err != nil {
		logrus.Fatalf("invalid site configuration: %v", err)
	}

	logrus.Infof("xiaohongshu-mcp version: %s", version)
	logrus.Infof("site: %s", xiaohongshu.Site().Name)

	accessPolicy := AccessPolicy{
		MinInterval: minInterval,
		MaxJitter:   requestJitter,
		MaxComments: maxComments,
		MaxReplies:  maxReplies,
	}
	if err := accessPolicy.Validate(); err != nil {
		logrus.Fatalf("invalid access policy: %v", err)
	}

	// 启动时解析浏览器，缺失时直接退出，不拖到第一个请求才失败。
	resolution, err := browser.ResolveBrowser()
	if err != nil {
		logrus.Fatalf("%v", err)
	}
	logrus.Infof("using browser binary: %s (%s)", resolution.Path, resolution.Source)
	if !resolution.SourceFingerprint {
		logrus.Warn("browser does not support source-level fingerprint flags; using go-rod/stealth fallback")
	}

	configs.InitHeadless(headless)
	// 入口层解析出 seed 和代理，经 configs 透传给浏览器工厂。
	// seed 取值：环境变量 > 站点会话文件 > 新生成并写回。
	configs.SetFingerprintSeed(configs.ResolveFingerprintSeed(
		cookies.NewLoadCookie(cookies.GetCookiesFilePathForSite(site))))
	configs.SetProxy(configs.ProxyFromEnv())
	configs.SetBrowser(resolution.Path, resolution.SourceFingerprint)

	// 初始化服务
	xiaohongshuService := NewXiaohongshuService(accessPolicy)

	// 创建并启动应用服务器
	appServer := NewAppServer(xiaohongshuService)
	if err := appServer.Start(port); err != nil {
		logrus.Fatalf("failed to run server: %v", err)
	}
}
