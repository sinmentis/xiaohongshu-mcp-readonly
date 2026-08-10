package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"runtime/debug"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
)

// SearchFeedsArgs 搜索内容的参数。
type SearchFeedsArgs struct {
	Keyword string       `json:"keyword" jsonschema:"Search keyword"`
	Filters FilterOption `json:"filters,omitempty" jsonschema:"Optional search filters"`
}

// FilterOption 搜索筛选参数。
type FilterOption struct {
	SortBy      string `json:"sort_by,omitempty" jsonschema:"Sort: 综合|最新|最多点赞|最多评论|最多收藏; default 综合"`
	NoteType    string `json:"note_type,omitempty" jsonschema:"Note type: 不限|视频|图文; default 不限"`
	PublishTime string `json:"publish_time,omitempty" jsonschema:"Published: 不限|一天内|一周内|半年内; default 不限"`
	SearchScope string `json:"search_scope,omitempty" jsonschema:"Scope: 不限|已看过|未看过|已关注; default 不限"`
	Location    string `json:"location,omitempty" jsonschema:"Location: 不限|同城|附近; default 不限"`
}

// FeedDetailArgs 获取笔记详情的参数。
type FeedDetailArgs struct {
	FeedID           string `json:"feed_id" jsonschema:"Note ID from a feed or search result"`
	XsecToken        string `json:"xsec_token" jsonschema:"Access token from the feed result xsecToken field"`
	LoadAllComments  bool   `json:"load_all_comments,omitempty" jsonschema:"Load more than the initial comments; default false"`
	Limit            int    `json:"limit,omitempty" jsonschema:"Top-level comment limit when load_all_comments is true; default 20 and clamped by server policy"`
	ClickMoreReplies bool   `json:"click_more_replies,omitempty" jsonschema:"Expand nested replies when load_all_comments is true; default false"`
	ReplyLimit       int    `json:"reply_limit,omitempty" jsonschema:"Skip threads above this reply count; default 10 and clamped by server policy"`
	ScrollSpeed      string `json:"scroll_speed,omitempty" jsonschema:"Compatibility field; the server always forces slow scrolling"`
}

// UserProfileArgs 获取用户主页的参数。
type UserProfileArgs struct {
	UserID    string `json:"user_id" jsonschema:"User ID from a feed result"`
	XsecToken string `json:"xsec_token" jsonschema:"Access token from a feed result"`
	Tab       string `json:"tab,omitempty" jsonschema:"Profile tab: note (default), fav, or liked; the latter two may be private"`
}

// InitMCPServer 创建只读 MCP Server。
func InitMCPServer(appServer *AppServer) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "xiaohongshu-readonly-mcp",
			Version: "0.1.0",
		},
		nil,
	)

	registerReadOnlyTools(server, appServer)
	logrus.Info("只读 MCP Server 已初始化，共开放 6 个工具")
	return server
}

func withPanicRecovery[T any](
	toolName string,
	handler func(context.Context, *mcp.CallToolRequest, T) (*mcp.CallToolResult, any, error),
) func(context.Context, *mcp.CallToolRequest, T) (*mcp.CallToolResult, any, error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		args T,
	) (result *mcp.CallToolResult, response any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logrus.WithFields(logrus.Fields{
					"tool":  toolName,
					"panic": recovered,
				}).Error("Tool handler panicked")
				logrus.Errorf("Stack trace:\n%s", debug.Stack())

				result = &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{
							Text: fmt.Sprintf("Tool %s failed internally. Check server logs.", toolName),
						},
					},
					IsError: true,
				}
				response = nil
				err = nil
			}
		}()

		return handler(ctx, req, args)
	}
}

// registerReadOnlyTools 使用正向列表注册工具，新增上游工具不会自动暴露。
func registerReadOnlyTools(server *mcp.Server, appServer *AppServer) {
	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "check_login_status",
			Description: "Check the current Xiaohongshu or RedNote login session",
			Annotations: &mcp.ToolAnnotations{
				Title:        "Check Login Status",
				ReadOnlyHint: true,
			},
		},
		withPanicRecovery(
			"check_login_status",
			func(
				ctx context.Context,
				_ *mcp.CallToolRequest,
				_ any,
			) (*mcp.CallToolResult, any, error) {
				return convertToMCPResult(appServer.handleCheckLoginStatus(ctx)), nil, nil
			},
		),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "get_login_qrcode",
			Description: "Get the current login QR code, including optional device verification",
			Annotations: &mcp.ToolAnnotations{
				Title:        "Get Login QR Code",
				ReadOnlyHint: true,
			},
		},
		withPanicRecovery(
			"get_login_qrcode",
			func(
				ctx context.Context,
				_ *mcp.CallToolRequest,
				_ any,
			) (*mcp.CallToolResult, any, error) {
				return convertToMCPResult(appServer.handleGetLoginQrcode(ctx)), nil, nil
			},
		),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "list_feeds",
			Description: "Read the home feed",
			Annotations: &mcp.ToolAnnotations{
				Title:        "List Feeds",
				ReadOnlyHint: true,
			},
		},
		withPanicRecovery(
			"list_feeds",
			func(
				ctx context.Context,
				_ *mcp.CallToolRequest,
				_ any,
			) (*mcp.CallToolResult, any, error) {
				return convertToMCPResult(appServer.handleListFeeds(ctx)), nil, nil
			},
		),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "search_feeds",
			Description: "Search Xiaohongshu or RedNote content; login required",
			Annotations: &mcp.ToolAnnotations{
				Title:        "Search Feeds",
				ReadOnlyHint: true,
			},
		},
		withPanicRecovery(
			"search_feeds",
			func(
				ctx context.Context,
				_ *mcp.CallToolRequest,
				args SearchFeedsArgs,
			) (*mcp.CallToolResult, any, error) {
				return convertToMCPResult(appServer.handleSearchFeeds(ctx, args)), nil, nil
			},
		),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name: "get_feed_detail",
			Description: "Read a note, author, interaction data, and comments. " +
				"Video notes also return temporary video and subtitle URLs. " +
				"Set load_all_comments=true to load beyond the initial comments.",
			Annotations: &mcp.ToolAnnotations{
				Title:        "Get Feed Detail",
				ReadOnlyHint: true,
			},
		},
		withPanicRecovery(
			"get_feed_detail",
			func(
				ctx context.Context,
				_ *mcp.CallToolRequest,
				args FeedDetailArgs,
			) (*mcp.CallToolResult, any, error) {
				return convertToMCPResult(appServer.handleGetFeedDetail(ctx, args)), nil, nil
			},
		),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name: "user_profile",
			Description: "Read a public user profile, interaction totals, and visible notes. " +
				"Tab may be note, fav, or liked; the latter two may be private.",
			Annotations: &mcp.ToolAnnotations{
				Title:        "User Profile",
				ReadOnlyHint: true,
			},
		},
		withPanicRecovery(
			"user_profile",
			func(
				ctx context.Context,
				_ *mcp.CallToolRequest,
				args UserProfileArgs,
			) (*mcp.CallToolResult, any, error) {
				argsMap := map[string]any{
					"user_id":    args.UserID,
					"xsec_token": args.XsecToken,
					"tab":        args.Tab,
				}
				return convertToMCPResult(appServer.handleUserProfile(ctx, argsMap)), nil, nil
			},
		),
	)
}

func defaultPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

// convertToMCPResult 将内部结果转换为官方 SDK 格式。
func convertToMCPResult(result *MCPToolResult) *mcp.CallToolResult {
	contents := make([]mcp.Content, 0, len(result.Content))
	for _, content := range result.Content {
		switch content.Type {
		case "text":
			contents = append(contents, &mcp.TextContent{Text: content.Text})
		case "image":
			imageData, err := base64.StdEncoding.DecodeString(content.Data)
			if err != nil {
				logrus.WithError(err).Error("Failed to decode base64 image data")
				contents = append(contents, &mcp.TextContent{
					Text: "Failed to decode image data: " + err.Error(),
				})
				continue
			}
			contents = append(contents, &mcp.ImageContent{
				Data:     imageData,
				MIMEType: content.MimeType,
			})
		}
	}

	return &mcp.CallToolResult{
		Content: contents,
		IsError: result.IsError,
	}
}
