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

func (s *AppServer) handleCheckLoginStatus(ctx context.Context) *MCPToolResult {
	logrus.Info("MCP: 检查登录状态")

	status, err := s.xiaohongshuService.CheckLoginStatus(ctx)
	if err != nil {
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: "Failed to check login status: " + safeErrorText(err),
			}},
			IsError: true,
		}
	}

	return &MCPToolResult{
		Content: []MCPContent{{
			Type: "text",
			Text: loginStatusText(status),
		}},
	}
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

func (s *AppServer) handleGetLoginQrcode(ctx context.Context) *MCPToolResult {
	logrus.Info("MCP: 获取登录扫码图片")

	result, err := s.xiaohongshuService.GetLoginQrcode(ctx)
	if err != nil {
		return &MCPToolResult{
			Content: []MCPContent{{Type: "text", Text: "Failed to get login QR code: " + safeErrorText(err)}},
			IsError: true,
		}
	}

	if result.IsLoggedIn {
		return &MCPToolResult{
			Content: []MCPContent{{Type: "text", Text: "Already logged in."}},
		}
	}

	now := time.Now()
	deadline := func() string {
		d, err := time.ParseDuration(result.Timeout)
		if err != nil {
			return now.Format("2006-01-02 15:04:05")
		}
		return now.Add(d).Format("2006-01-02 15:04:05")
	}()

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
	return &MCPToolResult{Content: contents}
}

func (s *AppServer) handleListFeeds(ctx context.Context) *MCPToolResult {
	logrus.Info("MCP: 获取Feeds列表")

	result, err := s.xiaohongshuService.ListFeeds(ctx)
	if err != nil {
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: "Failed to list feeds: " + safeErrorText(err),
			}},
			IsError: true,
		}
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: fmt.Sprintf("Feeds were loaded but could not be encoded: %v", err),
			}},
			IsError: true,
		}
	}

	return &MCPToolResult{
		Content: []MCPContent{{
			Type: "text",
			Text: string(jsonData),
		}},
	}
}

func (s *AppServer) handleSearchFeeds(ctx context.Context, args SearchFeedsArgs) *MCPToolResult {
	logrus.Info("MCP: 搜索Feeds")

	if args.Keyword == "" {
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: "Search failed: keyword is required.",
			}},
			IsError: true,
		}
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
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: "Search failed: " + safeErrorText(err),
			}},
			IsError: true,
		}
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: fmt.Sprintf("Search completed but the result could not be encoded: %v", err),
			}},
			IsError: true,
		}
	}

	return &MCPToolResult{
		Content: []MCPContent{{
			Type: "text",
			Text: string(jsonData),
		}},
	}
}

func (s *AppServer) handleGetFeedDetail(ctx context.Context, args FeedDetailArgs) *MCPToolResult {
	logrus.Info("MCP: 获取Feed详情")

	if args.FeedID == "" {
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: "Feed detail failed: feed_id is required.",
			}},
			IsError: true,
		}
	}

	if args.XsecToken == "" {
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: "Feed detail failed: xsec_token is required.",
			}},
			IsError: true,
		}
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
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: "Feed detail failed: " + safeErrorText(err),
			}},
			IsError: true,
		}
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: fmt.Sprintf("Feed detail loaded but could not be encoded: %v", err),
			}},
			IsError: true,
		}
	}

	return &MCPToolResult{
		Content: []MCPContent{{
			Type: "text",
			Text: string(jsonData),
		}},
	}
}

func (s *AppServer) handleUserProfile(ctx context.Context, args map[string]any) *MCPToolResult {
	logrus.Info("MCP: 获取用户主页")

	userID, ok := args["user_id"].(string)
	if !ok || userID == "" {
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: "User profile failed: user_id is required.",
			}},
			IsError: true,
		}
	}

	xsecToken, ok := args["xsec_token"].(string)
	if !ok || xsecToken == "" {
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: "User profile failed: xsec_token is required.",
			}},
			IsError: true,
		}
	}

	tab, _ := args["tab"].(string)

	result, err := s.xiaohongshuService.UserProfile(ctx, userID, xsecToken, tab)
	if err != nil {
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: "User profile failed: " + safeErrorText(err),
			}},
			IsError: true,
		}
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return &MCPToolResult{
			Content: []MCPContent{{
				Type: "text",
				Text: fmt.Sprintf("User profile loaded but could not be encoded: %v", err),
			}},
			IsError: true,
		}
	}

	return &MCPToolResult{
		Content: []MCPContent{{
			Type: "text",
			Text: string(jsonData),
		}},
	}
}
