package application

import (
	"errors"
	"testing"

	"github.com/dujiao-next/internal/constants"
	orderapp "github.com/dujiao-next/internal/modules/order/application"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

type storefrontPaymentSettings struct {
	mode string
	err  error
}

func (r *storefrontPaymentSettings) GetByKey(key string) (jsonmap.JSON, bool, error) {
	if r.err != nil {
		return nil, false, r.err
	}
	return jsonmap.JSON{"storefront_mode": r.mode}, key == constants.SettingKeySiteConfig, nil
}
func (r *storefrontPaymentSettings) Upsert(_ string, value jsonmap.JSON) (jsonmap.JSON, error) {
	return value, nil
}

func TestStorefrontClosedBlocksExistingOrderPayments(t *testing.T) {
	for _, balance := range []bool{false, true} {
		svc, db := setupPaymentServiceWalletTest(t)
		svc.settingService = settingsapp.NewService(&storefrontPaymentSettings{mode: "sampling"})
		order := orderdomain.Order{OrderNo: "QA-CLOSED", Status: constants.OrderStatusPendingPayment, Currency: "CNY"}
		if err := db.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		_, err := svc.CreatePayment(CreatePaymentInput{OrderID: order.ID, ChannelID: 1, UseBalance: balance})
		if !errors.Is(err, orderapp.ErrStorefrontPaused) {
			t.Fatalf("balance=%v: error=%v", balance, err)
		}
		var count int64
		db.Model(&paymentdomain.Payment{}).Count(&count)
		if count != 0 {
			t.Fatal("closed request created a payment")
		}
		db.First(&order, order.ID)
		if order.Status != constants.OrderStatusPendingPayment {
			t.Fatal("closed request changed order")
		}
	}
}

func TestStorefrontPaymentSettingsErrorFailsClosed(t *testing.T) {
	svc, db := setupPaymentServiceWalletTest(t)
	failure := errors.New("settings unavailable")
	svc.settingService = settingsapp.NewService(&storefrontPaymentSettings{err: failure})
	order := orderdomain.Order{OrderNo: "QA-UNAVAILABLE", Status: constants.OrderStatusPendingPayment, Currency: "CNY"}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreatePayment(CreatePaymentInput{OrderID: order.ID}); !errors.Is(err, failure) {
		t.Fatalf("error=%v", err)
	}
}

func TestStorefrontPaymentGuardLeavesResellerOrdersAlone(t *testing.T) {
	svc, db := setupPaymentServiceWalletTest(t)
	svc.settingService = settingsapp.NewService(&storefrontPaymentSettings{mode: "sampling"})
	resellerID := uint(12)
	order := orderdomain.Order{OrderNo: "QA-RESELLER", ResellerID: &resellerID, Status: constants.OrderStatusPendingPayment, Currency: "CNY"}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreatePayment(CreatePaymentInput{OrderID: order.ID, ChannelID: 999}); !errors.Is(err, ErrPaymentChannelNotFound) {
		t.Fatalf("error=%v", err)
	}
}
