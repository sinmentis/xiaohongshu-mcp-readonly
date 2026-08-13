package main

import (
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var requestSequence atomic.Uint64

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
		requestID := fmt.Sprintf("req-%d", requestSequence.Add(1))
		c.Header("X-Request-ID", requestID)

		started := time.Now()
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
		}).Info("HTTP request started")

		c.Next()
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"status":     c.Writer.Status(),
			"latency":    time.Since(started),
		}).Info("HTTP request finished")
	}
}

func errorHandlingMiddleware() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logrus.WithFields(logrus.Fields{
			"panic_type": fmt.Sprintf("%T", recovered),
			"path":       c.Request.URL.Path,
		}).Error("HTTP handler panicked")

		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR",
			"Internal server error", nil)
	})
}
