package xiaohongshu

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-rod/rod"
	"github.com/pkg/errors"
)

const (
	initialLoginQRCodeSelector  = ".login-container .qrcode-img"
	deviceVerificationSelector  = ".r-captcha-modal .qrcode-img"
	loginContainerSelector      = ".login-container"
	loginPageSettleDelay        = 2 * time.Second
	loginStatusCheckSettleDelay = 1 * time.Second
	loginStatusCheckTimeout     = 30 * time.Second
	currentUserReadTimeout      = 10 * time.Second
	loginStatePollingInterval   = 500 * time.Millisecond
)

var errCurrentUserNotFound = errors.New("current user not found in page state")

type LoginStage string

const (
	LoginStageLoggedIn            LoginStage = "logged_in"
	LoginStageDeviceVerification  LoginStage = "device_verification"
	LoginStageWaitingConfirmation LoginStage = "waiting_confirmation"
	LoginStageQRCode              LoginStage = "login_qrcode"
	LoginStageVerifying           LoginStage = "verifying"
	LoginStagePersistenceFailed   LoginStage = "persistence_failed"
	LoginStageIdle                LoginStage = "idle"
	LoginStageUnknown             LoginStage = "unknown"
)

type LoginState struct {
	Stage  LoginStage
	QRCode string
}

type loginPageSignals struct {
	SiteMatched           bool
	Authenticated         bool
	DeviceVerificationQR  string
	InitialLoginQRCode    string
	LoginContainerVisible bool
}

type LoginAction struct {
	page *rod.Page
}

func NewLogin(page *rod.Page) *LoginAction {
	return &LoginAction{page: page}
}

func (a *LoginAction) CheckLoginStatus(ctx context.Context) (bool, error) {
	state, err := a.CheckLoginState(ctx)
	if err != nil {
		return false, err
	}

	return state.Stage == LoginStageLoggedIn, nil
}

func (a *LoginAction) CheckLoginState(ctx context.Context) (LoginState, error) {
	return a.openLoginPage(ctx, loginStatusCheckSettleDelay)
}

// CurrentUser 当前登录用户的基础信息。
type CurrentUser struct {
	Nickname string `json:"nickname"`
	UserID   string `json:"userId"`
}

// CurrentUser 从当前页面的 __INITIAL_STATE__ 读取登录用户信息。
// 需在 CheckLoginStatus 之后调用：复用已加载的 explore 页，不做额外导航。
func (a *LoginAction) CurrentUser(ctx context.Context) (*CurrentUser, error) {
	pp := a.page.Context(ctx).Timeout(currentUserReadTimeout)

	res, err := pp.Eval(`() => {
		const u = window.__INITIAL_STATE__ && window.__INITIAL_STATE__.user;
		const info = u && u.userInfo && u.userInfo.value !== undefined ? u.userInfo.value : (u && u.userInfo);
		if (!info || info.guest) return "";
		return JSON.stringify({nickname: info.nickname, userId: info.userId || info.user_id});
	}`)
	if err != nil {
		return nil, errors.Wrap(err, "read current user state failed")
	}

	raw := res.Value.String()
	if raw == "" {
		return nil, errCurrentUserNotFound
	}

	var user CurrentUser
	if err := json.Unmarshal([]byte(raw), &user); err != nil {
		return nil, errors.Wrap(err, "unmarshal current user failed")
	}

	return &user, nil
}

func (a *LoginAction) Login(ctx context.Context) error {
	state, err := a.openLoginPage(ctx, loginPageSettleDelay)
	if err != nil {
		return err
	}
	if state.Stage == LoginStageLoggedIn {
		return nil
	}
	if !a.WaitForLogin(ctx) {
		return errors.New("login was not completed")
	}
	return nil
}

func (a *LoginAction) FetchQrcodeImage(ctx context.Context) (string, bool, error) {
	state, err := a.FetchLoginState(ctx)
	if err != nil {
		return "", false, err
	}
	if state.Stage == LoginStageLoggedIn {
		return "", true, nil
	}
	if state.QRCode == "" {
		return "", false, errors.Errorf("qrcode is not available in login stage %s", state.Stage)
	}

	return state.QRCode, false, nil
}

func (a *LoginAction) FetchLoginState(ctx context.Context) (LoginState, error) {
	return a.openLoginPage(ctx, loginPageSettleDelay)
}

func (a *LoginAction) WaitForLogin(ctx context.Context) bool {
	ticker := time.NewTicker(loginStatePollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			state, err := a.CurrentState(ctx)
			if err == nil && state.Stage == LoginStageLoggedIn {
				return true
			}
		}
	}
}

func (a *LoginAction) CurrentState(ctx context.Context) (LoginState, error) {
	_, userErr := a.CurrentUser(ctx)
	if userErr != nil && !errors.Is(userErr, errCurrentUserNotFound) {
		return LoginState{}, userErr
	}

	deviceQR, err := visibleImageSource(a.page.Context(ctx), deviceVerificationSelector)
	if err != nil {
		return LoginState{}, errors.Wrap(err, "read device verification qrcode failed")
	}
	initialQR, err := visibleImageSource(a.page.Context(ctx), initialLoginQRCodeSelector)
	if err != nil {
		return LoginState{}, errors.Wrap(err, "read login qrcode failed")
	}
	loginContainerVisible, err := elementVisible(a.page.Context(ctx), loginContainerSelector)
	if err != nil {
		return LoginState{}, errors.Wrap(err, "read login container failed")
	}

	pageInfo, err := a.page.Context(ctx).Info()
	if err != nil {
		return LoginState{}, errors.Wrap(err, "read login page URL failed")
	}

	return classifyLoginState(loginPageSignals{
		SiteMatched:           Site().MatchesURL(pageInfo.URL),
		Authenticated:         userErr == nil,
		DeviceVerificationQR:  deviceQR,
		InitialLoginQRCode:    initialQR,
		LoginContainerVisible: loginContainerVisible,
	}), nil
}

func (a *LoginAction) openLoginPage(ctx context.Context, settleDelay time.Duration) (LoginState, error) {
	pp := a.page.Context(ctx).Timeout(loginStatusCheckTimeout)
	applySiteLocale(pp)
	if err := pp.Navigate(Site().Home); err != nil {
		return LoginState{}, errors.Wrap(err, "navigate to login page failed")
	}
	if err := pp.WaitLoad(); err != nil {
		return LoginState{}, errors.Wrap(err, "wait for login page failed")
	}

	timer := time.NewTimer(settleDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return LoginState{}, ctx.Err()
	case <-timer.C:
	}

	return a.CurrentState(ctx)
}

// classifyLoginState 判定当前登录阶段。
// 设备验证二维码优先于 __INITIAL_STATE__ 的已认证信号：验证弹窗还在就没真正登录完成。
func classifyLoginState(signals loginPageSignals) LoginState {
	switch {
	case !signals.SiteMatched:
		return LoginState{Stage: LoginStageUnknown}
	case signals.DeviceVerificationQR != "":
		return LoginState{
			Stage:  LoginStageDeviceVerification,
			QRCode: signals.DeviceVerificationQR,
		}
	case signals.Authenticated:
		return LoginState{Stage: LoginStageLoggedIn}
	case signals.InitialLoginQRCode != "":
		return LoginState{
			Stage:  LoginStageQRCode,
			QRCode: signals.InitialLoginQRCode,
		}
	case signals.LoginContainerVisible:
		return LoginState{Stage: LoginStageWaitingConfirmation}
	default:
		return LoginState{Stage: LoginStageUnknown}
	}
}

func visibleImageSource(page *rod.Page, selector string) (string, error) {
	exists, element, err := page.Has(selector)
	if err != nil {
		return "", err
	}
	if !exists || element == nil {
		return "", nil
	}

	visible, err := element.Visible()
	if err != nil {
		return "", err
	}
	if !visible {
		return "", nil
	}

	src, err := element.Attribute("src")
	if err != nil {
		return "", err
	}
	if src == nil {
		return "", nil
	}
	return *src, nil
}

func elementVisible(page *rod.Page, selector string) (bool, error) {
	exists, element, err := page.Has(selector)
	if err != nil || !exists || element == nil {
		return false, err
	}
	return element.Visible()
}
