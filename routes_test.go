package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sinmentis/xiaohongshu-mcp-readonly/xiaohongshu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeMCPResponse(t *testing.T, response *http.Response, target any) {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	if !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		require.NoError(t, json.Unmarshal(body, target))
		return
	}

	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var envelope struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal([]byte(payload), &envelope) != nil || len(envelope.ID) == 0 {
			continue
		}
		require.NoError(t, json.Unmarshal([]byte(payload), target))
		return
	}
	t.Fatalf("MCP response did not contain a JSON-RPC result: %s", body)
}

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
	decodeMCPResponse(t, resp, &result)

	require.Nil(t, result.Error, "无握手的 tools/list 不应报错")
	assert.NotEmpty(t, result.Result.Tools, "应返回已注册的工具")
}

func TestMCPInitializeIncludesReadOnlyInstructions(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))
	server := httptest.NewServer(router)
	defer server.Close()

	request, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/mcp",
		strings.NewReader(`{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "initialize",
			"params": {
				"protocolVersion": "2025-11-25",
				"capabilities": {},
				"clientInfo": {"name": "test", "version": "1.0"}
			}
		}`),
	)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)

	var result struct {
		Result struct {
			Instructions string `json:"instructions"`
			ServerInfo   struct {
				Version string `json:"version"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	decodeMCPResponse(t, response, &result)
	assert.Equal(t, readOnlyServerInstructions, result.Result.Instructions)
	assert.LessOrEqual(t, len(readOnlyServerInstructions), 512)
	assert.Equal(t, version, result.Result.ServerInfo.Version)
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
	decodeMCPResponse(t, resp, &result)

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

func TestMCPToolsExposeStructuredContracts(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))
	server := httptest.NewServer(router)
	defer server.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Result struct {
			Tools []struct {
				Name         string         `json:"name"`
				InputSchema  map[string]any `json:"inputSchema"`
				OutputSchema map[string]any `json:"outputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	decodeMCPResponse(t, resp, &result)

	tools := make(map[string]struct {
		InputSchema  map[string]any
		OutputSchema map[string]any
	}, len(result.Result.Tools))
	for _, tool := range result.Result.Tools {
		tools[tool.Name] = struct {
			InputSchema  map[string]any
			OutputSchema map[string]any
		}{
			InputSchema:  tool.InputSchema,
			OutputSchema: tool.OutputSchema,
		}
		require.NotEmpty(t, tool.OutputSchema, "%s must declare outputSchema", tool.Name)
	}

	search := tools["search_feeds"].InputSchema
	properties := search["properties"].(map[string]any)
	filters := properties["filters"].(map[string]any)
	filterProperties := filters["properties"].(map[string]any)
	sortBy := filterProperties["sort_by"].(map[string]any)
	assert.Contains(t, sortBy["enum"], "relevance")
	assert.Contains(t, sortBy["enum"], "综合")

	limit := properties["limit"].(map[string]any)
	assert.Equal(t, float64(maxFeedResultLimit), limit["maximum"])
}

func TestMCPToolErrorIncludesStructuredRecoveryAction(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))
	server := httptest.NewServer(router)
	defer server.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/mcp",
		strings.NewReader(`{
			"jsonrpc":"2.0",
			"id":1,
			"method":"tools/call",
			"params":{
				"name":"search_feeds",
				"arguments":{"keyword":" "}
			}
		}`),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Result struct {
			IsError           bool `json:"isError"`
			StructuredContent struct {
				OK    bool `json:"ok"`
				Error struct {
					Source     string `json:"source"`
					Code       string `json:"code"`
					Retryable  bool   `json:"retryable"`
					NextAction string `json:"next_action"`
					RequestID  string `json:"request_id"`
				} `json:"error"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	decodeMCPResponse(t, resp, &result)

	assert.True(t, result.Result.IsError)
	assert.False(t, result.Result.StructuredContent.OK)
	assert.Equal(t, "input", result.Result.StructuredContent.Error.Source)
	assert.Equal(t, "INVALID_ARGUMENT", result.Result.StructuredContent.Error.Code)
	assert.False(t, result.Result.StructuredContent.Error.Retryable)
	assert.Equal(t, actionCorrectInput, result.Result.StructuredContent.Error.NextAction)
	assert.Equal(t, resp.Header.Get("X-Request-ID"), result.Result.StructuredContent.Error.RequestID)
}

func TestMCPToolSuccessIncludesStructuredContent(t *testing.T) {
	service := NewXiaohongshuService()
	service.logins.remember(xiaohongshu.LoginState{
		Stage: xiaohongshu.LoginStageLoggedIn,
	})
	router := setupRoutes(NewAppServer(service))
	server := httptest.NewServer(router)
	defer server.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/mcp",
		strings.NewReader(`{
			"jsonrpc":"2.0",
			"id":1,
			"method":"tools/call",
			"params":{
				"name":"check_login_status",
				"arguments":{}
			}
		}`),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Result struct {
			IsError           bool `json:"isError"`
			StructuredContent struct {
				OK   bool `json:"ok"`
				Data struct {
					Ready      bool   `json:"ready"`
					NextAction string `json:"next_action"`
				} `json:"data"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	decodeMCPResponse(t, resp, &result)

	assert.False(t, result.Result.IsError)
	assert.True(t, result.Result.StructuredContent.OK)
	assert.True(t, result.Result.StructuredContent.Data.Ready)
	assert.Equal(t, actionUseReadTools, result.Result.StructuredContent.Data.NextAction)
}

func TestHealthIncludesSiteAndEffectivePolicy(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Host = "127.0.0.1:18060"

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			Site   string `json:"site"`
			Policy struct {
				MinInterval  string `json:"min_interval"`
				MaxJitter    string `json:"max_jitter"`
				MaxQueueWait string `json:"max_queue_wait"`
				MaxComments  int    `json:"max_comments"`
				MaxReplies   int    `json:"max_replies"`
			} `json:"policy"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, xiaohongshu.Site().Name, response.Data.Site)
	assert.Equal(t, "30s", response.Data.Policy.MinInterval)
	assert.Equal(t, "15s", response.Data.Policy.MaxJitter)
	assert.Equal(t, "1m0s", response.Data.Policy.MaxQueueWait)
	assert.Equal(t, 50, response.Data.Policy.MaxComments)
	assert.Equal(t, 10, response.Data.Policy.MaxReplies)
}

func TestMCPProgressUsesRequestStream(t *testing.T) {
	appServer := NewAppServer(NewXiaohongshuService())
	progressServer := mcp.NewServer(
		&mcp.Implementation{Name: "progress-test", Version: "1.0"},
		nil,
	)
	mcp.AddTool(
		progressServer,
		&mcp.Tool{Name: "progress"},
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
			_ any,
		) (*mcp.CallToolResult, any, error) {
			if err := req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
				ProgressToken: req.Params.GetProgressToken(),
				Message:       "working",
			}); err != nil {
				return nil, nil, err
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "done"}},
			}, nil, nil
		},
	)
	appServer.mcpServer = progressServer

	server := httptest.NewServer(setupRoutes(appServer))
	defer server.Close()

	request, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/mcp",
		strings.NewReader(`{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "tools/call",
			"params": {
				"name": "progress",
				"arguments": {},
				"_meta": {"progressToken": "progress-1"}
			}
		}`),
	)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Contains(t, response.Header.Get("Content-Type"), "text/event-stream")

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"method":"notifications/progress"`)
	assert.Contains(t, string(body), `"message":"working"`)
	assert.Contains(t, string(body), `"text":"done"`)
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

func TestHealthReportsRunningOperation(t *testing.T) {
	service := NewXiaohongshuService(AccessPolicy{
		MinInterval:  0,
		MaxJitter:    0,
		MaxQueueWait: time.Second,
		MaxComments:  10,
		MaxReplies:   5,
	})
	router := setupRoutes(NewAppServer(service))

	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- service.accessGate.Run(
			context.Background(),
			"search_feeds",
			time.Second,
			func(context.Context) error {
				<-release
				return nil
			},
		)
	}()
	require.Eventually(t, func() bool {
		return service.accessGate.Snapshot().Phase == "running"
	}, time.Second, 10*time.Millisecond)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Host = "127.0.0.1:18060"
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			Status string       `json:"status"`
			Access accessHealth `json:"access"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "busy", response.Data.Status)
	assert.Equal(t, "search_feeds", response.Data.Access.Operation)
	assert.Equal(t, "running", response.Data.Access.Phase)

	close(release)
	require.NoError(t, <-done)
}

func TestHealthReportsStuckOperation(t *testing.T) {
	service := NewXiaohongshuService(AccessPolicy{
		MinInterval:  0,
		MaxJitter:    0,
		MaxQueueWait: time.Second,
		MaxComments:  10,
		MaxReplies:   5,
	})
	router := setupRoutes(NewAppServer(service))

	release := make(chan struct{})
	err := service.accessGate.Run(
		context.Background(),
		"get_feed_detail",
		20*time.Millisecond,
		func(context.Context) error {
			<-release
			return nil
		},
	)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Host = "127.0.0.1:18060"
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	var response struct {
		Data struct {
			Status string       `json:"status"`
			Access accessHealth `json:"access"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "degraded", response.Data.Status)
	assert.Equal(t, "cancelling", response.Data.Access.Phase)

	close(release)
	require.Eventually(t, func() bool {
		return service.accessGate.Snapshot().State == "idle"
	}, time.Second, 10*time.Millisecond)
}

func TestRequestLoggingAddsRequestID(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Host = "127.0.0.1:18060"

	router.ServeHTTP(recorder, request)

	assert.Regexp(t, `^req-\d+$`, recorder.Header().Get("X-Request-ID"))
}

func TestLocalRequestMiddlewareRequiresJSONForPost(t *testing.T) {
	router := setupRoutes(NewAppServer(NewXiaohongshuService()))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/login/session", nil)
	request.Host = "127.0.0.1:18060"

	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusUnsupportedMediaType, recorder.Code)
}
