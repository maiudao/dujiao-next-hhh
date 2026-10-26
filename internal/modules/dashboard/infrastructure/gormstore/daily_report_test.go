package gormstore

import (
	"context"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
	"strings"
	"testing"
	"time"
)

func TestDailyExportSeparatesCreatedPaidRefundAndCurrencies(t *testing.T) {
	repo, db := setupDashboardRepositoryTest(t)
	zone := time.FixedZone("Asia/Shanghai", 28800)
	start := time.Date(2026, 10, 25, 0, 0, 0, 0, zone)
	end := start.AddDate(0, 0, 1)
	paid := start.Add(time.Hour)
	next := end.Add(time.Hour)
	orders := []orderdomain.Order{
		{OrderNo: "private-order-one", UserID: 1, Status: "paid", Currency: "CNY", TotalAmount: money.FromDecimal(decimal.NewFromInt(100)), CreatedAt: start.Add(-time.Hour), PaidAt: &paid},
		{OrderNo: "private-order-two", UserID: 2, Status: "paid", Currency: "USD", TotalAmount: money.FromDecimal(decimal.NewFromInt(50)), CreatedAt: start.Add(-time.Hour), PaidAt: &paid},
		{OrderNo: "private-order-next", UserID: 3, Status: "paid", Currency: "CNY", TotalAmount: money.FromDecimal(decimal.NewFromInt(1000)), CreatedAt: start.Add(time.Hour), PaidAt: &next},
	}
	for i := range orders {
		if err := db.Create(&orders[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, at := range []time.Time{start.Add(time.Hour), end.Add(time.Hour)} {
		r := orderdomain.OrderRefundRecord{OrderID: orders[0].ID, Type: "partial", Currency: "CNY", Amount: money.FromDecimal(decimal.NewFromInt(10)), CreatedAt: at}
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	report, err := repo.BuildDailyExport(context.Background(), start, end, false, 5)
	if err != nil {
		t.Fatal(err)
	}
	sections := map[string]string{}
	states := map[string]string{}
	for _, v := range report.Modules {
		sections[v.Key] = strings.Join(v.Lines, "\n")
		states[v.Key] = v.State
	}
	if !strings.Contains(sections["paid_events"], "CNY：昨日支付 1单 / 订单金额 100.00") || !strings.Contains(sections["paid_events"], "USD：昨日支付 1单 / 订单金额 50.00") {
		t.Fatalf("paid window %s", sections["paid_events"])
	}
	if !strings.Contains(sections["refund_events"], "昨日退款 1笔 / 金额 10.00") {
		t.Fatalf("refund window %s", sections["refund_events"])
	}
	if states["profit"] != "partial" {
		t.Fatal("mixed currency merged")
	}
	for _, v := range sections {
		if strings.Contains(v, "private-order") {
			t.Fatal("private order leaked")
		}
	}
	if err := db.Migrator().DropTable(&orderdomain.OrderRefundRecord{}); err != nil {
		t.Fatal(err)
	}
	report, err = repo.BuildDailyExport(context.Background(), start, end, false, 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range report.Modules {
		if v.Key == "refund_events" && v.State != "unavailable" {
			t.Fatal("failed refund source displayed zero")
		}
		if v.Key == "paid_events" && v.State != "ok" {
			t.Fatal("refund failure poisoned payment module")
		}
	}
}
