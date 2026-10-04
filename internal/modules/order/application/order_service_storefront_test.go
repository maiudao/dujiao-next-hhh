package application

import (
	"errors"
	"testing"

	"github.com/dujiao-next/internal/constants"
	resellercontract "github.com/dujiao-next/internal/modules/reseller/contract"
	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	settingscontract "github.com/dujiao-next/internal/modules/settings/contract"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

type storefrontSettingRepo struct {
	value jsonmap.JSON
	err   error
}

func (repo *storefrontSettingRepo) GetByKey(key string) (jsonmap.JSON, bool, error) {
	if repo.err != nil {
		return nil, false, repo.err
	}
	if key != constants.SettingKeySiteConfig || repo.value == nil {
		return nil, false, nil
	}
	return repo.value, true, nil
}

func (repo *storefrontSettingRepo) Upsert(key string, value jsonmap.JSON) (jsonmap.JSON, error) {
	repo.value = value
	return value, nil
}

var _ settingscontract.Store = (*storefrontSettingRepo)(nil)

func TestStorefrontSamplingBlocksMainOrdersButNotResellerOrders(t *testing.T) {
	repo := &storefrontSettingRepo{value: jsonmap.JSON{"storefront_mode": "sampling"}}
	service := &OrderService{settingService: settingsapp.NewService(repo)}

	if _, err := service.CreateOrder(CreateOrderInput{UserID: 1}); !errors.Is(err, ErrStorefrontPaused) {
		t.Fatalf("CreateOrder error = %v, want ErrStorefrontPaused", err)
	}
	if _, err := service.PreviewOrder(CreateOrderInput{UserID: 1}); !errors.Is(err, ErrStorefrontPaused) {
		t.Fatalf("PreviewOrder error = %v, want ErrStorefrontPaused", err)
	}
	if _, err := service.PreviewGuestOrder(CreateGuestOrderInput{}); !errors.Is(err, ErrStorefrontPaused) {
		t.Fatalf("PreviewGuestOrder error = %v, want ErrStorefrontPaused", err)
	}

	resellerTenant := resellercontract.ResellerTenantContext("reseller.example", 2, 3, "shop.example")
	if _, err := service.CreateOrder(CreateOrderInput{UserID: 1, Tenant: resellerTenant}); !errors.Is(err, ErrQueueUnavailable) {
		t.Fatalf("reseller CreateOrder error = %v, want the normal queue check", err)
	}
}

func TestStorefrontModeLookupFailsClosedOnSettingsError(t *testing.T) {
	repo := &storefrontSettingRepo{err: errors.New("settings store unavailable")}
	service := &OrderService{settingService: settingsapp.NewService(repo)}

	if err := service.ensureStorefrontOpen(resellercontract.MainTenantContext("shop.example")); err == nil {
		t.Fatal("expected settings lookup error")
	}
}
