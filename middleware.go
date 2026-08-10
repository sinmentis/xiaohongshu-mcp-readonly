package main

import (
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// localRequestMiddleware rejects non-loopback hosts, cross-site browser
// requests, cross-origin requests, and simple form POSTs.
func localRequestMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isLoopbackHost(c.Request.Host) {
			respondError(c, http.StatusForbidden, "LOCAL_ONLY",
				"Only local loopback requests are accepted", nil)
			c.Abort()
			return
		}

		if fetchSite := strings.ToLower(c.GetHeader("Sec-Fetch-Site")); fetchSite == "cross-site" &&
			(strings.HasPrefix(c.Request.URL.Path, "/api/") ||
				strings.HasPrefix(c.Request.URL.Path, "/mcp")) {
			respondError(c, http.StatusForbidden, "CROSS_SITE_REQUEST",
				"Cross-site browser requests are rejected", nil)
			c.Abort()
			return
		}

		if origin := c.GetHeader("Origin"); origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil ||
				parsed.Scheme != "http" ||
				!isLoopbackHost(parsed.Host) ||
				!strings.EqualFold(parsed.Host, c.Request.Host) {
				respondError(c, http.StatusForbidden, "CROSS_ORIGIN_REQUEST",
					"Cross-origin requests are rejected", nil)
				c.Abort()
				return
			}
		}

		if c.Request.Method == http.MethodPost {
			mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
			if err != nil || mediaType != "application/json" {
				respondError(c, http.StatusUnsupportedMediaType, "JSON_REQUIRED",
					"POST requests must use application/json", nil)
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

func isLoopbackHost(hostPort string) bool {
	parsed, err := url.Parse("//" + hostPort)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func requestLoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		logrus.WithFields(logrus.Fields{
			"method":  c.Request.Method,
			"path":    c.Request.URL.Path,
			"status":  c.Writer.Status(),
			"latency": time.Since(started),
		}).Info("HTTP request")
	}
}

func errorHandlingMiddleware() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logrus.Errorf("服务器内部错误: %v, path: %s", recovered, c.Request.URL.Path)

		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR",
			"Internal server error", nil)
	})
}
