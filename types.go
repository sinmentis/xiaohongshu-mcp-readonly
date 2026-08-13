package main

import "github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"

type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Details any    `json:"details,omitempty"`
}

type SuccessResponse struct {
	Success bool   `json:"success"`
	Data    any    `json:"data"`
	Message string `json:"message,omitempty"`
}

type MCPToolResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type MCPContent struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type CommentLoadConfig struct {
	ClickMoreReplies    bool   `json:"click_more_replies,omitempty"`
	MaxRepliesThreshold int    `json:"max_replies_threshold,omitempty"`
	MaxCommentItems     int    `json:"max_comment_items,omitempty"`
	ScrollSpeed         string `json:"scroll_speed,omitempty"`
}

type FeedDetailRequest struct {
	FeedID          string             `json:"feed_id" binding:"required"`
	XsecToken       string             `json:"xsec_token" binding:"required"`
	LoadAllComments bool               `json:"load_all_comments,omitempty"`
	CommentConfig   *CommentLoadConfig `json:"comment_config,omitempty"`
}

type SearchFeedsRequest struct {
	Keyword string                   `json:"keyword" binding:"required"`
	Filters xiaohongshu.FilterOption `json:"filters,omitempty"`
}

type FeedDetailResponse struct {
	FeedID string `json:"feed_id"`
	Data   any    `json:"data"`
}

type UserProfileRequest struct {
	UserID    string `json:"user_id" binding:"required"`
	XsecToken string `json:"xsec_token" binding:"required"`
	Tab       string `json:"tab,omitempty"`
}
