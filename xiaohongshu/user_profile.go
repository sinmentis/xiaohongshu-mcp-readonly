package xiaohongshu

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

type ProfileTab string

const (
	TabNotes     ProfileTab = "note"
	TabFavorites ProfileTab = "fav"
	TabLiked     ProfileTab = "liked"
)

func ParseProfileTab(s string) (ProfileTab, error) {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "", "note", "notes", "笔记":
		return TabNotes, nil
	case "fav", "favorites", "favorite", "收藏":
		return TabFavorites, nil
	case "liked", "like", "点赞":
		return TabLiked, nil
	}
	return "", fmt.Errorf("未知的主页 tab %q，可选：note / fav / liked", s)
}

type UserProfileAction struct {
	page *rod.Page
}

func NewUserProfileAction(page *rod.Page) *UserProfileAction {
	pp := page.Timeout(60 * time.Second)
	applySiteLocale(pp)
	return &UserProfileAction{page: pp}
}

func (u *UserProfileAction) UserProfile(ctx context.Context, userID, xsecToken string, tab ProfileTab) (*UserProfileResponse, error) {
	// Context replaces the constructor timeout.
	page := u.page.Context(ctx).Timeout(60 * time.Second)

	searchURL := makeUserProfileURL(userID, xsecToken, tab)
	page.MustNavigate(searchURL)
	page.MustWaitStable()

	return u.extractUserProfileData(page, tab)
}

func (u *UserProfileAction) extractUserProfileData(page *rod.Page, tab ProfileTab) (*UserProfileResponse, error) {
	page.MustWait(`() => window.__INITIAL_STATE__ !== undefined`)

	userDataResult := page.MustEval(`() => {
		if (window.__INITIAL_STATE__ &&
		    window.__INITIAL_STATE__.user &&
		    window.__INITIAL_STATE__.user.userPageData) {
			const userPageData = window.__INITIAL_STATE__.user.userPageData;
			const data = userPageData.value !== undefined ? userPageData.value : userPageData._value;
			if (data) {
				return JSON.stringify(data);
			}
		}
		return "";
	}`).String()

	if userDataResult == "" {
		return nil, fmt.Errorf("user.userPageData.value not found in __INITIAL_STATE__")
	}

	notesResult := page.MustEval(`() => {
		const u = window.__INITIAL_STATE__ && window.__INITIAL_STATE__.user;
		if (!u || !u.notes) return "";
		const unwrap = (o) => (o && o.value !== undefined) ? o.value : (o && o._value);
		const notes = unwrap(u.notes);
		if (!notes) return "";
		const active = unwrap(u.activeTab) || {};
		return JSON.stringify({notes: notes, index: active.index || 0, query: active.query || ""});
	}`).String()

	if notesResult == "" {
		return nil, fmt.Errorf("user.notes.value not found in __INITIAL_STATE__")
	}

	var userPageData struct {
		Interactions []UserInteractions `json:"interactions"`
		BasicInfo    UserBasicInfo      `json:"basicInfo"`
	}
	if err := json.Unmarshal([]byte(userDataResult), &userPageData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal userPageData: %w", err)
	}

	var notesData struct {
		Notes [][]Feed `json:"notes"`
		Index int      `json:"index"`
		Query string   `json:"query"`
	}
	if err := json.Unmarshal([]byte(notesResult), &notesData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notes: %w", err)
	}

	want := tab
	if want == "" {
		want = TabNotes
	}
	if notesData.Query != "" && ProfileTab(notesData.Query) != want {
		return nil, fmt.Errorf("当前 tab 为 %q，与请求的 %q 不符", notesData.Query, want)
	}

	response := &UserProfileResponse{
		UserBasicInfo: userPageData.BasicInfo,
		Interactions:  userPageData.Interactions,
	}

	// Each tab has its own notes bucket.
	if notesData.Index >= 0 && notesData.Index < len(notesData.Notes) {
		response.Feeds = append(response.Feeds, notesData.Notes[notesData.Index]...)
	}

	return response, nil
}

func makeUserProfileURL(userID, xsecToken string, tab ProfileTab) string {
	url := fmt.Sprintf("%s/user/profile/%s?xsec_token=%s&xsec_source=pc_note", Site().Base, userID, xsecToken)
	if tab != "" && tab != TabNotes {
		url += fmt.Sprintf("&tab=%s&subTab=note", tab)
	}
	return url
}
