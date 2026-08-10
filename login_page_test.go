package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginPage(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))

	root := httptest.NewRecorder()
	rootRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRequest.Host = "127.0.0.1:18060"
	router.ServeHTTP(root, rootRequest)
	require.Equal(t, http.StatusTemporaryRedirect, root.Code)
	assert.Equal(t, "/login", root.Header().Get("Location"))

	for _, path := range []string{
		"/login",
		"/favicon.ico",
		"/login/styles.css",
		"/login/app.js",
		"/login/favicon.svg",
	} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Host = "127.0.0.1:18060"

			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			assert.Contains(t, recorder.Header().Get("Content-Security-Policy"), "default-src 'self'")
			assert.NotEmpty(t, recorder.Body.String())
		})
	}

}

func TestLoginSessionStateUsesCompletedStateAfterClose(t *testing.T) {
	service := NewXiaohongshuService()
	closed := 0
	session := newLoginSession(
		time.Now().Add(time.Minute),
		func() {},
		func(context.Context) (xiaohongshu.LoginState, error) {
			return xiaohongshu.LoginState{}, nil
		},
		func() error { return nil },
		func() { closed++ },
	)
	service.logins.start(session)
	session.stop()
	service.logins.remember(xiaohongshu.LoginState{Stage: xiaohongshu.LoginStageLoggedIn})

	state, err := service.LoginSessionState(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, closed)
	assert.True(t, state.IsLoggedIn)
	assert.Equal(t, xiaohongshu.LoginStageLoggedIn, state.Stage)
}

func TestPublicLoginStateHidesUnverifiedSuccess(t *testing.T) {
	raw := xiaohongshu.LoginState{
		Stage:  xiaohongshu.LoginStageLoggedIn,
		QRCode: "stale",
	}

	got := publicLoginState(raw)

	assert.Equal(t, xiaohongshu.LoginStageVerifying, got.Stage)
	assert.Empty(t, got.QRCode)
}

func TestLoginSessionStateEndpoint(t *testing.T) {
	service := NewXiaohongshuService()
	router := setupRoutes(NewAppServer(service))

	requestState := func(t *testing.T) LoginQrcodeResponse {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/login/session", nil)
		request.Host = "127.0.0.1:18060"
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)

		var response struct {
			Success bool                `json:"success"`
			Data    LoginQrcodeResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.True(t, response.Success)
		return response.Data
	}

	idle := requestState(t)
	assert.False(t, idle.Active)
	assert.False(t, idle.IsLoggedIn)
	assert.Equal(t, xiaohongshu.LoginStageIdle, idle.Stage)

	service.logins.remember(xiaohongshu.LoginState{Stage: xiaohongshu.LoginStageLoggedIn})
	loggedIn := requestState(t)
	assert.False(t, loggedIn.Active)
	assert.True(t, loggedIn.IsLoggedIn)
	assert.Equal(t, xiaohongshu.LoginStageLoggedIn, loggedIn.Stage)
}
