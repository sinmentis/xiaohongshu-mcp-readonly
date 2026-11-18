package xiaohongshu

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
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
	SortBy      string `json:"sort_by,omitempty" jsonschema:"Sort order using a stable value such as relevance or latest"`
	NoteType    string `json:"note_type,omitempty" jsonschema:"Note type using all, video, or image"`
	PublishTime string `json:"publish_time,omitempty" jsonschema:"Publication window using all, day, week, or half_year"`
	SearchScope string `json:"search_scope,omitempty" jsonschema:"Search scope using all, viewed, unviewed, or following"`
	Location    string `json:"location,omitempty" jsonschema:"Location using all, same_city, or nearby"`
}

// Text matching is required because duplicate tags make index-based selectors unstable.
type filterGroup struct {
	field        string
	siteLabel    string
	pick         func(FilterOption) string
	defaultValue string
	canonical    []string
	aliases      map[string]string
}

var filterGroups = []filterGroup{
	{"sort_by", "排序依据", func(f FilterOption) string { return f.SortBy },
		"综合",
		[]string{"relevance", "latest", "most_liked", "most_commented", "most_collected"},
		map[string]string{
			"relevance": "综合", "latest": "最新", "most_liked": "最多点赞",
			"most_commented": "最多评论", "most_collected": "最多收藏",
			"综合": "综合", "最新": "最新", "最多点赞": "最多点赞",
			"最多评论": "最多评论", "最多收藏": "最多收藏",
		}},
	{"note_type", "笔记类型", func(f FilterOption) string { return f.NoteType },
		"不限",
		[]string{"all", "video", "image"},
		map[string]string{
			"all": "不限", "video": "视频", "image": "图文",
			"不限": "不限", "视频": "视频", "图文": "图文",
		}},
	{"publish_time", "发布时间", func(f FilterOption) string { return f.PublishTime },
		"不限",
		[]string{"all", "day", "week", "half_year"},
		map[string]string{
			"all": "不限", "day": "一天内", "week": "一周内", "half_year": "半年内",
			"不限": "不限", "一天内": "一天内", "一周内": "一周内", "半年内": "半年内",
		}},
	{"search_scope", "搜索范围", func(f FilterOption) string { return f.SearchScope },
		"不限",
		[]string{"all", "viewed", "unviewed", "following"},
		map[string]string{
			"all": "不限", "viewed": "已看过", "unviewed": "未看过", "following": "已关注",
			"不限": "不限", "已看过": "已看过", "未看过": "未看过", "已关注": "已关注",
		}},
	{"location", "位置距离", func(f FilterOption) string { return f.Location },
		"不限",
		[]string{"all", "same_city", "nearby"},
		map[string]string{
			"all": "不限", "same_city": "同城", "nearby": "附近",
			"不限": "不限", "同城": "同城", "附近": "附近",
		}},
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
			value := strings.TrimSpace(g.pick(f))
			if value == "" {
				continue
			}
			siteValue, ok := g.aliases[value]
			if !ok {
				return nil, &InvalidArgumentError{
					Field:     g.field,
					Value:     value,
					Supported: g.canonical,
				}
			}
			if siteValue == g.defaultValue {
				continue
			}
			pending = append(pending, pendingFilter{
				group:  g.siteLabel,
				option: siteValue,
			})
		}
	}

	return pending, nil
}

type SearchAction struct {
	page *rod.Page
}

type searchNavigator interface {
	Navigate(string) error
	GetContext() context.Context
	Eval(string, ...interface{}) (*proto.RuntimeRemoteObject, error)
}

var (
	ErrSearchLoginRequired        = stderrors.New("search requires an active login")
	ErrSearchVerificationRequired = stderrors.New("search requires platform security verification")
)

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

func navigateToSearch(page searchNavigator, site SiteConfig, searchURL string) error {
	if err := page.Navigate(searchURL); err != nil {
		return fmt.Errorf("navigate to search page: %w", err)
	}
	if err := waitForSearchCondition(page, site, searchStateReadyJS); err != nil {
		return fmt.Errorf("wait for search results: %w", err)
	}
	return nil
}

func classifySearchRedirect(site SiteConfig, rawURL string) error {
	current, err := url.Parse(rawURL)
	if err != nil || !site.MatchesURL(rawURL) {
		return nil
	}
	path := current.EscapedPath()
	switch {
	case path == "/website-login/captcha", strings.HasPrefix(path, "/website-login/captcha/"):
		return ErrSearchVerificationRequired
	case path == "/website-login", strings.HasPrefix(path, "/website-login/"),
		path == "/login", path == "/login/":
		return ErrSearchLoginRequired
	default:
		return nil
	}
}

func waitForSearchCondition(page searchNavigator, site SiteConfig, condition string) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	// Read URL and readiness from the same document, including delayed redirects.
	snapshotJS := `() => {
		const userInfo = window.__INITIAL_STATE__?.user?.userInfo;
		const user = userInfo && (userInfo.value !== undefined ? userInfo.value : userInfo);
		return {
			url: location.href,
			ready: (` + condition + `)(),
			loggedOut: !!(user && user.guest)
		};
	}`
	ctx := page.GetContext()
	for {
		state, err := page.Eval(snapshotJS)
		if err == nil {
			if blocked := classifySearchRedirect(site, state.Value.Get("url").Str()); blocked != nil {
				return blocked
			}
			if state.Value.Get("ready").Bool() {
				return nil
			}
			if state.Value.Get("loggedOut").Bool() {
				return ErrSearchLoginRequired
			}
		} else if !stderrors.Is(err, cdp.ErrCtxNotFound) &&
			!stderrors.Is(err, cdp.ErrCtxDestroyed) &&
			!stderrors.Is(err, cdp.ErrObjNotFound) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func currentSearchRedirect(page *rod.Page, site SiteConfig) error {
	probe := page.Timeout(time.Second)
	defer probe.CancelTimeout()
	tree, err := (proto.PageGetFrameTree{}).Call(probe)
	if err != nil || tree == nil || tree.FrameTree == nil || tree.FrameTree.Frame == nil {
		return nil
	}
	return classifySearchRedirect(site, tree.FrameTree.Frame.URL)
}

func (s *SearchAction) Search(ctx context.Context, keyword string, filters ...FilterOption) (_ []Feed, searchErr error) {
	pending, err := collectFilters(filters)
	if err != nil {
		return nil, err
	}

	// Context replaces the constructor deadline, so reapply the bounded timeout.
	page := s.page.Context(ctx).Timeout(60 * time.Second)
	defer page.CancelTimeout()
	site := Site()
	defer func() {
		if searchErr != nil && !stderrors.Is(searchErr, ErrSearchLoginRequired) &&
			!stderrors.Is(searchErr, ErrSearchVerificationRequired) {
			if blocked := currentSearchRedirect(page, site); blocked != nil {
				searchErr = blocked
			}
		}
	}()

	searchURL := makeSearchURL(site, keyword)
	if err := navigateToSearch(page, site, searchURL); err != nil {
		return nil, err
	}
	humanize.Delay(page.GetContext(), humanize.AfterNavigate)

	if len(pending) > 0 {
		if err := waitForSearchCondition(page, site, `() => document.querySelector('div.filter') !== null`); err != nil {
			return nil, err
		}
		filterButton, err := page.Element(`div.filter`)
		if err != nil {
			return nil, err
		}
		if err := humanize.Hover(filterButton); err != nil {
			return nil, fmt.Errorf("悬停筛选按钮失败: %w", err)
		}
		humanize.Delay(ctx, humanize.BeforeClick)

		if err := waitForSearchCondition(page, site, `() => document.querySelector('div.filter-panel') !== null`); err != nil {
			return nil, err
		}

		before := readFeedIDs(page)

		// WaitInteractable misreads the hover panel as blocked; ClickNoWait keeps it open.
		for _, pf := range pending {
			if blocked := currentSearchRedirect(page, site); blocked != nil {
				return nil, blocked
			}
			option, err := findFilterOption(page, pf)
			if err != nil {
				return nil, err
			}
			humanize.Delay(ctx, humanize.BeforeClick)
			if err := humanize.ClickNoWait(option); err != nil {
				return nil, fmt.Errorf("点击筛选选项「%s」失败: %w", pf.option, err)
			}
		}

		if err := waitFeedsChanged(page, site, before, 15*time.Second); err != nil {
			return nil, err
		}
	}

	snapshot, err := page.Eval(`() => {
		const url = location.href;
		if (window.__INITIAL_STATE__ &&
		    window.__INITIAL_STATE__.search &&
		    window.__INITIAL_STATE__.search.feeds) {
			const feeds = window.__INITIAL_STATE__.search.feeds;
			const feedsData = feeds.value !== undefined ? feeds.value : feeds._value;
			if (feedsData) {
				return {url, result: JSON.stringify(feedsData)};
			}
		}
		return {url, result: ""};
	}`)
	if err != nil {
		return nil, fmt.Errorf("read search results: %w", err)
	}
	if blocked := classifySearchRedirect(site, snapshot.Value.Get("url").Str()); blocked != nil {
		return nil, blocked
	}

	result := snapshot.Value.Get("result").Str()
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
func waitFeedsChanged(page *rod.Page, site SiteConfig, before string, timeout time.Duration) error {
	bounded := page.Timeout(timeout)
	defer bounded.CancelTimeout()
	previous, _ := json.Marshal(before)
	condition := `() => { const ids = (` + feedIDsJS + `)(); return ids !== "" && ids !== ` + string(previous) + `; }`
	err := waitForSearchCondition(bounded, site, condition)
	if !stderrors.Is(err, context.DeadlineExceeded) || page.GetContext().Err() != nil {
		return err
	}
	logrus.Warnf("筛选后等待结果刷新超时（%s），返回的可能是筛选前的数据", timeout)
	return nil
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

func makeSearchURL(site SiteConfig, keyword string) string {
	values := url.Values{}
	values.Set("keyword", keyword)
	values.Set("source", "web_explore_feed")

	return fmt.Sprintf("%s/search_result?%s", site.Base, values.Encode())
}
