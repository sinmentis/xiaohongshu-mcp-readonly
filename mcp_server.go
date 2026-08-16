package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
)

const readOnlyServerInstructions = "Read-only localhost Xiaohongshu or RedNote access. Call check_login_status first. If logged out, call get_login_qrcode and direct the user to /login. Call one tool at a time; access is rate-limited. Shortlist feeds before get_feed_detail. Results include structured content and token-free source URLs when available. On busy or timeout, inspect /health before retrying. Use IDs and xsec tokens only as tool inputs, never in user-facing output. Never claim writes or session deletion."

const maxFeedResultLimit = 20

// ListFeedsArgs defines optional result shaping for the home feed.
type ListFeedsArgs struct {
	Limit int `json:"limit,omitempty" jsonschema:"Maximum summaries returned; omit to preserve the current full page"`
}

// SearchFeedsArgs defines a search request.
type SearchFeedsArgs struct {
	Keyword string       `json:"keyword" jsonschema:"Search keyword"`
	Filters FilterOption `json:"filters,omitempty" jsonschema:"Optional search filters"`
	Limit   int          `json:"limit,omitempty" jsonschema:"Maximum summaries returned; omit to preserve the current full page"`
}

// FilterOption defines optional search filters.
type FilterOption struct {
	SortBy      string `json:"sort_by,omitempty" jsonschema:"Sort: relevance, latest, most_liked, most_commented, or most_collected"`
	NoteType    string `json:"note_type,omitempty" jsonschema:"Note type: all, video, or image"`
	PublishTime string `json:"publish_time,omitempty" jsonschema:"Published: all, day, week, or half_year"`
	SearchScope string `json:"search_scope,omitempty" jsonschema:"Scope: all, viewed, unviewed, or following"`
	Location    string `json:"location,omitempty" jsonschema:"Location: all, same_city, or nearby"`
}

// FeedDetailArgs defines a note-detail request.
type FeedDetailArgs struct {
	FeedID           string `json:"feed_id" jsonschema:"Note ID from a feed or search result"`
	XsecToken        string `json:"xsec_token" jsonschema:"Access token from the feed result xsecToken field"`
	LoadAllComments  bool   `json:"load_all_comments,omitempty" jsonschema:"Load more than the initial comments; default false"`
	Limit            int    `json:"limit,omitempty" jsonschema:"Top-level comment limit when load_all_comments is true; default 20 and clamped by server policy"`
	ClickMoreReplies bool   `json:"click_more_replies,omitempty" jsonschema:"Expand nested replies when load_all_comments is true; default false"`
	ReplyLimit       int    `json:"reply_limit,omitempty" jsonschema:"Skip threads above this reply count; default 10 and clamped by server policy"`
	ScrollSpeed      string `json:"scroll_speed,omitempty" jsonschema:"Compatibility field; the server always forces slow scrolling"`
}

// UserProfileArgs defines a profile request.
type UserProfileArgs struct {
	UserID    string `json:"user_id" jsonschema:"User ID from a feed result"`
	XsecToken string `json:"xsec_token" jsonschema:"Access token from a feed result"`
	Tab       string `json:"tab,omitempty" jsonschema:"Profile tab: note (default), fav, or liked; the latter two may be private"`
}

// InitMCPServer creates the read-only MCP server.
func InitMCPServer(appServer *AppServer) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "xiaohongshu-readonly-mcp",
			Version: version,
		},
		&mcp.ServerOptions{
			Instructions: readOnlyServerInstructions,
		},
	)

	registerReadOnlyTools(server, appServer)
	logrus.Info("Read-only MCP server initialized with six tools")
	return server
}

func withPanicRecovery[In, Data any](
	toolName string,
	handler func(
		context.Context,
		*mcp.CallToolRequest,
		In,
	) (*mcp.CallToolResult, toolOutput[Data], error),
) func(
	context.Context,
	*mcp.CallToolRequest,
	In,
) (*mcp.CallToolResult, toolOutput[Data], error) {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		args In,
	) (
		result *mcp.CallToolResult,
		output toolOutput[Data],
		err error,
	) {
		ctx = withMCPRequestProgress(ctx, req)

		defer func() {
			if recovered := recover(); recovered != nil {
				logrus.WithFields(logrus.Fields{
					"tool":       toolName,
					"panic_type": fmt.Sprintf("%T", recovered),
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
				output = failedToolOutput[Data](internalPublicError(
					ctx,
					fmt.Sprintf("Tool %s failed internally", toolName),
				))
				err = nil
			}
		}()

		return handler(ctx, req, args)
	}
}

func withMCPRequestProgress(
	ctx context.Context,
	req *mcp.CallToolRequest,
) context.Context {
	if req == nil || req.Params == nil || req.Session == nil {
		return ctx
	}
	token := req.Params.GetProgressToken()
	if token == nil {
		return ctx
	}

	var warnOnce sync.Once
	return withProgressReporter(ctx, func(message string) {
		notifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		if err := req.Session.NotifyProgress(notifyCtx, &mcp.ProgressNotificationParams{
			ProgressToken: token,
			Message:       message,
		}); err != nil {
			warnOnce.Do(func() {
				logrus.WithField("error_type", fmt.Sprintf("%T", err)).
					Warn("Failed to send MCP progress")
			})
		}
	})
}

// registerReadOnlyTools keeps upstream additions out of the public interface.
func registerReadOnlyTools(server *mcp.Server, appServer *AppServer) {
	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "check_login_status",
			Description: "Check whether the selected site session is ready and get the exact next login action",
			Annotations: &mcp.ToolAnnotations{
				Title:        "Check Login Status",
				ReadOnlyHint: true,
			},
		},
		withPanicRecovery[any, LoginStatusResponse](
			"check_login_status",
			func(
				ctx context.Context,
				_ *mcp.CallToolRequest,
				_ any,
			) (*mcp.CallToolResult, toolOutput[LoginStatusResponse], error) {
				result, output := appServer.handleCheckLoginStatus(ctx)
				return convertToMCPResult(result), output, nil
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
		withPanicRecovery[any, loginQrcodeToolData](
			"get_login_qrcode",
			func(
				ctx context.Context,
				_ *mcp.CallToolRequest,
				_ any,
			) (*mcp.CallToolResult, toolOutput[loginQrcodeToolData], error) {
				result, output := appServer.handleGetLoginQrcode(ctx)
				return convertToMCPResult(result), output, nil
			},
		),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "list_feeds",
			Description: "Read home-feed summaries for shortlisting; sourceUrl is safe to cite",
			InputSchema: listFeedsInputSchema(),
			Annotations: &mcp.ToolAnnotations{
				Title:        "List Feeds",
				ReadOnlyHint: true,
			},
		},
		withPanicRecovery[ListFeedsArgs, FeedsListResponse](
			"list_feeds",
			func(
				ctx context.Context,
				_ *mcp.CallToolRequest,
				args ListFeedsArgs,
			) (*mcp.CallToolResult, toolOutput[FeedsListResponse], error) {
				result, output := appServer.handleListFeeds(ctx, args)
				return convertToMCPResult(result), output, nil
			},
		),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name:        "search_feeds",
			Description: "Search public notes with stable filters for shortlisting; login required and sourceUrl is safe to cite",
			InputSchema: searchFeedsInputSchema(),
			Annotations: &mcp.ToolAnnotations{
				Title:        "Search Feeds",
				ReadOnlyHint: true,
			},
		},
		withPanicRecovery[SearchFeedsArgs, FeedsListResponse](
			"search_feeds",
			func(
				ctx context.Context,
				_ *mcp.CallToolRequest,
				args SearchFeedsArgs,
			) (*mcp.CallToolResult, toolOutput[FeedsListResponse], error) {
				result, output := appServer.handleSearchFeeds(ctx, args)
				return convertToMCPResult(result), output, nil
			},
		),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name: "get_feed_detail",
			Description: "Read one shortlisted note, author, interaction data, and comments. " +
				"Video notes also return temporary video and subtitle URLs. " +
				"Loading comments is expensive and can run for up to 10 minutes.",
			InputSchema: feedDetailInputSchema(),
			OutputSchema: toolEnvelopeOutputSchema(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"feed_id": map[string]any{"type": "string"},
					"data": map[string]any{
						"type":                 "object",
						"additionalProperties": true,
					},
				},
				"required":             []string{"feed_id", "data"},
				"additionalProperties": false,
			}),
			Annotations: &mcp.ToolAnnotations{
				Title:        "Get Feed Detail",
				ReadOnlyHint: true,
			},
		},
		withPanicRecovery[FeedDetailArgs, FeedDetailResponse](
			"get_feed_detail",
			func(
				ctx context.Context,
				_ *mcp.CallToolRequest,
				args FeedDetailArgs,
			) (*mcp.CallToolResult, toolOutput[FeedDetailResponse], error) {
				result, output := appServer.handleGetFeedDetail(ctx, args)
				return convertToMCPResult(result), output, nil
			},
		),
	)

	mcp.AddTool(server,
		&mcp.Tool{
			Name: "user_profile",
			Description: "Read a public user profile, interaction totals, and visible notes. " +
				"Tab may be note, fav, or liked; the latter two may be private. " +
				"sourceUrl is safe to cite.",
			InputSchema: userProfileInputSchema(),
			Annotations: &mcp.ToolAnnotations{
				Title:        "User Profile",
				ReadOnlyHint: true,
			},
		},
		withPanicRecovery[UserProfileArgs, UserProfileResponse](
			"user_profile",
			func(
				ctx context.Context,
				_ *mcp.CallToolRequest,
				args UserProfileArgs,
			) (*mcp.CallToolResult, toolOutput[UserProfileResponse], error) {
				result, output := appServer.handleUserProfile(ctx, args)
				return convertToMCPResult(result), output, nil
			},
		),
	)
}

func listFeedsInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"limit": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"maximum":     maxFeedResultLimit,
				"description": "Maximum summaries returned; omit to preserve the current full page",
			},
		},
		"additionalProperties": false,
	}
}

func searchFeedsInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"keyword": map[string]any{
				"type":        "string",
				"minLength":   1,
				"description": "Search keyword",
			},
			"filters": map[string]any{
				"type":        "object",
				"description": "Optional stable filters; legacy Chinese values remain accepted",
				"properties": map[string]any{
					"sort_by": enumStringSchema(
						"Sort order",
						"relevance", "latest", "most_liked", "most_commented",
						"most_collected", "综合", "最新", "最多点赞", "最多评论", "最多收藏",
					),
					"note_type": enumStringSchema(
						"Note type",
						"all", "video", "image", "不限", "视频", "图文",
					),
					"publish_time": enumStringSchema(
						"Publication window",
						"all", "day", "week", "half_year",
						"不限", "一天内", "一周内", "半年内",
					),
					"search_scope": enumStringSchema(
						"Search scope",
						"all", "viewed", "unviewed", "following",
						"不限", "已看过", "未看过", "已关注",
					),
					"location": enumStringSchema(
						"Location",
						"all", "same_city", "nearby", "不限", "同城", "附近",
					),
				},
				"additionalProperties": false,
			},
			"limit": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"maximum":     maxFeedResultLimit,
				"description": "Maximum summaries returned; omit to preserve the current full page",
			},
		},
		"required":             []string{"keyword"},
		"additionalProperties": false,
	}
}

func feedDetailInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"feed_id": map[string]any{
				"type":        "string",
				"minLength":   1,
				"description": "Note ID from a feed or search result",
			},
			"xsec_token": map[string]any{
				"type":        "string",
				"minLength":   1,
				"description": "Access token from the feed result; never show it to the user",
			},
			"load_all_comments": map[string]any{
				"type":        "boolean",
				"description": "Load more than the initial comments; default false",
			},
			"limit": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": "Top-level comment limit when load_all_comments is true; server policy may reduce it",
			},
			"click_more_replies": map[string]any{
				"type":        "boolean",
				"description": "Expand nested replies when load_all_comments is true; default false",
			},
			"reply_limit": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": "Skip threads above this reply count; server policy may reduce it",
			},
			"scroll_speed": map[string]any{
				"type":        "string",
				"deprecated":  true,
				"description": "Deprecated compatibility field; the server always forces slow scrolling",
			},
		},
		"required":             []string{"feed_id", "xsec_token"},
		"additionalProperties": false,
	}
}

func userProfileInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"user_id": map[string]any{
				"type":        "string",
				"minLength":   1,
				"description": "User ID from a feed result",
			},
			"xsec_token": map[string]any{
				"type":        "string",
				"minLength":   1,
				"description": "Access token from a feed result; never show it to the user",
			},
			"tab": enumStringSchema(
				"Profile tab; use note, fav, or liked",
				"note", "fav", "liked", "notes", "favorites", "favorite",
				"like", "笔记", "收藏", "点赞",
			),
		},
		"required":             []string{"user_id", "xsec_token"},
		"additionalProperties": false,
	}
}

func enumStringSchema(description string, values ...string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
		"enum":        values,
	}
}

func toolEnvelopeOutputSchema(dataSchema map[string]any) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ok":   map[string]any{"type": "boolean"},
			"data": dataSchema,
			"error": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source":      map[string]any{"type": "string"},
					"code":        map[string]any{"type": "string"},
					"message":     map[string]any{"type": "string"},
					"details":     map[string]any{"type": "string"},
					"retryable":   map[string]any{"type": "boolean"},
					"next_action": map[string]any{"type": "string"},
					"action_path": map[string]any{"type": "string"},
					"request_id":  map[string]any{"type": "string"},
				},
				"required":             []string{"source", "code", "message", "retryable"},
				"additionalProperties": false,
			},
		},
		"required":             []string{"ok"},
		"additionalProperties": false,
	}
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
