package main

import (
	"encoding/json"
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

func TestLoginStatusActions(t *testing.T) {
	loggedIn := finalizeLoginStatus(&LoginStatusResponse{
		IsLoggedIn: true,
		Stage:      xiaohongshu.LoginStageLoggedIn,
	})
	assert.True(t, loggedIn.Ready)
	assert.Equal(t, actionUseReadTools, loggedIn.NextAction)
	assert.Empty(t, loggedIn.ActionPath)

	loggedOut := finalizeLoginStatus(&LoginStatusResponse{
		Stage: xiaohongshu.LoginStageIdle,
	})
	assert.False(t, loggedOut.Ready)
	assert.Equal(t, actionCallLoginTool, loggedOut.NextAction)
	assert.Equal(t, loginActionPath, loggedOut.ActionPath)
}

func TestLimitFeedsResponse(t *testing.T) {
	feeds := make([]xiaohongshu.Feed, 25)
	for index := range feeds {
		feeds[index].ID = string(rune('a' + index))
	}
	full := &FeedsListResponse{Feeds: feeds, Count: len(feeds)}

	limited := limitFeedsResponse(full, 5)
	assert.Len(t, limited.Feeds, 5)
	assert.Equal(t, 5, limited.Count)
	assert.Equal(t, 25, limited.TotalCount)
	assert.True(t, limited.HasMore)
	assert.Len(t, full.Feeds, 25, "the original service result must remain unchanged")

	clamped := limitFeedsResponse(full, 100)
	assert.Len(t, clamped.Feeds, maxFeedResultLimit)
}

func TestLoginQrcodeStructuredDataExcludesImage(t *testing.T) {
	data := loginQrcodeToolData{
		Site:       xiaohongshu.SiteXiaohongshu,
		HasQRCode:  true,
		NextAction: actionScanQRCode,
	}
	encoded, err := json.Marshal(data)
	assert.NoError(t, err)
	assert.NotContains(t, string(encoded), "img")
}
