package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-rod/rod"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/browser"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/configs"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/cookies"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
	"github.com/sirupsen/logrus"
)

type XiaohongshuService struct {
	logins           loginSessions
	accessGate       *accessGate
	readBrowser      *browserRuntime
	readBrowserStale atomic.Bool
	policy           AccessPolicy
}

const (
	loginStateCacheTTL               = 90 * time.Second
	loginSessionTimeout              = 10 * time.Minute
	loginStabilityWindow             = 3 * time.Second
	persistedLoginVerificationLimit  = 90 * time.Second
	checkLoginStatusOperationLimit   = 90 * time.Second
	getLoginQrcodeOperationLimit     = 90 * time.Second
	listFeedsOperationLimit          = 3 * time.Minute
	searchFeedsOperationLimit        = 2 * time.Minute
	feedDetailOperationLimit         = 2 * time.Minute
	feedDetailCommentsOperationLimit = 10 * time.Minute
	userProfileOperationLimit        = 2 * time.Minute
)

func NewXiaohongshuService(policies ...AccessPolicy) *XiaohongshuService {
	policy := DefaultAccessPolicy()
	if len(policies) > 0 {
		policy = policies[0]
		if policy.MaxQueueWait == 0 {
			policy.MaxQueueWait = defaultMaxQueueWait
		}
	}

	service := &XiaohongshuService{
		accessGate: newAccessGate(
			policy.MinInterval,
			policy.MaxJitter,
			policy.MaxQueueWait,
		),
		policy: policy,
	}
	service.readBrowser = newBrowserRuntime(func() browserProcess {
		return newBrowser()
	})
	return service
}

type LoginStatusResponse struct {
	IsLoggedIn bool                   `json:"is_logged_in"`
	Stage      xiaohongshu.LoginStage `json:"stage,omitempty"`
	Username   string                 `json:"username,omitempty"` // 当前登录账号的昵称
	UserID     string                 `json:"user_id,omitempty"`  // 用户唯一标识（个人主页 URL 中的 ID）
}

type LoginQrcodeResponse struct {
	Timeout    string                 `json:"timeout"`
	Active     bool                   `json:"active"`
	IsLoggedIn bool                   `json:"is_logged_in"`
	Stage      xiaohongshu.LoginStage `json:"stage"`
	Site       string                 `json:"site"`
	Img        string                 `json:"img,omitempty"`
}

type FeedsListResponse struct {
	Feeds []xiaohongshu.Feed `json:"feeds"`
	Count int                `json:"count"`
}

type UserProfileResponse struct {
	UserBasicInfo xiaohongshu.UserBasicInfo      `json:"userBasicInfo"`
	Interactions  []xiaohongshu.UserInteractions `json:"interactions"`
	Feeds         []xiaohongshu.Feed             `json:"feeds"`
}

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

	return withReadAccess(
		s,
		ctx,
		"check_login_status",
		checkLoginStatusOperationLimit,
		func(operationCtx context.Context) (*LoginStatusResponse, error) {
			return withServiceReadPage(
				s,
				operationCtx,
				func(page *rod.Page) (*LoginStatusResponse, error) {
					loginAction := xiaohongshu.NewLogin(page)

					state, err := loginAction.CheckLoginState(operationCtx)
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

					if response.IsLoggedIn {
						if user, err := loginAction.CurrentUser(operationCtx); err != nil {
							logrus.WithField("error_type", fmt.Sprintf("%T", err)).
								Warn("Failed to get current user info")
						} else {
							response.Username = user.Nickname
							response.UserID = user.UserID
						}
					}

					return response, nil
				},
			)
		},
	)
}

func (s *XiaohongshuService) GetLoginQrcode(ctx context.Context) (*LoginQrcodeResponse, error) {
	if response, found, err := s.currentLoginQrcode(ctx); err != nil {
		return response, err
	} else if found {
		return response, nil
	}

	return withReadAccess(
		s,
		ctx,
		"get_login_qrcode",
		getLoginQrcodeOperationLimit,
		func(operationCtx context.Context) (*LoginQrcodeResponse, error) {
			if response, found, err := s.currentLoginQrcode(operationCtx); err != nil {
				return response, err
			} else if found {
				return response, nil
			}

			process, page, err := openEphemeralBrowserPage(
				operationCtx,
				func() browserProcess { return newBrowser() },
			)
			if err != nil {
				return nil, err
			}

			var closeOnce sync.Once
			closeBrowser := func() {
				closeOnce.Do(func() {
					closeEphemeralBrowserPage(process, page)
				})
			}

			loginAction := xiaohongshu.NewLogin(page)

			state, err := loginAction.FetchLoginState(operationCtx)
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
		},
	)
}

func (s *XiaohongshuService) Close(ctx context.Context) error {
	loginDone := make(chan struct{})
	go func() {
		s.logins.stopCurrent()
		close(loginDone)
	}()

	browserDone := make(chan error, 1)
	go func() {
		browserDone <- s.readBrowser.Close(ctx)
	}()

	for loginDone != nil || browserDone != nil {
		select {
		case <-loginDone:
			loginDone = nil
		case err := <-browserDone:
			browserDone = nil
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (s *XiaohongshuService) refreshReadBrowser(ctx context.Context) error {
	if !s.readBrowserStale.Swap(false) {
		return nil
	}
	if err := s.readBrowser.ResetAndWait(ctx, "login cookies changed"); err != nil {
		s.readBrowserStale.Store(true)
		return err
	}
	return nil
}

func withServiceReadPage[T any](
	service *XiaohongshuService,
	ctx context.Context,
	fn func(*rod.Page) (T, error),
) (T, error) {
	var zero T
	if err := service.refreshReadBrowser(ctx); err != nil {
		return zero, err
	}
	return withRuntimePage(service.readBrowser, ctx, fn)
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
				s.readBrowserStale.Store(true)
				s.logins.remember(xiaohongshu.LoginState{Stage: xiaohongshu.LoginStageVerifying})
				s.logins.finish(seq)
				completed = true

				if err := s.verifyRestoredLogin(); err != nil {
					if isLoginVerificationInfrastructureError(err) {
						logrus.WithField("error_type", fmt.Sprintf("%T", err)).
							Errorf("登录会话 #%d 的站点会话复验暂时无法完成", seq)
						return
					}
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
	_, err := withReadAccessQueue(
		s,
		context.Background(),
		"verify_saved_login",
		loginSessionTimeout,
		persistedLoginVerificationLimit,
		func(operationCtx context.Context) (struct{}, error) {
			if err := s.refreshReadBrowser(operationCtx); err != nil {
				return struct{}{}, err
			}
			return struct{}{}, verifyRestoredLogin(operationCtx)
		},
	)
	return err
}

func isLoginVerificationInfrastructureError(err error) bool {
	var queueTimeout *operationQueueTimeoutError
	var gateUnavailable *accessGateUnavailableError
	var browserUnavailable *browserRuntimeUnavailableError
	var browserPanic *browserStagePanicError
	var operationTimeout *operationTimeoutError
	return errors.As(err, &queueTimeout) ||
		errors.As(err, &gateUnavailable) ||
		errors.As(err, &browserUnavailable) ||
		errors.As(err, &browserPanic) ||
		errors.As(err, &operationTimeout) ||
		errors.Is(err, context.Canceled)
}

func verifyRestoredLogin(ctx context.Context) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("fresh-browser login verification failed internally")
		}
	}()

	process, page, err := openEphemeralBrowserPage(
		ctx,
		func() browserProcess { return newBrowser() },
	)
	if err != nil {
		return err
	}
	defer closeEphemeralBrowserPage(process, page)

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

func (s *XiaohongshuService) ListFeeds(ctx context.Context) (*FeedsListResponse, error) {
	return withReadAccess(
		s,
		ctx,
		"list_feeds",
		listFeedsOperationLimit,
		func(operationCtx context.Context) (*FeedsListResponse, error) {
			return withServiceReadPage(
				s,
				operationCtx,
				func(page *rod.Page) (*FeedsListResponse, error) {
					action := xiaohongshu.NewFeedsListAction(page)

					feeds, err := action.GetFeedsList(operationCtx)
					if err != nil {
						return nil, err
					}

					return &FeedsListResponse{
						Feeds: feeds,
						Count: len(feeds),
					}, nil
				},
			)
		},
	)
}

func (s *XiaohongshuService) SearchFeeds(ctx context.Context, keyword string, filters ...xiaohongshu.FilterOption) (*FeedsListResponse, error) {
	return withReadAccess(
		s,
		ctx,
		"search_feeds",
		searchFeedsOperationLimit,
		func(operationCtx context.Context) (*FeedsListResponse, error) {
			return withServiceReadPage(
				s,
				operationCtx,
				func(page *rod.Page) (*FeedsListResponse, error) {
					action := xiaohongshu.NewSearchAction(page)

					feeds, err := action.Search(operationCtx, keyword, filters...)
					if err != nil {
						return nil, err
					}

					return &FeedsListResponse{
						Feeds: feeds,
						Count: len(feeds),
					}, nil
				},
			)
		},
	)
}

func (s *XiaohongshuService) GetFeedDetail(ctx context.Context, feedID, xsecToken string, loadAllComments bool) (*FeedDetailResponse, error) {
	return s.GetFeedDetailWithConfig(ctx, feedID, xsecToken, loadAllComments, xiaohongshu.DefaultCommentLoadConfig())
}

func (s *XiaohongshuService) GetFeedDetailWithConfig(ctx context.Context, feedID, xsecToken string, loadAllComments bool, config xiaohongshu.CommentLoadConfig) (*FeedDetailResponse, error) {
	if loadAllComments {
		config = s.enforceCommentPolicy(config)
	}

	timeout := feedDetailOperationLimit
	if loadAllComments {
		timeout = feedDetailCommentsOperationLimit
	}

	return withReadAccess(
		s,
		ctx,
		"get_feed_detail",
		timeout,
		func(operationCtx context.Context) (*FeedDetailResponse, error) {
			return withServiceReadPage(
				s,
				operationCtx,
				func(page *rod.Page) (*FeedDetailResponse, error) {
					action := xiaohongshu.NewFeedDetailAction(page)

					result, err := action.GetFeedDetailWithConfig(
						operationCtx,
						feedID,
						xsecToken,
						loadAllComments,
						config,
					)
					if err != nil {
						return nil, err
					}

					return &FeedDetailResponse{
						FeedID: feedID,
						Data:   result,
					}, nil
				},
			)
		},
	)
}

func (s *XiaohongshuService) UserProfile(ctx context.Context, userID, xsecToken, tab string) (*UserProfileResponse, error) {
	parsed, err := xiaohongshu.ParseProfileTab(tab)
	if err != nil {
		return nil, err
	}

	return withReadAccess(
		s,
		ctx,
		"user_profile",
		userProfileOperationLimit,
		func(operationCtx context.Context) (*UserProfileResponse, error) {
			return withServiceReadPage(
				s,
				operationCtx,
				func(page *rod.Page) (*UserProfileResponse, error) {
					action := xiaohongshu.NewUserProfileAction(page)

					result, err := action.UserProfile(
						operationCtx,
						userID,
						xsecToken,
						parsed,
					)
					if err != nil {
						return nil, err
					}
					return &UserProfileResponse{
						UserBasicInfo: result.UserBasicInfo,
						Interactions:  result.Interactions,
						Feeds:         result.Feeds,
					}, nil
				},
			)
		},
	)
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
