package xiaohongshu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/errors"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/humanize"
	"github.com/sirupsen/logrus"
)

type SearchResult struct {
	Search struct {
		Feeds FeedsValue `json:"feeds"`
	} `json:"search"`
}

type FilterOption struct {
	SortBy      string `json:"sort_by,omitempty" jsonschema:"排序依据: 综合|最新|最多点赞|最多评论|最多收藏,默认为'综合'"`
	NoteType    string `json:"note_type,omitempty" jsonschema:"笔记类型: 不限|视频|图文,默认为'不限'"`
	PublishTime string `json:"publish_time,omitempty" jsonschema:"发布时间: 不限|一天内|一周内|半年内,默认为'不限'"`
	SearchScope string `json:"search_scope,omitempty" jsonschema:"搜索范围: 不限|已看过|未看过|已关注,默认为'不限'"`
	Location    string `json:"location,omitempty" jsonschema:"位置距离: 不限|同城|附近,默认为'不限'"`
}

// Text matching is required because duplicate tags make index-based selectors unstable.
type filterGroup struct {
	label        string
	pick         func(FilterOption) string
	defaultValue string
	allowed      []string
}

var filterGroups = []filterGroup{
	{"排序依据", func(f FilterOption) string { return f.SortBy },
		"综合",
		[]string{"综合", "最新", "最多点赞", "最多评论", "最多收藏"}},
	{"笔记类型", func(f FilterOption) string { return f.NoteType },
		"不限",
		[]string{"不限", "视频", "图文"}},
	{"发布时间", func(f FilterOption) string { return f.PublishTime },
		"不限",
		[]string{"不限", "一天内", "一周内", "半年内"}},
	{"搜索范围", func(f FilterOption) string { return f.SearchScope },
		"不限",
		[]string{"不限", "已看过", "未看过", "已关注"}},
	{"位置距离", func(f FilterOption) string { return f.Location },
		"不限",
		[]string{"不限", "同城", "附近"}},
}

type pendingFilter struct {
	group  string
	option string
}

// Validate before navigation so invalid filters do not trigger a site request.
func collectFilters(filters []FilterOption) ([]pendingFilter, error) {
	var pending []pendingFilter

	for _, f := range filters {
		for _, g := range filterGroups {
			value := g.pick(f)
			if value == "" {
				continue
			}
			if !slices.Contains(g.allowed, value) {
				return nil, fmt.Errorf("%s 不支持 %q，可选：%s",
					g.label, value, strings.Join(g.allowed, "、"))
			}
			if value == g.defaultValue {
				continue
			}
			pending = append(pending, pendingFilter{group: g.label, option: value})
		}
	}

	return pending, nil
}

type SearchAction struct {
	page *rod.Page
}

type searchNavigator interface {
	Navigate(string) error
	WaitLoad() error
	Wait(*rod.EvalOptions) error
}

const searchStateReadyJS = `() => {
	const search = window.__INITIAL_STATE__?.search;
	if (!search?.state) {
		return false;
	}
	const rawState = search.state;
	const state = typeof rawState === "string"
		? rawState
		: rawState.value !== undefined
			? rawState.value
			: rawState._value;
	return state === "success";
}`

func NewSearchAction(page *rod.Page) *SearchAction {
	pp := page.Timeout(60 * time.Second)
	applySiteLocale(pp)

	return &SearchAction{page: pp}
}

func navigateToSearch(page searchNavigator, searchURL string) error {
	if err := page.Navigate(searchURL); err != nil {
		return fmt.Errorf("navigate to search page: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return fmt.Errorf("wait for search page load: %w", err)
	}
	if err := page.Wait(rod.Eval(searchStateReadyJS)); err != nil {
		return fmt.Errorf("wait for search results: %w", err)
	}
	return nil
}

func (s *SearchAction) Search(ctx context.Context, keyword string, filters ...FilterOption) ([]Feed, error) {
	pending, err := collectFilters(filters)
	if err != nil {
		return nil, err
	}

	// Context replaces the constructor deadline, so reapply the bounded timeout.
	page := s.page.Context(ctx).Timeout(60 * time.Second)

	searchURL := makeSearchURL(keyword)
	if err := navigateToSearch(page, searchURL); err != nil {
		return nil, err
	}
	humanize.Delay(ctx, humanize.AfterNavigate)

	if len(pending) > 0 {
		filterButton := page.MustElement(`div.filter`)
		if err := humanize.Hover(filterButton); err != nil {
			return nil, fmt.Errorf("悬停筛选按钮失败: %w", err)
		}
		humanize.Delay(ctx, humanize.BeforeClick)

		page.MustWait(`() => document.querySelector('div.filter-panel') !== null`)

		before := readFeedIDs(page)

		// WaitInteractable misreads the hover panel as blocked; ClickNoWait keeps it open.
		for _, pf := range pending {
			option, err := findFilterOption(page, pf)
			if err != nil {
				return nil, err
			}
			humanize.Delay(ctx, humanize.BeforeClick)
			if err := humanize.ClickNoWait(option); err != nil {
				return nil, fmt.Errorf("点击筛选选项「%s」失败: %w", pf.option, err)
			}
		}

		waitFeedsChanged(page, before, 15*time.Second)
	}

	result := page.MustEval(`() => {
		if (window.__INITIAL_STATE__ &&
		    window.__INITIAL_STATE__.search &&
		    window.__INITIAL_STATE__.search.feeds) {
			const feeds = window.__INITIAL_STATE__.search.feeds;
			const feedsData = feeds.value !== undefined ? feeds.value : feeds._value;
			if (feedsData) {
				return JSON.stringify(feedsData);
			}
		}
		return "";
	}`).String()

	if result == "" {
		return nil, errors.ErrNoFeeds
	}

	var feeds []Feed
	if err := json.Unmarshal([]byte(result), &feeds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal feeds: %w", err)
	}

	return onlyNotes(feeds), nil
}

const feedIDsJS = `() => {
	const f = window.__INITIAL_STATE__?.search?.feeds;
	const v = f ? (f.value !== undefined ? f.value : f._value) : null;
	return v ? v.map(x => x.id).join(",") : "";
}`

func readFeedIDs(page *rod.Page) string {
	res, err := page.Eval(feedIDsJS)
	if err != nil {
		return ""
	}
	return res.Value.Str()
}

// The site clears feeds before refilling them, so wait for a changed non-empty ID set.
func waitFeedsChanged(page *rod.Page, before string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if now := readFeedIDs(page); now != "" && now != before {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	logrus.Warnf("筛选后等待结果刷新超时（%s），返回的可能是筛选前的数据", timeout)
}

// Scope text matching to the filter panel; the page repeats these labels elsewhere.
func findFilterOption(page *rod.Page, pf pendingFilter) (*rod.Element, error) {
	groups, err := page.Elements("div.filter-panel div.filters")
	if err != nil {
		return nil, fmt.Errorf("读取筛选面板失败: %w", err)
	}

	for _, group := range groups {
		label, err := group.Element(":scope > span")
		if err != nil {
			continue
		}
		text, err := label.Text()
		if err != nil || strings.TrimSpace(text) != pf.group {
			continue
		}

		options, err := group.Elements("div.tags")
		if err != nil {
			return nil, fmt.Errorf("读取「%s」的选项失败: %w", pf.group, err)
		}

		var available []string
		for _, opt := range options {
			t, err := opt.Text()
			if err != nil {
				continue
			}
			t = strings.TrimSpace(t)
			if t == pf.option {
				return opt, nil
			}
			available = append(available, t)
		}
		return nil, fmt.Errorf("「%s」里没有选项「%s」，页面上是：%s",
			pf.group, pf.option, strings.Join(available, "、"))
	}

	return nil, fmt.Errorf("筛选面板里没有「%s」这一组", pf.group)
}

func makeSearchURL(keyword string) string {
	values := url.Values{}
	values.Set("keyword", keyword)
	values.Set("source", "web_explore_feed")

	return fmt.Sprintf("%s/search_result?%s", Site().Base, values.Encode())
}
