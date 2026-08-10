package main

import (
	"embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed web/favicon.svg web/login.html web/login.css web/login.js
var loginPageAssets embed.FS

func registerLoginPageRoutes(router *gin.Engine) {
	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusTemporaryRedirect, "/login")
	})
	router.GET("/login", serveLoginAsset("web/login.html", "text/html; charset=utf-8"))
	router.GET("/login/styles.css", serveLoginAsset("web/login.css", "text/css; charset=utf-8"))
	router.GET("/login/app.js", serveLoginAsset("web/login.js", "text/javascript; charset=utf-8"))
	router.GET("/login/favicon.svg", serveLoginAsset("web/favicon.svg", "image/svg+xml"))
	router.GET("/favicon.ico", serveLoginAsset("web/favicon.svg", "image/svg+xml"))
}

func serveLoginAsset(path, contentType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := loginPageAssets.ReadFile(path)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}

		c.Header("Cache-Control", "no-store")
		c.Header("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self'; "+
				"script-src 'self'; connect-src 'self'; base-uri 'none'; "+
				"frame-ancestors 'none'; form-action 'none'")
		c.Header("Cross-Origin-Resource-Policy", "same-origin")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, contentType, data)
	}
}
