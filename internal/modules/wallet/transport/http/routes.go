package wallethttp

import (
	"github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/gin-gonic/gin"
)

// RegisterUserRoutes 注册用户钱包端点。
func RegisterUserRoutes(user gin.IRoutes, handler *UserHandler) {
	if user == nil || handler == nil {
		panic("wallet user routes: required dependency is nil")
	}
	// Close customer wallet access, including old clients and recharge links.
	// Keep administrator, channel, callback and refund paths available.
	disabled := func(c *gin.Context) {
		ginutil.RespondError(c, response.CodeForbidden, "error.forbidden", nil)
		c.Abort()
	}
	user.GET("/wallet", disabled)
	user.GET("/wallet/transactions", disabled)
	user.POST("/wallet/payment-channels", disabled)
	user.POST("/wallet/recharge", disabled)
	user.GET("/wallet/recharges", disabled)
	user.GET("/wallet/recharges/stats", disabled)
	user.GET("/wallet/recharges/:recharge_no", disabled)
	user.POST("/wallet/recharge/payments/:id/capture", disabled)
}

// RegisterAdminRoutes 注册后台钱包端点（须挂在 paymentProtected 分组）。
func RegisterAdminRoutes(paymentProtected gin.IRoutes, handler *AdminHandler) {
	if paymentProtected == nil || handler == nil {
		panic("wallet admin routes: required dependency is nil")
	}
	paymentProtected.GET("/users/:id/wallet", handler.GetUserWallet)
	paymentProtected.GET("/users/:id/wallet/transactions", handler.GetUserTransactions)
	paymentProtected.POST("/users/:id/wallet/adjust", handler.AdjustUserWallet)
	paymentProtected.GET("/wallet/recharges", handler.GetRecharges)
}

// RegisterChannelRoutes 注册渠道钱包端点。
func RegisterChannelRoutes(channel gin.IRoutes, handler *ChannelHandler) {
	if channel == nil || handler == nil {
		panic("wallet channel routes: required dependency is nil")
	}
	channel.GET("/wallet", handler.GetWallet)
	channel.GET("/wallet/transactions", handler.GetWalletTransactions)
	channel.POST("/wallet/recharge", handler.CreateWalletRecharge)
}
