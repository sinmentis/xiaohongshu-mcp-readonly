package main

import (
	"testing"

	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
	"github.com/stretchr/testify/assert"
)

func TestLoginStatusText(t *testing.T) {
	t.Run("logged in with username", func(t *testing.T) {
		text := loginStatusText(&LoginStatusResponse{IsLoggedIn: true, Username: "example"})
		assert.Equal(t, "Logged in as example. Other read-only tools are ready.", text)
	})

	t.Run("logged in without username", func(t *testing.T) {
		text := loginStatusText(&LoginStatusResponse{
			IsLoggedIn: true,
			Stage:      xiaohongshu.LoginStageLoggedIn,
		})
		assert.Equal(t, "Logged in. Other read-only tools are ready.", text)
		assert.NotContains(t, text, " as ")
	})

	t.Run("device verification", func(t *testing.T) {
		text := loginStatusText(&LoginStatusResponse{Stage: xiaohongshu.LoginStageDeviceVerification})
		assert.Contains(t, text, "Device verification")
	})

	t.Run("phone confirmation", func(t *testing.T) {
		text := loginStatusText(&LoginStatusResponse{Stage: xiaohongshu.LoginStageWaitingConfirmation})
		assert.Contains(t, text, "phone confirmation")
	})

	t.Run("saved session verification", func(t *testing.T) {
		text := loginStatusText(&LoginStatusResponse{Stage: xiaohongshu.LoginStageVerifying})
		assert.Contains(t, text, "fresh browser")
	})

	t.Run("saved session verification failed", func(t *testing.T) {
		text := loginStatusText(&LoginStatusResponse{Stage: xiaohongshu.LoginStagePersistenceFailed})
		assert.Contains(t, text, "Start a new login session")
	})

	t.Run("default logged-out prompt", func(t *testing.T) {
		text := loginStatusText(&LoginStatusResponse{Stage: xiaohongshu.LoginStageIdle})
		assert.Contains(t, text, "get_login_qrcode")
	})
}
