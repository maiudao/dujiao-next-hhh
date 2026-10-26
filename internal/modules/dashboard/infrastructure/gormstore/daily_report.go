package gormstore

import (
	"context"
	"fmt"
	dashboard "github.com/dujiao-next/internal/modules/dashboard/contract"
	"strings"
	"time"
)

func safeReportLabel(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	r := []rune(value)
	if len(r) > 100 {
		r = r[:100]
	}
	return string(r)
}

func (r *Store) BuildDailyExport(ctx context.Context, start, end time.Time, reverseCost bool, threshold int64) (*dashboard.DailyExport, error) {
	r = New(r.db.WithContext(ctx))
	out := &dashboard.DailyExport{}
	add := func(key string, err error, lines ...string) {
		s := dashboard.DailySection{Key: key, State: "ok", Lines: lines}
		if err != nil {
			s.State = "unavailable"
			s.Lines = []string{"统计读取失败，未用0代替"}
		}
		out.Modules = append(out.Modules, s)
	}
	v, err := r.GetOverview(start, end)
	add("orders", err, fmt.Sprintf("昨日创建主订单 %d；截至生成时已付状态 %d / 完成 %d / 待支付 %d / 处理中 %d；新注册 %d / 当前在售商品 %d", v.OrdersTotal, v.PaidOrders, v.CompletedOrders, v.PendingPaymentOrders, v.ProcessingOrders, v.NewUsers, v.ActiveProducts),
		fmt.Sprintf("昨日创建在线支付记录 %d，当前成功 %d / 失败 %d；此为记录创建批次，不是支付发生日流水", v.PaymentsTotal, v.PaymentsSuccess, v.PaymentsFailed))
	var currencies int64
	currencyErr := r.db.Table("orders").Where("deleted_at IS NULL AND parent_id IS NULL").Distinct("currency").Count(&currencies).Error
	p, profitErr := r.GetProfitOverview(start, end)
	if currencyErr != nil {
		profitErr = currencyErr
	}
	if currencies > 1 {
		out.Modules = append(out.Modules, dashboard.DailySection{Key: "profit", State: "partial", Lines: []string{"历史订单存在多币种，暂停合计金额和利润；按币种实收/退款见下一节"}})
	} else {
		cost := p.TotalCost + p.PaymentFee
		if reverseCost {
			cost -= p.RefundedCost
		}
		add("profit", profitErr, fmt.Sprintf("沿用商城会计口径 %s：净收入 %.2f / 成本含手续费 %.2f / 利润 %.2f / 手续费 %.2f；退款冲成本=%t", safeReportLabel(v.Currency), p.TotalRevenue, cost, p.TotalRevenue-cost, p.PaymentFee, reverseCost),
			"上述收入/商品成本按订单创建批次，扣除昨日发生退款；不是现金流或昨日真实支付日利润。")
	}
	type moneyRow struct {
		Currency string
		Count    int64
		Amount   float64
	}
	var paid, refunds []moneyRow
	paidWhere, paidArgs := timeRangeQuery(r.db, "paid_at", start, end)
	paidErr := r.db.Table("orders").Select("currency,COUNT(*) AS count,COALESCE(SUM(total_amount),0) AS amount").Where("deleted_at IS NULL AND parent_id IS NULL AND "+paidWhere, paidArgs...).Group("currency").Order("currency").Scan(&paid).Error
	lines := []string{"按 paid_at 统计真实支付主订单，包含钱包成分；不把多次支付尝试累加为订单。"}
	for _, v := range paid {
		lines = append(lines, fmt.Sprintf("%s：昨日支付 %d单 / 订单金额 %.2f", safeReportLabel(v.Currency), v.Count, v.Amount))
	}
	add("paid_events", paidErr, lines...)
	refundWhere, refundArgs := timeRangeQuery(r.db, "rr.created_at", start, end)
	refundErr := r.db.Table("order_refund_records rr").Select("o.currency,COUNT(*) AS count,COALESCE(SUM(rr.amount),0) AS amount").Joins("JOIN orders o ON o.id=rr.order_id").Where("rr.deleted_at IS NULL AND o.deleted_at IS NULL AND "+refundWhere, refundArgs...).Group("o.currency").Order("o.currency").Scan(&refunds).Error
	lines = []string{"退款按退款记录创建日；一次部分退款是一条记录，不等于一张全额退款订单。"}
	for _, v := range refunds {
		lines = append(lines, fmt.Sprintf("%s：昨日退款 %d笔 / 金额 %.2f", safeReportLabel(v.Currency), v.Count, v.Amount))
	}
	add("refund_events", refundErr, lines...)
	stock, stockErr := r.GetStockStats(threshold)
	add("stock", stockErr, fmt.Sprintf("当前缺货商品 %d / 低库存商品 %d / 缺货SKU %d / 低库存SKU %d；可用自动库存数量 %d / 手动库存数量 %d", stock.OutOfStockProducts, stock.LowStockProducts, stock.OutOfStockSKUs, stock.LowStockSKUs, stock.AutoAvailableSecrets, stock.ManualAvailableUnits))
	products, productErr := r.GetTopProducts(start, end, 500)
	lines = []string{"昨日创建且当前处于已付状态的商品/SKU明细（最多500项）；多币种时不展示金额合计。"}
	for _, v := range products {
		line := fmt.Sprintf("商品%d/SKU%d %s：%d单 / %d件", v.ProductID, v.SKUID, safeReportLabel(v.Title), v.PaidOrders, v.Quantity)
		if currencies <= 1 && currencyErr == nil {
			line += fmt.Sprintf(" / 金额%.2f / 商品成本%.2f", v.PaidAmount, v.TotalCost)
		}
		lines = append(lines, line)
	}
	add("products", productErr, lines...)
	if len(products) == 500 {
		out.Modules[len(out.Modules)-1].State = "partial"
	}
	channels, channelErr := r.GetTopChannels(start, end, 100)
	lines = []string{"昨日创建在线支付记录按渠道（最多100项）；不是按支付成功时间归属。"}
	for _, v := range channels {
		lines = append(lines, fmt.Sprintf("渠道%d %s / %s：成功%d / 失败%d", v.ChannelID, safeReportLabel(v.ChannelName), safeReportLabel(v.ChannelType), v.SuccessCount, v.FailedCount))
	}
	add("channels", channelErr, lines...)
	if len(channels) == 100 {
		out.Modules[len(out.Modules)-1].State = "partial"
	}
	var backlog []struct {
		Status string
		Count  int64
	}
	backlogErr := r.db.Table("orders").Select("status,COUNT(*) AS count").Where("deleted_at IS NULL AND parent_id IS NULL AND status IN ?", []string{"pending_payment", "paid", "fulfilling", "partially_delivered", "delivered"}).Group("status").Order("status").Scan(&backlog).Error
	lines = []string{"当前全量待支付/交付/确认积压（不是昨日新增）："}
	for _, v := range backlog {
		lines = append(lines, fmt.Sprintf("%s：%d单", safeReportLabel(v.Status), v.Count))
	}
	add("backlog", backlogErr, lines...)
	balance, balanceErr := r.GetTotalUserBalance()
	if currencies <= 1 && currencyErr == nil {
		add("balance", balanceErr, fmt.Sprintf("当前全站钱包余额合计 %.2f（存量，不是收入）", balance))
	}
	out.Modules = append(out.Modules, dashboard.DailySection{Key: "coverage", State: "partial", Lines: []string{"浏览器SPA页面/商品行为、售后处理时长和真实交付完成事件尚未采集；保留缺口，不从订单状态推断用户已下载。", "不返回客户、订单号、交付载荷、卡密、支付流水原文或管理会话。"}})
	return out, nil
}
