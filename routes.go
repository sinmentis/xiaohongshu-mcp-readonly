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
	router.Use(localRequestMiddleware())
	router.Use(requestLoggingMiddleware())
	router.Use(errorHandlingMiddleware())

	router.GET("/health", healthHandler)
	registerLoginPageRoutes(router)

	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			return appServer.mcpServer
		},
		&mcp.StreamableHTTPOptions{
			JSONResponse: true,
			// Stateless mode lets clients call tools without retaining an MCP
			// session. Server-initiated sampling, elicitation, and roots are
			// intentionally unavailable.
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
