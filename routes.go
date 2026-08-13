package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// setupRoutes registers the complete public HTTP interface.
func setupRoutes(appServer *AppServer) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(requestLoggingMiddleware())
	router.Use(errorHandlingMiddleware())
	router.Use(localRequestMiddleware())

	router.GET("/health", appServer.healthHandler)
	registerLoginPageRoutes(router)

	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			return appServer.mcpServer
		},
		&mcp.StreamableHTTPOptions{
			// Stateless mode lets clients call tools without retaining an MCP
			// session. Server-initiated sampling, elicitation, and roots are
			// intentionally unavailable. SSE responses keep progress
			// notifications on the same request stream.
			Stateless: true,
		},
	)
	router.Any("/mcp", gin.WrapH(mcpHandler))
	router.Any("/mcp/*path", gin.WrapH(mcpHandler))

	api := router.Group("/api/v1")
	{
		api.POST("/login/status", appServer.checkLoginStatusHandler)
		api.GET("/login/session", appServer.loginSessionStateHandler)
		api.POST("/login/session", appServer.startLoginSessionHandler)
		api.POST("/feeds/list", appServer.listFeedsHandler)
		api.POST("/feeds/search", appServer.searchFeedsHandler)
		api.POST("/feeds/detail", appServer.getFeedDetailHandler)
		api.POST("/user/profile", appServer.userProfileHandler)
	}

	return router
}
