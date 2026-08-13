package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
	"github.com/sirupsen/logrus"
)

func respondError(c *gin.Context, statusCode int, code, message string, details any) {
	logrus.Errorf("%s %s %d", c.Request.Method, c.Request.URL.Path, statusCode)
	c.JSON(statusCode, ErrorResponse{
		Error:   message,
		Code:    code,
		Details: details,
	})
}

func respondSuccess(c *gin.Context, data any, message string) {
	c.JSON(http.StatusOK, SuccessResponse{
		Success: true,
		Data:    data,
		Message: message,
	})
}

func respondServiceError(
	c *gin.Context,
	fallbackCode string,
	fallbackMessage string,
	err error,
) {
	statusCode := http.StatusInternalServerError
	code := fallbackCode
	message := fallbackMessage

	var operationTimeout *operationTimeoutError
	var queueTimeout *operationQueueTimeoutError
	var gateUnavailable *accessGateUnavailableError
	var browserUnavailable *browserRuntimeUnavailableError

	switch {
	case errors.As(err, &operationTimeout):
		statusCode = http.StatusGatewayTimeout
		code = "OPERATION_TIMEOUT"
		message = "Browser operation timed out"
	case errors.As(err, &queueTimeout):
		statusCode = http.StatusServiceUnavailable
		code = "SERVICE_BUSY"
		message = "Browser operation could not start"
	case errors.As(err, &gateUnavailable):
		statusCode = http.StatusServiceUnavailable
		code = "SERVICE_DEGRADED"
		message = "A previous browser operation is still stopping"
	case errors.As(err, &browserUnavailable):
		statusCode = http.StatusServiceUnavailable
		code = "BROWSER_UNAVAILABLE"
		message = "Browser runtime is unavailable"
	case errors.Is(err, context.DeadlineExceeded):
		statusCode = http.StatusGatewayTimeout
		code = "OPERATION_TIMEOUT"
		message = "Browser operation timed out"
	case errors.Is(err, context.Canceled):
		statusCode = http.StatusRequestTimeout
		code = "REQUEST_CANCELED"
		message = "Request was canceled"
	}

	respondError(c, statusCode, code, message, safeErrorText(err))
}

type accessHealth struct {
	State           string `json:"state"`
	OperationID     uint64 `json:"operation_id,omitempty"`
	Operation       string `json:"operation,omitempty"`
	Phase           string `json:"phase,omitempty"`
	Elapsed         string `json:"elapsed,omitempty"`
	Deadline        string `json:"deadline,omitempty"`
	Remaining       string `json:"remaining,omitempty"`
	Queued          int    `json:"queued"`
	LastFinished    string `json:"last_finished,omitempty"`
	LastOperationID uint64 `json:"last_operation_id,omitempty"`
	LastOperation   string `json:"last_operation,omitempty"`
	LastDuration    string `json:"last_duration,omitempty"`
	LastOutcome     string `json:"last_outcome,omitempty"`
}

type browserHealth struct {
	State       string `json:"state"`
	Launches    uint64 `json:"launches"`
	LastFailure string `json:"last_failure,omitempty"`
}

func (s *AppServer) healthHandler(c *gin.Context) {
	now := time.Now()
	accessSnapshot := s.xiaohongshuService.accessGate.Snapshot()
	browserSnapshot := s.xiaohongshuService.readBrowser.Snapshot()

	status := "healthy"
	statusCode := http.StatusOK
	if accessSnapshot.State == "busy" ||
		browserSnapshot.State == "starting" ||
		browserSnapshot.State == "resetting" {
		status = "busy"
	}
	if accessSnapshot.State == "degraded" ||
		browserSnapshot.State == "degraded" {
		status = "degraded"
		statusCode = http.StatusServiceUnavailable
	}

	access := accessHealth{
		State:           accessSnapshot.State,
		OperationID:     accessSnapshot.OperationID,
		Operation:       accessSnapshot.Operation,
		Phase:           accessSnapshot.Phase,
		Queued:          accessSnapshot.Queued,
		LastOperationID: accessSnapshot.LastOperationID,
		LastOperation:   accessSnapshot.LastOperation,
		LastOutcome:     accessSnapshot.LastOutcome,
		LastDuration:    durationString(accessSnapshot.LastDuration),
	}
	if !accessSnapshot.LastFinished.IsZero() {
		access.LastFinished = accessSnapshot.LastFinished.UTC().Format(time.RFC3339)
	}
	if !accessSnapshot.StartedAt.IsZero() {
		access.Elapsed = durationString(now.Sub(accessSnapshot.StartedAt))
	}
	if !accessSnapshot.Deadline.IsZero() {
		access.Deadline = accessSnapshot.Deadline.UTC().Format(time.RFC3339)
		remaining := time.Until(accessSnapshot.Deadline)
		if remaining < 0 {
			remaining = 0
		}
		access.Remaining = durationString(remaining)
	}

	c.JSON(statusCode, SuccessResponse{
		Success: statusCode == http.StatusOK,
		Data: map[string]any{
			"status":    status,
			"service":   "xiaohongshu-mcp-readonly",
			"version":   version,
			"timestamp": now.UTC().Format(time.RFC3339),
			"access":    access,
			"browser": browserHealth{
				State:       browserSnapshot.State,
				Launches:    browserSnapshot.Launches,
				LastFailure: browserSnapshot.LastFailure,
			},
		},
		Message: "Service health",
	})
}

func durationString(duration time.Duration) string {
	if duration <= 0 {
		return ""
	}
	return duration.Round(time.Second).String()
}

func (s *AppServer) checkLoginStatusHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	status, err := s.xiaohongshuService.CheckLoginStatus(c.Request.Context())
	if err != nil {
		respondServiceError(c, "STATUS_CHECK_FAILED",
			"Failed to check login status", err)
		return
	}
	respondSuccess(c, status, "Login status checked")
}

func (s *AppServer) loginSessionStateHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	result, err := s.xiaohongshuService.LoginSessionState(c.Request.Context())
	if err != nil {
		respondServiceError(c, "LOGIN_SESSION_FAILED",
			"Failed to get login session", err)
		return
	}
	respondSuccess(c, result, "Login session loaded")
}

func (s *AppServer) startLoginSessionHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	result, err := s.xiaohongshuService.GetLoginQrcode(c.Request.Context())
	if err != nil {
		respondServiceError(c, "LOGIN_SESSION_FAILED",
			"Failed to start login session", err)
		return
	}
	respondSuccess(c, result, "Login session started")
}

func (s *AppServer) listFeedsHandler(c *gin.Context) {
	result, err := s.xiaohongshuService.ListFeeds(c.Request.Context())
	if err != nil {
		respondServiceError(c, "LIST_FEEDS_FAILED",
			"Failed to list feeds", err)
		return
	}
	respondSuccess(c, result, "Feeds loaded")
}

func (s *AppServer) searchFeedsHandler(c *gin.Context) {
	var request SearchFeedsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST",
			"Invalid request", err.Error())
		return
	}

	result, err := s.xiaohongshuService.SearchFeeds(
		c.Request.Context(),
		request.Keyword,
		request.Filters,
	)
	if err != nil {
		respondServiceError(c, "SEARCH_FEEDS_FAILED",
			"Search failed", err)
		return
	}
	respondSuccess(c, result, "Search completed")
}

func (s *AppServer) getFeedDetailHandler(c *gin.Context) {
	var request FeedDetailRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST",
			"Invalid request", err.Error())
		return
	}

	var (
		result *FeedDetailResponse
		err    error
	)
	if request.CommentConfig == nil {
		result, err = s.xiaohongshuService.GetFeedDetail(
			c.Request.Context(),
			request.FeedID,
			request.XsecToken,
			request.LoadAllComments,
		)
	} else {
		config := xiaohongshu.CommentLoadConfig{
			ClickMoreReplies:    request.CommentConfig.ClickMoreReplies,
			MaxRepliesThreshold: request.CommentConfig.MaxRepliesThreshold,
			MaxCommentItems:     request.CommentConfig.MaxCommentItems,
			ScrollSpeed:         request.CommentConfig.ScrollSpeed,
		}
		result, err = s.xiaohongshuService.GetFeedDetailWithConfig(
			c.Request.Context(),
			request.FeedID,
			request.XsecToken,
			request.LoadAllComments,
			config,
		)
	}
	if err != nil {
		respondServiceError(c, "GET_FEED_DETAIL_FAILED",
			"Failed to load feed detail", err)
		return
	}
	respondSuccess(c, result, "Feed detail loaded")
}

func (s *AppServer) userProfileHandler(c *gin.Context) {
	var request UserProfileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST",
			"Invalid request", err.Error())
		return
	}

	result, err := s.xiaohongshuService.UserProfile(
		c.Request.Context(),
		request.UserID,
		request.XsecToken,
		request.Tab,
	)
	if err != nil {
		respondServiceError(c, "GET_USER_PROFILE_FAILED",
			"Failed to load user profile", err)
		return
	}
	respondSuccess(c, result, "User profile loaded")
}
