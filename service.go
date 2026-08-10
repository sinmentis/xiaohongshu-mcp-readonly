package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-rod/rod"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/browser"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/configs"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/cookies"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/pkg/downloader"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/pkg/xhsutil"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
	"github.com/sirupsen/logrus"
)

// XiaohongshuService 小红书业务服务
type XiaohongshuService struct {
	logins     loginSessions
	accessGate *accessGate
	policy     AccessPolicy
}

const (
	loginStateCacheTTL              = 90 * time.Second
	loginSessionTimeout             = 10 * time.Minute
	loginStabilityWindow            = 3 * time.Second
	persistedLoginVerificationLimit = 60 * time.Second
)

// NewXiaohongshuService 创建小红书服务实例
func NewXiaohongshuService(policies ...AccessPolicy) *XiaohongshuService {
	policy := DefaultAccessPolicy()
	if len(policies) > 0 {
		policy = policies[0]
	}

	return &XiaohongshuService{
		accessGate: newAccessGate(policy.MinInterval, policy.MaxJitter),
		policy:     policy,
	}
}

// PublishRequest 发布请求
type PublishRequest struct {
	Title      string   `json:"title" binding:"required"`
	Content    string   `json:"content" binding:"required"`
	Images     []string `json:"images" binding:"required,min=1"`
	Tags       []string `json:"tags,omitempty"`
	ScheduleAt string   `json:"schedule_at,omitempty"` // 定时发布时间，ISO8601格式，为空则立即发布
	IsOriginal bool     `json:"is_original,omitempty"` // 是否声明原创
	Visibility string   `json:"visibility,omitempty"`  // 可见范围: "公开可见"(默认), "仅自己可见", "仅互关好友可见"
	Products   []string `json:"products,omitempty"`    // 商品关键词列表，用于绑定带货商品
}

// LoginStatusResponse 登录状态响应
type LoginStatusResponse struct {
	IsLoggedIn bool                   `json:"is_logged_in"`
	Stage      xiaohongshu.LoginStage `json:"stage,omitempty"`
	Username   string                 `json:"username,omitempty"` // 当前登录账号的昵称
	UserID     string                 `json:"user_id,omitempty"`  // 用户唯一标识（个人主页 URL 中的 ID）
}

// LoginQrcodeResponse 登录扫码二维码
type LoginQrcodeResponse struct {
	Timeout    string                 `json:"timeout"`
	Active     bool                   `json:"active"`
	IsLoggedIn bool                   `json:"is_logged_in"`
	Stage      xiaohongshu.LoginStage `json:"stage"`
	Site       string                 `json:"site"`
	Img        string                 `json:"img,omitempty"`
}

// PublishResponse 发布响应
type PublishResponse struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Images  int    `json:"images"`
	Status  string `json:"status"`
}

// PublishVideoRequest 发布视频请求（仅支持本地单个视频文件）
type PublishVideoRequest struct {
	Title      string   `json:"title" binding:"required"`
	Content    string   `json:"content" binding:"required"`
	Video      string   `json:"video" binding:"required"`
	Tags       []string `json:"tags,omitempty"`
	ScheduleAt string   `json:"schedule_at,omitempty"` // 定时发布时间，ISO8601格式，为空则立即发布
	Visibility string   `json:"visibility,omitempty"`  // 可见范围: "公开可见"(默认), "仅自己可见", "仅互关好友可见"
	Products   []string `json:"products,omitempty"`    // 商品关键词列表，用于绑定带货商品
}

// PublishVideoResponse 发布视频响应
type PublishVideoResponse struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Video   string `json:"video"`
	Status  string `json:"status"`
}

// FeedsListResponse Feeds列表响应
type FeedsListResponse struct {
	Feeds []xiaohongshu.Feed `json:"feeds"`
	Count int                `json:"count"`
}

// UserProfileResponse 用户主页响应
type UserProfileResponse struct {
	UserBasicInfo xiaohongshu.UserBasicInfo      `json:"userBasicInfo"`
	Interactions  []xiaohongshu.UserInteractions `json:"interactions"`
	Feeds         []xiaohongshu.Feed             `json:"feeds"`
}

// DeleteCookies 删除 cookies 文件，用于登录重置
func (s *XiaohongshuService) DeleteCookies(ctx context.Context) error {
	cookiePath := cookies.GetCookiesFilePathForSite(xiaohongshu.Site().Name)
	cookieLoader := cookies.NewLoadCookie(cookiePath)
	return cookieLoader.DeleteCookies()
}

// CheckLoginStatus 检查登录状态
func (s *XiaohongshuService) CheckLoginStatus(ctx context.Context) (*LoginStatusResponse, error) {
	if session := s.logins.active(); session != nil {
		state, err := session.currentState(ctx)
		if err == nil {
			state = publicLoginState(state)
			return &LoginStatusResponse{
				IsLoggedIn: state.Stage == xiaohongshu.LoginStageLoggedIn,
				Stage:      state.Stage,
			}, nil
		}
		if !errors.Is(err, errLoginSessionClosed) {
			return nil, err
		}
	}
	if state, ok := s.logins.recentState(loginStateCacheTTL); ok {
		return &LoginStatusResponse{
			IsLoggedIn: state.Stage == xiaohongshu.LoginStageLoggedIn,
			Stage:      state.Stage,
		}, nil
	}

	return withReadAccess(s, ctx, "检查登录状态", func() (*LoginStatusResponse, error) {
		b := newBrowser()
		defer b.Close()

		page := b.NewPage()
		defer page.Close()

		loginAction := xiaohongshu.NewLogin(page)

		state, err := loginAction.CheckLoginState(ctx)
		if err != nil {
			return nil, err
		}

		response := &LoginStatusResponse{
			IsLoggedIn: state.Stage == xiaohongshu.LoginStageLoggedIn,
			Stage:      state.Stage,
		}
		if response.IsLoggedIn {
			s.logins.remember(state)
		}

		// 已登录时从当前页读取真实账号信息；读不到只记 warn，不影响状态返回。
		if response.IsLoggedIn {
			if user, err := loginAction.CurrentUser(ctx); err != nil {
				logrus.Warnf("failed to get current user info: %v", err)
			} else {
				response.Username = user.Nickname
				response.UserID = user.UserID
			}
		}

		return response, nil
	})
}

// GetLoginQrcode 获取登录的扫码二维码
func (s *XiaohongshuService) GetLoginQrcode(ctx context.Context) (*LoginQrcodeResponse, error) {
	if response, found, err := s.currentLoginQrcode(ctx); err != nil {
		return response, err
	} else if found {
		return response, nil
	}

	return withReadAccess(s, ctx, "获取登录二维码", func() (*LoginQrcodeResponse, error) {
		if response, found, err := s.currentLoginQrcode(ctx); err != nil {
			return response, err
		} else if found {
			return response, nil
		}

		b := newBrowser()
		page := b.NewPage()

		closeBrowser := func() {
			_ = page.Close()
			b.Close()
		}

		loginAction := xiaohongshu.NewLogin(page)

		state, err := loginAction.FetchLoginState(ctx)
		if err != nil {
			closeBrowser()
			return nil, err
		}
		if state.Stage == xiaohongshu.LoginStageLoggedIn {
			closeBrowser()
			s.logins.remember(state)
			return loginQrcodeResponse(state, 0, false), nil
		}
		if state.QRCode == "" {
			closeBrowser()
			return nil, fmt.Errorf("登录页面没有可用二维码，当前阶段: %s", state.Stage)
		}

		timeout := loginSessionTimeout
		expiresAt := time.Now().Add(timeout)
		ctxTimeout, cancel := context.WithDeadline(context.Background(), expiresAt)
		session := newLoginSession(
			expiresAt,
			cancel,
			loginAction.CurrentState,
			func() error { return saveCookies(page) },
			closeBrowser,
		)
		seq := s.logins.start(session)
		s.waitScanInBackground(ctxTimeout, session, seq, timeout)

		return loginQrcodeResponse(state, timeout, true), nil
	})
}

func (s *XiaohongshuService) LoginSessionState(
	ctx context.Context,
) (*LoginQrcodeResponse, error) {
	if response, found, err := s.currentLoginQrcode(ctx); found || err != nil {
		return response, err
	}
	return loginQrcodeResponse(
		xiaohongshu.LoginState{Stage: xiaohongshu.LoginStageIdle},
		0,
		false,
	), nil
}

func (s *XiaohongshuService) currentLoginQrcode(
	ctx context.Context,
) (*LoginQrcodeResponse, bool, error) {
	session := s.logins.active()
	if session != nil {
		state, err := session.currentState(ctx)
		if err == nil {
			state = publicLoginState(state)
			return loginQrcodeResponse(state, session.remaining(), true), true, nil
		}
		if !errors.Is(err, errLoginSessionClosed) {
			return nil, true, err
		}
	}

	if state, ok := s.logins.consumeRecentState(
		loginStateCacheTTL,
		xiaohongshu.LoginStagePersistenceFailed,
	); ok {
		return loginQrcodeResponse(state, 0, false), true, nil
	}
	if state, ok := s.logins.recentState(loginStateCacheTTL); ok {
		return loginQrcodeResponse(state, 0, false), true, nil
	}
	return nil, false, nil
}

func publicLoginState(state xiaohongshu.LoginState) xiaohongshu.LoginState {
	if state.Stage == xiaohongshu.LoginStageLoggedIn {
		return xiaohongshu.LoginState{Stage: xiaohongshu.LoginStageVerifying}
	}
	return state
}

func loginQrcodeResponse(
	state xiaohongshu.LoginState,
	timeout time.Duration,
	active bool,
) *LoginQrcodeResponse {
	if timeout < 0 {
		timeout = 0
	}
	return &LoginQrcodeResponse{
		Timeout:    timeout.Round(time.Second).String(),
		Active:     active,
		IsLoggedIn: state.Stage == xiaohongshu.LoginStageLoggedIn,
		Stage:      state.Stage,
		Site:       xiaohongshu.Site().Name,
		Img:        state.QRCode,
	}
}

// waitScanInBackground 等到最终登录态后才保存 cookies。
func (s *XiaohongshuService) waitScanInBackground(
	ctx context.Context,
	session *loginSession,
	seq uint64,
	timeout time.Duration,
) {
	logrus.Infof("等待扫码登录，会话 #%d，超时 %s", seq, timeout)

	go func() {
		completed := false
		defer func() {
			if !completed {
				s.logins.finish(seq)
			}
		}()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		lastStage := xiaohongshu.LoginStageUnknown
		reportedError := false
		var loggedSince time.Time
		for {
			select {
			case <-ctx.Done():
				logrus.Infof("登录会话 #%d 结束，未完成最终登录", seq)
				return
			case <-ticker.C:
				state, err := session.currentState(ctx)
				if err != nil {
					if errors.Is(err, context.Canceled) ||
						errors.Is(err, context.DeadlineExceeded) ||
						errors.Is(err, errLoginSessionClosed) {
						return
					}
					if !reportedError {
						logrus.Warnf("登录会话 #%d 状态检测失败: %v", seq, err)
						reportedError = true
					}
					continue
				}
				reportedError = false

				if state.Stage != lastStage {
					logLoginStage(seq, state.Stage)
					lastStage = state.Stage
				}
				if state.Stage != xiaohongshu.LoginStageLoggedIn {
					loggedSince = time.Time{}
					continue
				}
				if loggedSince.IsZero() {
					loggedSince = time.Now()
					continue
				}
				if time.Since(loggedSince) < loginStabilityWindow {
					continue
				}

				if err := session.saveCookies(); err != nil {
					logrus.Errorf("候选登录成功但保存 cookies 失败，会话 #%d: %v", seq, err)
					return
				}
				s.logins.remember(xiaohongshu.LoginState{Stage: xiaohongshu.LoginStageVerifying})
				s.logins.finish(seq)
				completed = true

				if err := s.verifyRestoredLogin(); err != nil {
					s.logins.remember(xiaohongshu.LoginState{
						Stage: xiaohongshu.LoginStagePersistenceFailed,
					})
					logrus.Errorf("登录未通过站点会话复验，会话 #%d: %v", seq, err)
					return
				}

				s.logins.remember(state)
				logrus.Infof("最终登录成功，站点会话复验通过，会话 #%d", seq)
				return
			}
		}
	}()
}

func (s *XiaohongshuService) verifyRestoredLogin() error {
	_, err := withReadAccess(
		s,
		context.Background(),
		"verify saved login",
		func() (struct{}, error) {
			return struct{}{}, verifyRestoredLogin()
		},
	)
	return err
}

func verifyRestoredLogin() (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("重开站点浏览器失败: %v", recovered)
		}
	}()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		persistedLoginVerificationLimit,
	)
	defer cancel()

	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	state, err := xiaohongshu.NewLogin(page).CheckLoginState(ctx)
	if err != nil {
		return err
	}
	if state.Stage != xiaohongshu.LoginStageLoggedIn {
		return fmt.Errorf("恢复站点会话后的登录阶段为 %s", state.Stage)
	}
	return nil
}

func logLoginStage(seq uint64, stage xiaohongshu.LoginStage) {
	switch stage {
	case xiaohongshu.LoginStageDeviceVerification:
		logrus.Infof("登录会话 #%d 需要设备安全验证二维码", seq)
	case xiaohongshu.LoginStageWaitingConfirmation:
		logrus.Infof("登录会话 #%d 等待手机端确认", seq)
	case xiaohongshu.LoginStageQRCode:
		logrus.Infof("登录会话 #%d 等待扫描登录二维码", seq)
	case xiaohongshu.LoginStageLoggedIn:
		logrus.Infof("登录会话 #%d 检测到候选登录状态，等待稳定后复验", seq)
	default:
		logrus.Infof("登录会话 #%d 当前阶段未知", seq)
	}
}

// PublishContent 发布内容
func (s *XiaohongshuService) PublishContent(ctx context.Context, req *PublishRequest) (*PublishResponse, error) {
	// 验证标题长度（小红书限制：最大20个字）
	if xhsutil.CalcTitleLength(req.Title) > 20 {
		return nil, fmt.Errorf("标题长度超过限制")
	}

	imagePaths, err := s.processImages(req.Images)
	if err != nil {
		return nil, err
	}

	var scheduleTime *time.Time
	if req.ScheduleAt != "" {
		t, err := time.Parse(time.RFC3339, req.ScheduleAt)
		if err != nil {
			return nil, fmt.Errorf("定时发布时间格式错误，请使用 ISO8601 格式: %v", err)
		}

		// 校验定时发布时间范围：1小时至14天
		now := time.Now()
		minTime := now.Add(1 * time.Hour)
		maxTime := now.Add(14 * 24 * time.Hour)

		if t.Before(minTime) {
			return nil, fmt.Errorf("定时发布时间必须至少在1小时后，当前设置: %s，最早可选: %s",
				t.Format("2006-01-02 15:04"), minTime.Format("2006-01-02 15:04"))
		}
		if t.After(maxTime) {
			return nil, fmt.Errorf("定时发布时间不能超过14天，当前设置: %s，最晚可选: %s",
				t.Format("2006-01-02 15:04"), maxTime.Format("2006-01-02 15:04"))
		}

		scheduleTime = &t
		logrus.Infof("设置定时发布时间: %s", t.Format("2006-01-02 15:04"))
	}

	content := xiaohongshu.PublishImageContent{
		Title:        req.Title,
		Content:      req.Content,
		Tags:         req.Tags,
		ImagePaths:   imagePaths,
		ScheduleTime: scheduleTime,
		IsOriginal:   req.IsOriginal,
		Visibility:   req.Visibility,
		Products:     req.Products,
	}

	if err := s.publishContent(ctx, content); err != nil {
		logrus.Errorf("发布内容失败: title=%s %v", content.Title, err)
		return nil, err
	}

	response := &PublishResponse{
		Title:   req.Title,
		Content: req.Content,
		Images:  len(imagePaths),
		Status:  "发布完成",
	}

	return response, nil
}

// processImages 处理图片列表，支持URL下载和本地路径
func (s *XiaohongshuService) processImages(images []string) ([]string, error) {
	processor := downloader.NewImageProcessor()
	return processor.ProcessImages(images)
}

// publishContent 执行内容发布
func (s *XiaohongshuService) publishContent(ctx context.Context, content xiaohongshu.PublishImageContent) error {
	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	action, err := xiaohongshu.NewPublishImageAction(page)
	if err != nil {
		return err
	}

	return action.Publish(ctx, content)
}

// PublishVideo 发布视频（本地文件）
func (s *XiaohongshuService) PublishVideo(ctx context.Context, req *PublishVideoRequest) (*PublishVideoResponse, error) {
	// 标题长度校验（小红书限制：最大20个字）
	if xhsutil.CalcTitleLength(req.Title) > 20 {
		return nil, fmt.Errorf("标题长度超过限制")
	}

	// 本地视频文件校验
	if req.Video == "" {
		return nil, fmt.Errorf("必须提供本地视频文件")
	}
	if _, err := os.Stat(req.Video); err != nil {
		return nil, fmt.Errorf("视频文件不存在或不可访问: %v", err)
	}

	var scheduleTime *time.Time
	if req.ScheduleAt != "" {
		t, err := time.Parse(time.RFC3339, req.ScheduleAt)
		if err != nil {
			return nil, fmt.Errorf("定时发布时间格式错误，请使用 ISO8601 格式: %v", err)
		}

		// 校验定时发布时间范围：1小时至14天
		now := time.Now()
		minTime := now.Add(1 * time.Hour)
		maxTime := now.Add(14 * 24 * time.Hour)

		if t.Before(minTime) {
			return nil, fmt.Errorf("定时发布时间必须至少在1小时后，当前设置: %s，最早可选: %s",
				t.Format("2006-01-02 15:04"), minTime.Format("2006-01-02 15:04"))
		}
		if t.After(maxTime) {
			return nil, fmt.Errorf("定时发布时间不能超过14天，当前设置: %s，最晚可选: %s",
				t.Format("2006-01-02 15:04"), maxTime.Format("2006-01-02 15:04"))
		}

		scheduleTime = &t
		logrus.Infof("设置定时发布时间: %s", t.Format("2006-01-02 15:04"))
	}

	content := xiaohongshu.PublishVideoContent{
		Title:        req.Title,
		Content:      req.Content,
		Tags:         req.Tags,
		VideoPath:    req.Video,
		ScheduleTime: scheduleTime,
		Visibility:   req.Visibility,
		Products:     req.Products,
	}

	if err := s.publishVideo(ctx, content); err != nil {
		return nil, err
	}

	resp := &PublishVideoResponse{
		Title:   req.Title,
		Content: req.Content,
		Video:   req.Video,
		Status:  "发布完成",
	}
	return resp, nil
}

// publishVideo 执行视频发布
func (s *XiaohongshuService) publishVideo(ctx context.Context, content xiaohongshu.PublishVideoContent) error {
	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	action, err := xiaohongshu.NewPublishVideoAction(page)
	if err != nil {
		return err
	}

	return action.PublishVideo(ctx, content)
}

// ListFeeds 获取Feeds列表
func (s *XiaohongshuService) ListFeeds(ctx context.Context) (*FeedsListResponse, error) {
	return withReadAccess(s, ctx, "读取首页列表", func() (*FeedsListResponse, error) {
		b := newBrowser()
		defer b.Close()

		page := b.NewPage()
		defer page.Close()

		action := xiaohongshu.NewFeedsListAction(page)

		feeds, err := action.GetFeedsList(ctx)
		if err != nil {
			logrus.Errorf("获取 Feeds 列表失败: %v", err)
			return nil, err
		}

		response := &FeedsListResponse{
			Feeds: feeds,
			Count: len(feeds),
		}

		return response, nil
	})
}

func (s *XiaohongshuService) SearchFeeds(ctx context.Context, keyword string, filters ...xiaohongshu.FilterOption) (*FeedsListResponse, error) {
	return withReadAccess(s, ctx, "搜索笔记", func() (*FeedsListResponse, error) {
		b := newBrowser()
		defer b.Close()

		page := b.NewPage()
		defer page.Close()

		action := xiaohongshu.NewSearchAction(page)

		feeds, err := action.Search(ctx, keyword, filters...)
		if err != nil {
			return nil, err
		}

		response := &FeedsListResponse{
			Feeds: feeds,
			Count: len(feeds),
		}

		return response, nil
	})
}

// GetFeedDetail 获取Feed详情
func (s *XiaohongshuService) GetFeedDetail(ctx context.Context, feedID, xsecToken string, loadAllComments bool) (*FeedDetailResponse, error) {
	return s.GetFeedDetailWithConfig(ctx, feedID, xsecToken, loadAllComments, xiaohongshu.DefaultCommentLoadConfig())
}

// GetFeedDetailWithConfig 使用配置获取Feed详情
func (s *XiaohongshuService) GetFeedDetailWithConfig(ctx context.Context, feedID, xsecToken string, loadAllComments bool, config xiaohongshu.CommentLoadConfig) (*FeedDetailResponse, error) {
	if loadAllComments {
		config = s.enforceCommentPolicy(config)
	}

	return withReadAccess(s, ctx, "读取笔记详情", func() (*FeedDetailResponse, error) {
		b := newBrowser()
		defer b.Close()

		page := b.NewPage()
		defer page.Close()

		action := xiaohongshu.NewFeedDetailAction(page)

		result, err := action.GetFeedDetailWithConfig(ctx, feedID, xsecToken, loadAllComments, config)
		if err != nil {
			return nil, err
		}

		response := &FeedDetailResponse{
			FeedID: feedID,
			Data:   result,
		}

		return response, nil
	})
}

// UserProfile 获取用户信息
func (s *XiaohongshuService) UserProfile(ctx context.Context, userID, xsecToken, tab string) (*UserProfileResponse, error) {
	parsed, err := xiaohongshu.ParseProfileTab(tab)
	if err != nil {
		return nil, err
	}

	return withReadAccess(s, ctx, "读取用户主页", func() (*UserProfileResponse, error) {
		b := newBrowser()
		defer b.Close()

		page := b.NewPage()
		defer page.Close()

		action := xiaohongshu.NewUserProfileAction(page)

		result, err := action.UserProfile(ctx, userID, xsecToken, parsed)
		if err != nil {
			return nil, err
		}
		response := &UserProfileResponse{
			UserBasicInfo: result.UserBasicInfo,
			Interactions:  result.Interactions,
			Feeds:         result.Feeds,
		}

		return response, nil
	})
}

func (s *XiaohongshuService) enforceCommentPolicy(config xiaohongshu.CommentLoadConfig) xiaohongshu.CommentLoadConfig {
	defaults := xiaohongshu.DefaultCommentLoadConfig()

	if config.MaxCommentItems <= 0 {
		config.MaxCommentItems = defaults.MaxCommentItems
	}
	if config.MaxCommentItems > s.policy.MaxComments {
		logrus.Infof(
			"访问保护: 一级评论数量已从 %d 调整为 %d",
			config.MaxCommentItems,
			s.policy.MaxComments,
		)
		config.MaxCommentItems = s.policy.MaxComments
	}

	if config.MaxRepliesThreshold <= 0 {
		config.MaxRepliesThreshold = defaults.MaxRepliesThreshold
	}
	if config.ClickMoreReplies && config.MaxRepliesThreshold > s.policy.MaxReplies {
		logrus.Infof(
			"访问保护: 单条评论回复阈值已从 %d 调整为 %d",
			config.MaxRepliesThreshold,
			s.policy.MaxReplies,
		)
		config.MaxRepliesThreshold = s.policy.MaxReplies
	}

	if config.ScrollSpeed != "" && config.ScrollSpeed != "slow" {
		logrus.Infof("访问保护: 评论滚动速度已从 %s 调整为 slow", config.ScrollSpeed)
	}
	config.ScrollSpeed = "slow"

	return config
}

// PostCommentToFeed 发表评论到Feed
func (s *XiaohongshuService) PostCommentToFeed(ctx context.Context, feedID, xsecToken, content string) (*PostCommentResponse, error) {
	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	action := xiaohongshu.NewCommentFeedAction(page)

	if err := action.PostComment(ctx, feedID, xsecToken, content); err != nil {
		return nil, err
	}

	return &PostCommentResponse{FeedID: feedID, Success: true, Message: "评论发表成功"}, nil
}

// LikeFeed 点赞笔记
func (s *XiaohongshuService) LikeFeed(ctx context.Context, feedID, xsecToken string) (*ActionResult, error) {
	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	action := xiaohongshu.NewLikeAction(page)
	if err := action.Like(ctx, feedID, xsecToken); err != nil {
		return nil, err
	}
	return &ActionResult{FeedID: feedID, Success: true, Message: "点赞成功或已点赞"}, nil
}

// UnlikeFeed 取消点赞笔记
func (s *XiaohongshuService) UnlikeFeed(ctx context.Context, feedID, xsecToken string) (*ActionResult, error) {
	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	action := xiaohongshu.NewLikeAction(page)
	if err := action.Unlike(ctx, feedID, xsecToken); err != nil {
		return nil, err
	}
	return &ActionResult{FeedID: feedID, Success: true, Message: "取消点赞成功或未点赞"}, nil
}

// FavoriteFeed 收藏笔记
func (s *XiaohongshuService) FavoriteFeed(ctx context.Context, feedID, xsecToken string) (*ActionResult, error) {
	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	action := xiaohongshu.NewFavoriteAction(page)
	if err := action.Favorite(ctx, feedID, xsecToken); err != nil {
		return nil, err
	}
	return &ActionResult{FeedID: feedID, Success: true, Message: "收藏成功或已收藏"}, nil
}

// UnfavoriteFeed 取消收藏笔记
func (s *XiaohongshuService) UnfavoriteFeed(ctx context.Context, feedID, xsecToken string) (*ActionResult, error) {
	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	action := xiaohongshu.NewFavoriteAction(page)
	if err := action.Unfavorite(ctx, feedID, xsecToken); err != nil {
		return nil, err
	}
	return &ActionResult{FeedID: feedID, Success: true, Message: "取消收藏成功或未收藏"}, nil
}

// ReplyCommentToFeed 回复指定评论
func (s *XiaohongshuService) ReplyCommentToFeed(ctx context.Context, feedID, xsecToken, commentID, userID, content string) (*ReplyCommentResponse, error) {
	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	action := xiaohongshu.NewCommentFeedAction(page)

	if err := action.ReplyToComment(ctx, feedID, xsecToken, commentID, userID, content); err != nil {
		return nil, err
	}

	return &ReplyCommentResponse{
		FeedID:          feedID,
		TargetCommentID: commentID,
		TargetUserID:    userID,
		Success:         true,
		Message:         "评论回复成功",
	}, nil
}

// GetUnreadCount 获取通知未读数
func (s *XiaohongshuService) GetUnreadCount(ctx context.Context) (*xiaohongshu.NotificationCount, error) {
	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	return xiaohongshu.NewNotificationAction(page).UnreadCount(ctx)
}

// ListNotifications 获取指定分区的通知列表
func (s *XiaohongshuService) ListNotifications(ctx context.Context, tab string, limit int) (*xiaohongshu.NotificationList, error) {
	parsed, err := xiaohongshu.ParseNotificationTab(tab)
	if err != nil {
		return nil, err
	}

	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	return xiaohongshu.NewNotificationAction(page).List(ctx, parsed, limit)
}

// LikeNotification 给通知里的评论点赞或取消点赞
func (s *XiaohongshuService) LikeNotification(ctx context.Context, commentID string, unlike bool) (*xiaohongshu.NotificationLikeResult, error) {
	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	return xiaohongshu.NewNotificationAction(page).Like(ctx, commentID, unlike)
}

// ReplyNotification 在通知页就地回复评论
func (s *XiaohongshuService) ReplyNotification(ctx context.Context, commentID, content string) (*xiaohongshu.NotificationReplyResult, error) {
	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	return xiaohongshu.NewNotificationAction(page).Reply(ctx, commentID, content)
}

func newBrowser() *browser.Browser {
	return browser.NewBrowser(configs.IsHeadless(),
		browser.WithBrowserBinary(configs.BrowserBin(), configs.BrowserSourceFingerprint()),
		browser.WithFingerprintSeed(configs.FingerprintSeed()),
		browser.WithProxy(configs.Proxy()),
		browser.WithSite(xiaohongshu.Site().Name),
	)
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

// withBrowserPage 执行需要浏览器页面的操作的通用函数
func withBrowserPage(fn func(*rod.Page) error) error {
	b := newBrowser()
	defer b.Close()

	page := b.NewPage()
	defer page.Close()

	return fn(page)
}

// GetMyProfile 获取当前登录用户的个人信息
func (s *XiaohongshuService) GetMyProfile(ctx context.Context, tab string) (*UserProfileResponse, error) {
	parsed, err := xiaohongshu.ParseProfileTab(tab)
	if err != nil {
		return nil, err
	}

	var result *xiaohongshu.UserProfileResponse

	err = withBrowserPage(func(page *rod.Page) error {
		action := xiaohongshu.NewUserProfileAction(page)
		result, err = action.GetMyProfileViaSidebar(ctx, parsed)
		return err
	})

	if err != nil {
		return nil, err
	}

	response := &UserProfileResponse{
		UserBasicInfo: result.UserBasicInfo,
		Interactions:  result.Interactions,
		Feeds:         result.Feeds,
	}

	return response, nil
}
