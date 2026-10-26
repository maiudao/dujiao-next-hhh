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

// RegisterDailyReportRoute is opt-in, loopback-only and uses an independent
// read-only credential. No admin cookie or transaction access is delegated.
func RegisterDailyReportRoute(r gin.IRoutes, reader DailyReader, token string) {
	token = strings.TrimSpace(token)
	if len(token) < 32 {
		return
	}
	gate := make(chan struct{}, 1)
	r.GET("/internal/reports/daily", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() || subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte("Bearer "+token)) != 1 {
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
