package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMCPStatelessSinglePost 固定「/mcp 接受不带 initialize 握手的单次 POST」这一契约。
//
// 契约由 routes.go 里一行 Stateless 支撑，丢掉它编译和其他单测都不会报错。
func TestMCPStatelessSinglePost(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))
	server := httptest.NewServer(router)
	defer server.Close()

	post := func(t *testing.T, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, server.URL+"/mcp", strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	// 只用 tools/list：它不碰浏览器，而契约丢失时它恰好就是失败点——
	// 有状态模式下无 session 调用 tools/list 会被回以「握手前不允许调用」。
	resp := post(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	require.Nil(t, result.Error, "无握手的 tools/list 不应报错")
	assert.NotEmpty(t, result.Result.Tools, "应返回已注册的工具")
}

func TestOnlyReadOnlyToolsRegistered(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))
	server := httptest.NewServer(router)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	names := make([]string, 0, len(result.Result.Tools))
	for _, tool := range result.Result.Tools {
		names = append(names, tool.Name)
	}

	assert.ElementsMatch(t, []string{
		"check_login_status",
		"get_login_qrcode",
		"list_feeds",
		"search_feeds",
		"get_feed_detail",
		"user_profile",
	}, names)
}

func TestOnlyReadOnlyRoutesRegistered(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))

	var registered []string
	for _, r := range router.Routes() {
		registered = append(registered, r.Method+" "+r.Path)
	}

	expected := []string{
		"GET /",
		"GET /favicon.ico",
		"GET /health",
		"GET /login",
		"GET /login/app.js",
		"GET /login/favicon.svg",
		"GET /login/styles.css",
		"POST /api/v1/login/status",
		"GET /api/v1/login/session",
		"POST /api/v1/login/session",
		"POST /api/v1/feeds/list",
		"POST /api/v1/feeds/search",
		"POST /api/v1/feeds/detail",
		"POST /api/v1/user/profile",
	}

	for _, method := range []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodHead,
		http.MethodOptions,
		http.MethodDelete,
		http.MethodConnect,
		http.MethodTrace,
	} {
		expected = append(expected, method+" /mcp", method+" /mcp/*path")
	}

	assert.ElementsMatch(t, expected, registered)
}

func TestLocalRequestMiddlewareRejectsRemoteHost(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Host = "example.com:18060"

	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestLocalRequestMiddlewareRejectsCrossOrigin(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Host = "127.0.0.1:18060"
	request.Header.Set("Origin", "https://example.com")

	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestLocalRequestMiddlewareRejectsCrossSiteBrowserRequest(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/login/session", nil)
	request.Host = "127.0.0.1:18060"
	request.Header.Set("Sec-Fetch-Site", "cross-site")

	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestLocalRequestMiddlewareAcceptsSameLoopbackOrigin(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Host = "127.0.0.1:18060"
	request.Header.Set("Origin", "http://127.0.0.1:18060")

	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
}

func TestLocalRequestMiddlewareRequiresJSONForPost(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/login/session", nil)
	request.Host = "127.0.0.1:18060"

	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusUnsupportedMediaType, recorder.Code)
}
