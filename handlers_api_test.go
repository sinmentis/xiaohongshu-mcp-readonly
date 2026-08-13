package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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
