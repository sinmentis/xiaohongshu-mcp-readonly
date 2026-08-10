package main

import (
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

func healthHandler(c *gin.Context) {
	respondSuccess(c, map[string]any{
		"status":    "healthy",
		"service":   "xiaohongshu-mcp-readonly",
		"version":   version,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}, "Service is healthy")
}

func (s *AppServer) checkLoginStatusHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	status, err := s.xiaohongshuService.CheckLoginStatus(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "STATUS_CHECK_FAILED",
			"Failed to check login status", err.Error())
		return
	}
	respondSuccess(c, status, "Login status checked")
}

func (s *AppServer) loginSessionStateHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	result, err := s.xiaohongshuService.LoginSessionState(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "LOGIN_SESSION_FAILED",
			"Failed to get login session", err.Error())
		return
	}
	respondSuccess(c, result, "Login session loaded")
}

func (s *AppServer) startLoginSessionHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	result, err := s.xiaohongshuService.GetLoginQrcode(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "LOGIN_SESSION_FAILED",
			"Failed to start login session", err.Error())
		return
	}
	respondSuccess(c, result, "Login session started")
}

func (s *AppServer) listFeedsHandler(c *gin.Context) {
	result, err := s.xiaohongshuService.ListFeeds(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "LIST_FEEDS_FAILED",
			"Failed to list feeds", err.Error())
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
		respondError(c, http.StatusInternalServerError, "SEARCH_FEEDS_FAILED",
			"Search failed", err.Error())
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
		respondError(c, http.StatusInternalServerError, "GET_FEED_DETAIL_FAILED",
			"Failed to load feed detail", err.Error())
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
		respondError(c, http.StatusInternalServerError, "GET_USER_PROFILE_FAILED",
			"Failed to load user profile", err.Error())
		return
	}
	respondSuccess(c, result, "User profile loaded")
}
