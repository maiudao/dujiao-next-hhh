package dashboardhttp

import (
	"context"
	"crypto/subtle"
	dashboard "github.com/dujiao-next/internal/modules/dashboard/contract"
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"strings"
	"time"
)

type DailyReader interface {
	DailyReport(context.Context, string) (*dashboard.DailyExport, error)
}

// RegisterDailyReportRoute uses an independent read-only credential and accepts
// loopback by default. An optional single private peer supports a Docker bridge
// gateway only when the host port is loopback-bound and the public proxy blocks
// this path. Forwarded headers never grant access.
func RegisterDailyReportRoute(r gin.IRoutes, reader DailyReader, token string, containerPeer ...string) {
	token = strings.TrimSpace(token)
	if len(token) < 32 || len(containerPeer) > 1 {
		return
	}
	var peerIP net.IP
	if len(containerPeer) == 1 && strings.TrimSpace(containerPeer[0]) != "" {
		peerIP = net.ParseIP(strings.TrimSpace(containerPeer[0]))
		if peerIP == nil || !peerIP.IsPrivate() {
			return
		}
	}
	gate := make(chan struct{}, 1)
	r.GET("/internal/reports/daily", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
		ip := net.ParseIP(host)
		allowedPeer := ip != nil && (ip.IsLoopback() || (peerIP != nil && ip.Equal(peerIP)))
		if err != nil || !allowedPeer || subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte("Bearer "+token)) != 1 {
			c.Status(http.StatusNotFound)
			return
		}
		select {
		case gate <- struct{}{}:
			defer func() { <-gate }()
		default:
			c.Status(http.StatusTooManyRequests)
			return
		}
		if reader == nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		report, err := reader.DailyReport(ctx, c.Query("date"))
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "daily report unavailable"})
			return
		}
		c.JSON(http.StatusOK, report)
	})
}
