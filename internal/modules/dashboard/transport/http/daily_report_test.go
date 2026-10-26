package dashboardhttp

import (
	"context"
	dashboard "github.com/dujiao-next/internal/modules/dashboard/contract"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

type dailyReaderStub struct{ calls int }

func (s *dailyReaderStub) DailyReport(context.Context, string) (*dashboard.DailyExport, error) {
	s.calls++
	return &dashboard.DailyExport{SchemaVersion: 1}, nil
}
func TestDailyExportOnlyAcceptsLoopbackIndependentBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := strings.Repeat("r", 32)
	for _, fixture := range []struct {
		remote, auth string
		code         int
	}{{"127.0.0.1:1234", "Bearer " + token, 200}, {"[::1]:1234", "Bearer " + token, 200}, {"203.0.113.1:1234", "Bearer " + token, 404}, {"127.0.0.1:1234", "", 404}, {"127.0.0.1:1234", "Bearer wrong", 404}} {
		router := gin.New()
		reader := &dailyReaderStub{}
		RegisterDailyReportRoute(router, reader, token)
		req := httptest.NewRequest("GET", "/internal/reports/daily?date=2026-10-26", nil)
		req.RemoteAddr = fixture.remote
		req.Header.Set("Authorization", fixture.auth)
		req.Header.Set("X-Forwarded-For", "127.0.0.1")
		req.Header.Set("Cookie", "admin=fake")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != fixture.code {
			t.Fatalf("remote=%s status=%d", fixture.remote, w.Code)
		}
		if fixture.code != 200 && reader.calls != 0 {
			t.Fatal("rejected request read data")
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("report cacheable")
		}
	}
	router := gin.New()
	RegisterDailyReportRoute(router, &dailyReaderStub{}, "short")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/internal/reports/daily", nil))
	if w.Code != 404 {
		t.Fatal("weak token enabled route")
	}
}
