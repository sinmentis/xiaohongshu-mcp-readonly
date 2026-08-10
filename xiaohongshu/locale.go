package xiaohongshu

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/sirupsen/logrus"
	"github.com/ysmood/gson"
)

// applySiteLocale keeps RedNote compatible with existing Chinese selectors.
func applySiteLocale(page *rod.Page) {
	if !Site().ForceZhCN {
		return
	}

	if err := (proto.NetworkSetExtraHTTPHeaders{Headers: proto.NetworkHeaders{
		"Accept-Language": gson.New("zh-CN,zh;q=0.9,en;q=0.8"),
	}}).Call(page); err != nil {
		logrus.Warnf("Failed to set site request language: %v", err)
	}
	if err := (&proto.EmulationSetLocaleOverride{Locale: "zh-CN"}).Call(page); err != nil {
		logrus.Warnf("Failed to set site locale: %v", err)
	}
}
