package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
	"github.com/stretchr/testify/require"
)

func TestClassifySearchRedirectPublicErrors(t *testing.T) {
	for _, tt := range []struct {
		err    error
		status int
		code   string
		action string
		path   string
	}{
		{xiaohongshu.ErrSearchLoginRequired, http.StatusUnauthorized, "LOGIN_REQUIRED", "restart_login", "/login"},
		{xiaohongshu.ErrSearchVerificationRequired, http.StatusForbidden, "PLATFORM_VERIFICATION_REQUIRED", "complete_platform_verification", ""},
	} {
		t.Run(tt.code, func(t *testing.T) {
			ctx := contextWithRequestID(context.Background(), "req-42")
			wrapped := fmt.Errorf("redirect with sensitive context: %w", tt.err)
			got := classifyPublicError(ctx, "SEARCH_FEEDS_FAILED", "Search failed", wrapped)
			require.Equal(t, tt.status, got.StatusCode)
			require.Equal(t, tt.code, got.Error.Code)
			require.Equal(t, "platform", got.Error.Source)
			require.Equal(t, tt.action, got.Error.NextAction)
			require.Equal(t, tt.path, got.Error.ActionPath)
			require.Equal(t, "req-42", got.Error.RequestID)
			require.False(t, got.Error.Retryable)
			require.Empty(t, got.Error.Details)
		})
	}
}
