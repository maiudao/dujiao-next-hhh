package application

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dujiao-next/internal/constants"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	walletdomain "github.com/dujiao-next/internal/modules/wallet/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
)

type walletDisabledSettings struct{ storefrontPaymentSettings }

func (r *walletDisabledSettings) GetByKey(key string) (jsonmap.JSON, bool, error) {
	if key == constants.SettingKeyWalletConfig {
		return jsonmap.JSON{constants.SettingFieldWalletOnlyPayment: true}, true, nil
	}
	return r.storefrontPaymentSettings.GetByKey(key)
}

func TestMainShopWalletDisabled(t *testing.T) {
	for _, walletOnly := range []bool{false, true} {
		for _, useBalance := range []bool{false, true} {
			t.Run(fmt.Sprintf("wallet_only=%v/use_balance=%v", walletOnly, useBalance), func(t *testing.T) {
				svc, db := setupPaymentServiceWalletTest(t)
				if walletOnly {
					svc.settingService = settingsapp.NewService(&walletDisabledSettings{})
				}
				user := userdomain.User{Email: "wallet-disabled@example.test", PasswordHash: "hash", Status: constants.UserStatusActive}
				if err := db.Create(&user).Error; err != nil {
					t.Fatal(err)
				}
				account := walletdomain.Account{UserID: user.ID, Balance: money.FromDecimal(decimal.NewFromInt(100))}
				order := orderdomain.Order{OrderNo: "SHOP-WALLET-DISABLED", UserID: user.ID,
					Status: constants.OrderStatusPendingPayment, Currency: "CNY",
					TotalAmount:      money.FromDecimal(decimal.NewFromInt(50)),
					OnlinePaidAmount: money.FromDecimal(decimal.NewFromInt(50))}
				if err := db.Create(&account).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Create(&order).Error; err != nil {
					t.Fatal(err)
				}
				channel := createUnderpaidChannel(t, db, svc, "Online", constants.PaymentChannelTypeWechat)
				channels, channelErr := svc.GetAllowedChannelsForProducts(nil)
				if channelErr != nil || len(channels) != 1 || channels[0].ID != channel.ID {
					t.Fatalf("online channels must remain available: channels=%v error=%v", channels, channelErr)
				}
				result, err := svc.CreatePayment(CreatePaymentInput{
					OrderID: order.ID, ChannelID: channel.ID, UseBalance: useBalance, Context: context.Background(),
				})
				if useBalance {
					if !errors.Is(err, ErrPaymentInvalid) || result != nil {
						t.Fatalf("balance request must fail: result=%+v error=%v", result, err)
					}
				} else if err != nil || result == nil || result.Payment == nil || result.OrderPaid ||
					!result.OnlinePayAmount.Decimal.Equal(decimal.NewFromInt(50)) || !result.WalletPaidAmount.Decimal.IsZero() {
					t.Fatalf("online payment must remain available: result=%+v error=%v", result, err)
				}
				if got := walletBalance(t, db, user.ID); got != "100.00" {
					t.Fatalf("wallet changed: %s", got)
				}
				var count int64
				if err := db.Model(&walletdomain.Transaction{}).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("unexpected wallet transactions: count=%d error=%v", count, err)
				}
				if err := db.Model(&paymentdomain.Payment{}).Count(&count).Error; err != nil ||
					(useBalance && count != 0) || (!useBalance && count != 1) {
					t.Fatalf("unexpected payment count: count=%d error=%v", count, err)
				}
				if err := db.First(&order, order.ID).Error; err != nil || order.Status != constants.OrderStatusPendingPayment ||
					!order.WalletPaidAmount.Decimal.IsZero() {
					t.Fatalf("unexpected order mutation: order=%+v error=%v", order, err)
				}
			})
		}
	}
}
