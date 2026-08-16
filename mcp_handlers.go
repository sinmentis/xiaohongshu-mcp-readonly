package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
	"github.com/sirupsen/logrus"
)

type loginQrcodeToolData struct {
	Timeout    string                 `json:"timeout"`
	ExpiresAt  string                 `json:"expires_at,omitempty"`
	Active     bool                   `json:"active"`
	IsLoggedIn bool                   `json:"is_logged_in"`
	Stage      xiaohongshu.LoginStage `json:"stage"`
	Site       string                 `json:"site"`
	HasQRCode  bool                   `json:"has_qr_code"`
	NextAction string                 `json:"next_action"`
	ActionPath string                 `json:"action_path,omitempty"`
}

func (s *AppServer) handleCheckLoginStatus(
	ctx context.Context,
) (*MCPToolResult, toolOutput[LoginStatusResponse]) {
	logrus.Info("MCP: 检查登录状态")

	status, err := s.xiaohongshuService.CheckLoginStatus(ctx)
	if err != nil {
		return mcpServiceFailure[LoginStatusResponse](
			ctx,
			"STATUS_CHECK_FAILED",
			"Failed to check login status",
			err,
		)
	}

	return &MCPToolResult{
		Content: []MCPContent{{
			Type: "text",
			Text: loginStatusText(status),
		}},
	}, successfulToolOutput(*status)
}

func loginStatusText(status *LoginStatusResponse) string {
	if status.IsLoggedIn {
		if status.Username == "" {
			return "Logged in. Other read-only tools are ready."
		}
		return fmt.Sprintf(
			"Logged in as %s. Other read-only tools are ready.",
			status.Username,
		)
	}

	switch status.Stage {
	case xiaohongshu.LoginStageDeviceVerification:
		return "Not logged in. Device verification is required; call get_login_qrcode for the second QR code."
	case xiaohongshu.LoginStageWaitingConfirmation:
		return "Not logged in. The QR code was scanned and is waiting for phone confirmation."
	case xiaohongshu.LoginStageVerifying:
		return "Not logged in yet. The saved site cookies are being verified in a fresh browser."
	case xiaohongshu.LoginStagePersistenceFailed:
		return "Login could not be restored from the saved site cookies. Start a new login session."
	default:
		return "Not logged in. Use get_login_qrcode to start login."
	}
}

func (s *AppServer) handleGetLoginQrcode(
	ctx context.Context,
) (*MCPToolResult, toolOutput[loginQrcodeToolData]) {
	logrus.Info("MCP: 获取登录扫码图片")

	result, err := s.xiaohongshuService.GetLoginQrcode(ctx)
	if err != nil {
		return mcpServiceFailure[loginQrcodeToolData](
			ctx,
			"LOGIN_SESSION_FAILED",
			"Failed to get login QR code",
			err,
		)
	}

	data := loginQrcodeToolData{
		Timeout:    result.Timeout,
		ExpiresAt:  result.ExpiresAt,
		Active:     result.Active,
		IsLoggedIn: result.IsLoggedIn,
		Stage:      result.Stage,
		Site:       result.Site,
		HasQRCode:  result.HasQRCode,
		NextAction: result.NextAction,
		ActionPath: result.ActionPath,
	}

	if result.IsLoggedIn {
		return &MCPToolResult{
			Content: []MCPContent{{Type: "text", Text: "Already logged in."}},
		}, successfulToolOutput(data)
	}

	deadline := loginDeadlineText(result)
	message := "Scan this login QR code in the selected app before " + deadline + "."
	switch result.Stage {
	case xiaohongshu.LoginStageDeviceVerification:
		message = "The first scan was confirmed. Scan this device-verification QR code before " + deadline + "."
	case xiaohongshu.LoginStageWaitingConfirmation:
		message = "The login QR code was scanned. Confirm the login on the phone."
	case xiaohongshu.LoginStageVerifying:
		message = "The scan completed. Verifying the saved site cookies in a fresh browser."
	case xiaohongshu.LoginStagePersistenceFailed:
		message = "The saved site cookies did not restore a login. Start a new login session."
	case xiaohongshu.LoginStageUnknown:
		message = "Login is still progressing, but no QR code is currently available."
	}

	contents := []MCPContent{{Type: "text", Text: message}}
	if result.Img != "" {
		contents = append(contents, MCPContent{
			Type:     "image",
			MimeType: "image/png",
			Data:     strings.TrimPrefix(result.Img, "data:image/png;base64,"),
		})
	}
	return &MCPToolResult{Content: contents}, successfulToolOutput(data)
}

func loginDeadlineText(result *LoginQrcodeResponse) string {
	if result.ExpiresAt != "" {
		if deadline, err := time.Parse(time.RFC3339, result.ExpiresAt); err == nil {
			return deadline.Local().Format("2006-01-02 15:04:05")
		}
	}

	now := time.Now()
	duration, err := time.ParseDuration(result.Timeout)
	if err != nil {
		return now.Format("2006-01-02 15:04:05")
	}
	return now.Add(duration).Format("2006-01-02 15:04:05")
}

func (s *AppServer) handleListFeeds(
	ctx context.Context,
	args ListFeedsArgs,
) (*MCPToolResult, toolOutput[FeedsListResponse]) {
	logrus.Info("MCP: 获取Feeds列表")

	result, err := s.xiaohongshuService.ListFeeds(ctx)
	if err != nil {
		return mcpServiceFailure[FeedsListResponse](
			ctx,
			"LIST_FEEDS_FAILED",
			"Failed to list feeds",
			err,
		)
	}

	return mcpJSONSuccess(
		ctx,
		*limitFeedsResponse(result, args.Limit),
		"Feeds were loaded but could not be encoded",
	)
}

func (s *AppServer) handleSearchFeeds(
	ctx context.Context,
	args SearchFeedsArgs,
) (*MCPToolResult, toolOutput[FeedsListResponse]) {
	logrus.Info("MCP: 搜索Feeds")

	if strings.TrimSpace(args.Keyword) == "" {
		return mcpInputFailure[FeedsListResponse](
			ctx,
			"keyword is required",
		)
	}

	filter := xiaohongshu.FilterOption{
		SortBy:      args.Filters.SortBy,
		NoteType:    args.Filters.NoteType,
		PublishTime: args.Filters.PublishTime,
		SearchScope: args.Filters.SearchScope,
		Location:    args.Filters.Location,
	}

	result, err := s.xiaohongshuService.SearchFeeds(ctx, args.Keyword, filter)
	if err != nil {
		return mcpServiceFailure[FeedsListResponse](
			ctx,
			"SEARCH_FEEDS_FAILED",
			"Search failed",
			err,
		)
	}

	return mcpJSONSuccess(
		ctx,
		*limitFeedsResponse(result, args.Limit),
		"Search completed but the result could not be encoded",
	)
}

func (s *AppServer) handleGetFeedDetail(
	ctx context.Context,
	args FeedDetailArgs,
) (*MCPToolResult, toolOutput[FeedDetailResponse]) {
	logrus.Info("MCP: 获取Feed详情")

	if args.FeedID == "" {
		return mcpInputFailure[FeedDetailResponse](ctx, "feed_id is required")
	}
	if args.XsecToken == "" {
		return mcpInputFailure[FeedDetailResponse](ctx, "xsec_token is required")
	}

	config := xiaohongshu.DefaultCommentLoadConfig()
	if args.LoadAllComments {
		config.ClickMoreReplies = args.ClickMoreReplies
		config.MaxCommentItems = defaultPositive(args.Limit, 20)
		config.MaxRepliesThreshold = defaultPositive(args.ReplyLimit, 10)
		if args.ScrollSpeed != "" {
			config.ScrollSpeed = args.ScrollSpeed
		}
	}

	result, err := s.xiaohongshuService.GetFeedDetailWithConfig(
		ctx,
		args.FeedID,
		args.XsecToken,
		args.LoadAllComments,
		config,
	)
	if err != nil {
		return mcpServiceFailure[FeedDetailResponse](
			ctx,
			"GET_FEED_DETAIL_FAILED",
			"Failed to load feed detail",
			err,
		)
	}

	return mcpJSONSuccess(
		ctx,
		*result,
		"Feed detail loaded but could not be encoded",
	)
}

func (s *AppServer) handleUserProfile(
	ctx context.Context,
	args UserProfileArgs,
) (*MCPToolResult, toolOutput[UserProfileResponse]) {
	logrus.Info("MCP: 获取用户主页")

	if args.UserID == "" {
		return mcpInputFailure[UserProfileResponse](ctx, "user_id is required")
	}
	if args.XsecToken == "" {
		return mcpInputFailure[UserProfileResponse](ctx, "xsec_token is required")
	}

	result, err := s.xiaohongshuService.UserProfile(
		ctx,
		args.UserID,
		args.XsecToken,
		args.Tab,
	)
	if err != nil {
		return mcpServiceFailure[UserProfileResponse](
			ctx,
			"GET_USER_PROFILE_FAILED",
			"Failed to load user profile",
			err,
		)
	}

	return mcpJSONSuccess(
		ctx,
		*result,
		"User profile loaded but could not be encoded",
	)
}

func limitFeedsResponse(
	result *FeedsListResponse,
	limit int,
) *FeedsListResponse {
	if limit > maxFeedResultLimit {
		limit = maxFeedResultLimit
	}
	if limit <= 0 || len(result.Feeds) <= limit {
		return result
	}

	limited := *result
	limited.TotalCount = len(result.Feeds)
	limited.Feeds = append([]xiaohongshu.Feed(nil), result.Feeds[:limit]...)
	limited.Count = len(limited.Feeds)
	limited.HasMore = limited.Count < limited.TotalCount
	return &limited
}

func mcpJSONSuccess[T any](
	ctx context.Context,
	data T,
	encodingMessage string,
) (*MCPToolResult, toolOutput[T]) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		publicError := internalPublicError(ctx, encodingMessage)
		publicError.Details = safeErrorText(err)
		return mcpPublicFailure[T](publicError)
	}

	return &MCPToolResult{
		Content: []MCPContent{{
			Type: "text",
			Text: string(jsonData),
		}},
	}, successfulToolOutput(data)
}

func mcpInputFailure[T any](
	ctx context.Context,
	message string,
) (*MCPToolResult, toolOutput[T]) {
	return mcpPublicFailure[T](invalidArgumentPublicError(ctx, message))
}

func mcpServiceFailure[T any](
	ctx context.Context,
	fallbackCode string,
	fallbackMessage string,
	err error,
) (*MCPToolResult, toolOutput[T]) {
	classified := classifyPublicError(ctx, fallbackCode, fallbackMessage, err)
	return mcpPublicFailure[T](classified.Error)
}

func mcpPublicFailure[T any](
	publicError PublicError,
) (*MCPToolResult, toolOutput[T]) {
	text := publicError.Message
	if publicError.Details != "" {
		text += ": " + publicError.Details
	}
	return &MCPToolResult{
		Content: []MCPContent{{
			Type: "text",
			Text: text,
		}},
		IsError: true,
	}, failedToolOutput[T](publicError)
}
