package xiaohongshu

import (
	"context"
	"errors"
	"testing"

	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/require"
	"github.com/ysmood/gson"
)

type searchSnapshot struct {
	url       string
	ready     bool
	loggedOut bool
	err       error
}

type fakeSearchNavigator struct {
	calls       []string
	waitJS      string
	navigateErr error
	snapshots   []searchSnapshot
}

func (f *fakeSearchNavigator) Navigate(string) error {
	f.calls = append(f.calls, "navigate")
	return f.navigateErr
}

func (f *fakeSearchNavigator) GetContext() context.Context {
	return context.Background()
}

func (f *fakeSearchNavigator) Eval(js string, _ ...interface{}) (*proto.RuntimeRemoteObject, error) {
	f.calls = append(f.calls, "search-state")
	f.waitJS = js
	if len(f.snapshots) == 0 {
		return nil, errors.New("unexpected search wait")
	}
	snapshot := f.snapshots[0]
	f.snapshots = f.snapshots[1:]
	return &proto.RuntimeRemoteObject{Value: gson.New(map[string]any{
		"url": snapshot.url, "ready": snapshot.ready, "loggedOut": snapshot.loggedOut,
	})}, snapshot.err
}

func TestNavigateToSearchWaitsForSearchState(t *testing.T) {
	site := sites[SiteRednote]
	searchURL := site.Base + "/search_result"
	t.Run("waits for search state without global page stability", func(t *testing.T) {
		page := &fakeSearchNavigator{snapshots: []searchSnapshot{{url: searchURL, ready: true}}}

		err := navigateToSearch(page, site, searchURL)

		require.NoError(t, err)
		require.Equal(t, []string{"navigate", "search-state"}, page.calls)
		require.Contains(t, page.waitJS, "search.state")
		require.Contains(t, page.waitJS, `state === "success"`)
	})

	t.Run("ready results take precedence over a stale guest signal", func(t *testing.T) {
		page := &fakeSearchNavigator{snapshots: []searchSnapshot{{
			url: searchURL, ready: true, loggedOut: true,
		}}}

		require.NoError(t, navigateToSearch(page, site, searchURL))
	})

	t.Run("stops on delayed verification even if ready", func(t *testing.T) {
		page := &fakeSearchNavigator{snapshots: []searchSnapshot{
			{url: searchURL},
			{err: cdp.ErrCtxDestroyed},
			{url: site.Base + "/website-login/captcha?redirectPath=/search_result", ready: true},
		}}
		require.ErrorIs(t, navigateToSearch(page, site, searchURL), ErrSearchVerificationRequired)
		require.Empty(t, page.snapshots)
	})

	t.Run("stops after navigation failure", func(t *testing.T) {
		page := &fakeSearchNavigator{navigateErr: errors.New("navigation failed")}

		err := navigateToSearch(page, site, searchURL)

		require.ErrorContains(t, err, "navigate to search page")
		require.Equal(t, []string{"navigate"}, page.calls)
	})
}

func TestNavigateToSearchDetectsLoggedOutPage(t *testing.T) {
	site := sites[SiteRednote]
	searchURL := site.Base + "/search_result"
	page := &fakeSearchNavigator{snapshots: []searchSnapshot{
		{url: searchURL, loggedOut: true},
	}}

	err := navigateToSearch(page, site, searchURL)

	require.ErrorIs(t, err, ErrSearchLoginRequired)
	require.Empty(t, page.snapshots)
}

func TestClassifySearchRedirect(t *testing.T) {
	for _, tt := range []struct {
		url  string
		want error
	}{
		{"https://www.xiaohongshu.com/website-login/captcha/continue", ErrSearchVerificationRequired},
		{"https://www.xiaohongshu.com/website-login?redirectPath=/website-login/captcha", ErrSearchLoginRequired},
		{"https://www.xiaohongshu.com/login/", ErrSearchLoginRequired},
		{"https://www.xiaohongshu.com/search_result?keyword=/website-login/captcha", nil},
		{"https://www.xiaohongshu.com/website-login-other", nil},
		{"https://www.xiaohongshu.com/website-login%2Fcaptcha", nil},
		{"https://www.rednote.com/website-login/captcha", nil},
	} {
		t.Run(tt.url, func(t *testing.T) {
			got := classifySearchRedirect(sites[SiteXiaohongshu], tt.url)
			if tt.want == nil {
				require.NoError(t, got)
			} else {
				require.ErrorIs(t, got, tt.want)
			}
		})
	}
}

func TestCollectFilters(t *testing.T) {
	t.Run("只展开非空字段", func(t *testing.T) {
		pending, err := collectFilters([]FilterOption{{
			NoteType:    "图文",
			PublishTime: "一天内",
		}})
		require.NoError(t, err)
		require.Equal(t, []pendingFilter{
			{group: "笔记类型", option: "图文"},
			{group: "发布时间", option: "一天内"},
		}, pending)
	})

	t.Run("五个字段全给", func(t *testing.T) {
		pending, err := collectFilters([]FilterOption{{
			SortBy:      "最新",
			NoteType:    "视频",
			PublishTime: "一周内",
			SearchScope: "已关注",
			Location:    "同城",
		}})
		require.NoError(t, err)
		require.Len(t, pending, 5)
		require.Equal(t, pendingFilter{group: "排序依据", option: "最新"}, pending[0])
		require.Equal(t, pendingFilter{group: "位置距离", option: "同城"}, pending[4])
	})

	t.Run("stable values map to site labels", func(t *testing.T) {
		pending, err := collectFilters([]FilterOption{{
			SortBy:      "most_liked",
			NoteType:    "image",
			PublishTime: "week",
			SearchScope: "following",
			Location:    "nearby",
		}})
		require.NoError(t, err)
		require.Equal(t, []pendingFilter{
			{group: "排序依据", option: "最多点赞"},
			{group: "笔记类型", option: "图文"},
			{group: "发布时间", option: "一周内"},
			{group: "搜索范围", option: "已关注"},
			{group: "位置距离", option: "附近"},
		}, pending)
	})

	t.Run("全空则无待应用项", func(t *testing.T) {
		pending, err := collectFilters([]FilterOption{{}})
		require.NoError(t, err)
		require.Empty(t, pending)
	})

	t.Run("显式默认值也不点击筛选面板", func(t *testing.T) {
		pending, err := collectFilters([]FilterOption{{
			SortBy:      "综合",
			NoteType:    "不限",
			PublishTime: "不限",
			SearchScope: "不限",
			Location:    "不限",
		}})
		require.NoError(t, err)
		require.Empty(t, pending)
	})

	t.Run("非法取值在打开页面之前就报错", func(t *testing.T) {
		_, err := collectFilters([]FilterOption{{NoteType: "不存在的类型"}})
		require.Error(t, err)
		// 错误里要带上可选值，调用方才知道该怎么改
		require.Contains(t, err.Error(), "note_type")
		require.Contains(t, err.Error(), "image")
	})

	t.Run("取值不能跨组", func(t *testing.T) {
		// 「视频」是笔记类型的选项，不能当排序依据
		_, err := collectFilters([]FilterOption{{SortBy: "视频"}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "sort_by")
	})
}

// TestFilterGroupsCoverFilterOption 组表必须覆盖 FilterOption 的每个字段，
// 否则以后新增字段会被静默忽略。
func TestFilterGroupsCoverFilterOption(t *testing.T) {
	all := FilterOption{
		SortBy:      "最新",
		NoteType:    "视频",
		PublishTime: "一天内",
		SearchScope: "已关注",
		Location:    "同城",
	}

	pending, err := collectFilters([]FilterOption{all})
	require.NoError(t, err)
	require.Len(t, pending, 5, "组表漏了 FilterOption 的字段")

	for _, g := range filterGroups {
		require.NotEmpty(t, g.field)
		require.NotEmpty(t, g.siteLabel)
		require.NotEmpty(t, g.defaultValue)
		require.NotEmpty(t, g.canonical)
		require.NotEmpty(t, g.aliases, "%s 没有合法取值清单", g.field)
		require.Contains(t, g.aliases, g.defaultValue)
		require.NotNil(t, g.pick, "%s 没有取值函数", g.field)
	}
}
