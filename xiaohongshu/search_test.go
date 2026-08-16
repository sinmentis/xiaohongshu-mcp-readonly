package xiaohongshu

import (
	"errors"
	"testing"

	"github.com/go-rod/rod"
	"github.com/stretchr/testify/require"
)

type fakeSearchNavigator struct {
	calls       []string
	waitJS      string
	navigateErr error
	loadErr     error
	stateErr    error
}

func (f *fakeSearchNavigator) Navigate(string) error {
	f.calls = append(f.calls, "navigate")
	return f.navigateErr
}

func (f *fakeSearchNavigator) WaitLoad() error {
	f.calls = append(f.calls, "load")
	return f.loadErr
}

func (f *fakeSearchNavigator) Wait(options *rod.EvalOptions) error {
	f.calls = append(f.calls, "search-state")
	f.waitJS = options.JS
	return f.stateErr
}

func TestNavigateToSearchWaitsForSearchState(t *testing.T) {
	t.Run("waits for load and search state without global page stability", func(t *testing.T) {
		page := &fakeSearchNavigator{}

		err := navigateToSearch(page, "https://www.rednote.com/search_result")

		require.NoError(t, err)
		require.Equal(t, []string{"navigate", "load", "search-state"}, page.calls)
		require.Contains(t, page.waitJS, "search.state")
		require.Contains(t, page.waitJS, `state === "success"`)
	})

	t.Run("stops after navigation failure", func(t *testing.T) {
		page := &fakeSearchNavigator{navigateErr: errors.New("navigation failed")}

		err := navigateToSearch(page, "https://www.rednote.com/search_result")

		require.ErrorContains(t, err, "navigate to search page")
		require.Equal(t, []string{"navigate"}, page.calls)
	})
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
