package wallethttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/gin-gonic/gin"
)

func TestAdjustUserWalletRequiresRemarkWithDedicatedMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("admin_id", uint(1))
	ctx.Params = gin.Params{{Key: "id", Value: "10"}}
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/users/10/wallet/adjust",
		strings.NewReader(`{"operation":"add","amount":"10.00","remark":"   "}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("Accept-Language", "en-US")

	(&AdminHandler{}).AdjustUserWallet(ctx)

	if !strings.Contains(recorder.Body.String(), "A remark is required for wallet balance adjustments") {
		t.Fatalf("expected dedicated remark validation message, got %s", recorder.Body.String())
	}
}

func TestCustomerWalletRoutesDisabledWithoutBlockingOtherRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	user := router.Group("/api/v1")
	// Empty dependencies ensure no wallet service can be called.
	RegisterUserRoutes(user, &UserHandler{})
	user.GET("/orders", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/wallet"},
		{http.MethodGet, "/wallet/transactions"},
		{http.MethodPost, "/wallet/payment-channels"},
		{http.MethodPost, "/wallet/recharge"},
		{http.MethodGet, "/wallet/recharges"},
		{http.MethodGet, "/wallet/recharges/stats"},
		{http.MethodGet, "/wallet/recharges/WR-OLD"},
		{http.MethodPost, "/wallet/recharge/payments/1/capture"},
	} {
		t.Run(route.method+route.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(route.method, "/api/v1"+route.path, nil))
			var body response.Response
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.StatusCode != response.CodeForbidden {
				t.Fatalf("wallet route must be forbidden: body=%s error=%v", recorder.Body.String(), err)
			}
		})
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("non-wallet route blocked: status=%d", recorder.Code)
	}
}
