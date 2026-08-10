package main

import (
	"context"
	"encoding/json"
	"flag"

	"github.com/go-rod/rod"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/browser"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/configs"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/cookies"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
	"github.com/sirupsen/logrus"
)

func main() {
	var site string
	flag.StringVar(&site, "site", xiaohongshu.SiteXiaohongshu, "site: xiaohongshu | rednote")
	flag.Parse()

	if err := xiaohongshu.SetSite(site); err != nil {
		logrus.Fatalf("invalid site configuration: %v", err)
	}

	// 登录的时候，需要界面，所以不能无头模式。
	// 登录与后续运行共用同一份站点会话文件和 seed。
	store := cookies.NewLoadCookie(cookies.GetCookiesFilePathForSite(site))

	b := browser.NewBrowser(false,
		browser.WithFingerprintSeed(configs.ResolveFingerprintSeed(store)),
		browser.WithProxy(configs.ProxyFromEnv()),
		browser.WithSite(site),
	)
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	action := xiaohongshu.NewLogin(page)

	status, err := action.CheckLoginStatus(context.Background())
	if err != nil {
		logrus.Fatalf("failed to check login status: %v", err)
	}

	logrus.Infof("当前登录状态: %v", status)

	if status {
		return
	}

	// 开始登录流程
	logrus.Info("开始登录流程...")
	if err = action.Login(context.Background()); err != nil {
		logrus.Fatalf("登录失败: %v", err)
	} else {
		if err := saveCookies(page); err != nil {
			logrus.Fatalf("failed to save cookies: %v", err)
		}
	}

	// 再次检查登录状态确认成功
	status, err = action.CheckLoginStatus(context.Background())
	if err != nil {
		logrus.Fatalf("failed to check login status after login: %v", err)
	}

	if status {
		logrus.Info("登录成功！")
	} else {
		logrus.Error("登录流程完成但仍未登录")
	}

}

func saveCookies(page *rod.Page) error {
	cks, err := page.Browser().GetCookies()
	if err != nil {
		return err
	}

	data, err := json.Marshal(cks)
	if err != nil {
		return err
	}

	cookieLoader := cookies.NewLoadCookie(
		cookies.GetCookiesFilePathForSite(xiaohongshu.Site().Name),
	)
	return cookieLoader.SaveCookies(data)
}
