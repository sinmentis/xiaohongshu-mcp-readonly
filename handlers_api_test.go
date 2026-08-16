package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondServiceErrorClassifiesOperationTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		respondServiceError(c, "FALLBACK", "fallback", &operationTimeoutError{
			Operation:   "search_feeds",
			OperationID: 7,
			Timeout:     2 * time.Minute,
		})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))

	require.Equal(t, http.StatusGatewayTimeout, recorder.Code)
	var response ErrorResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "OPERATION_TIMEOUT", response.Code)
	assert.Contains(t, response.Details, "inspect /health")
	assert.True(t, response.Retryable)
	assert.Equal(t, actionInspectHealth, response.NextAction)
	assert.Equal(t, healthActionPath, response.ActionPath)
}

func TestRespondServiceErrorClassifiesStuckGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		respondServiceError(c, "FALLBACK", "fallback", &accessGateUnavailableError{
			Operation:   "get_feed_detail",
			OperationID: 9,
		})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	var response ErrorResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "SERVICE_DEGRADED", response.Code)
}

func TestRespondServiceErrorClassifiesInvalidArgument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestLoggingMiddleware())
	router.GET("/test", func(c *gin.Context) {
		respondServiceError(c, "FALLBACK", "fallback", &xiaohongshu.InvalidArgumentError{
			Field:     "tab",
			Value:     "unknown",
			Supported: []string{"note", "fav", "liked"},
		})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/test", nil))

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var response ErrorResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "input", response.Source)
	assert.Equal(t, "INVALID_ARGUMENT", response.Code)
	assert.Equal(t, actionCorrectInput, response.NextAction)
	assert.Equal(t, recorder.Header().Get("X-Request-ID"), response.RequestID)
}
