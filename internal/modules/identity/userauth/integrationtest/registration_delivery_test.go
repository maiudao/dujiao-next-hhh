package integrationtest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	emailverificationdomain "github.com/dujiao-next/internal/modules/identity/emailverification/domain"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	"github.com/dujiao-next/internal/shared/mailbrand"
	"gorm.io/gorm"
)

type registrationDelivery struct {
	mu    sync.Mutex
	calls int
	fail  bool
	code  string
	brand mailbrand.Brand
}

type databaseBrandResolver struct{ db *gorm.DB }

func (r databaseBrandResolver) ResolveEmailBrand(ctx context.Context, _ mailbrand.Scope) (mailbrand.Brand, error) {
	var one int
	err := r.db.WithContext(ctx).Raw("SELECT 1").Scan(&one).Error
	return mailbrand.Brand{SiteName: "Storefront QA"}, err
}

func TestNativeVerificationResolvesBrandBeforeSingleConnectionTransaction(t *testing.T) {
	sender := &registrationDelivery{}
	svc, _, db := newRegistrationDomainPolicyAuthServiceWithSender(t, sender)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	svc.SetEmailBrandResolver(databaseBrandResolver{db: db})
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := svc.SendVerifyCode(ctx, "native-brand@example.com", "register", "zh-CN"); err != nil {
		t.Fatal("native verification blocked on its own database transaction", err)
	}
	if sender.calls != 1 || sender.brand.SiteName != "Storefront QA" {
		t.Fatal("verification did not retain the resolved storefront brand")
	}
}

func (s *registrationDelivery) SendVerifyCode(_, code, _, _ string, brand mailbrand.Brand) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.code = code
	s.brand = brand
	if s.fail {
		return errors.New("synthetic SMTP unavailable")
	}
	return nil
}

func TestRegistrationDeliveryConcurrentCooldownFailureAndBrand(t *testing.T) {
	sender := &registrationDelivery{}
	svc, _, db := newRegistrationDomainPolicyAuthServiceWithSender(t, sender)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	brand := mailbrand.Brand{SiteName: "绘画荟", SiteURL: "https://main.example.invalid", FromName: "绘画荟"}
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- svc.SendRegistrationCodeForSite(context.Background(), "delivery@example.com", "zh-CN", brand)
		}()
	}
	wg.Wait()
	close(results)
	success, rejected := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, userauthapp.ErrVerifyCodeTooFrequent):
			rejected++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || rejected != 7 || sender.calls != 1 {
		t.Fatal("concurrent requests sent more than one email")
	}
	if sender.brand != brand || len(sender.code) != 6 {
		t.Fatal("main-site brand or configured OTP length lost")
	}
	var latest emailverificationdomain.Code
	if err := db.Where("email=?", "delivery@example.com").First(&latest).Error; err != nil {
		t.Fatal(err)
	}
	if lifetime := latest.ExpiresAt.Sub(latest.SentAt); lifetime != 10*time.Minute {
		t.Fatal("unexpected code expiry")
	}
	sender.fail = true
	if err := svc.SendRegistrationCodeForSite(t.Context(), "failure@example.com", "zh-CN", brand); err == nil {
		t.Fatal("SMTP failure reported success")
	}
	var count int64
	if err := db.Model(&emailverificationdomain.Code{}).Where("email=?", "failure@example.com").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("failed delivery created a usable code")
	}
	sender.fail = false
	if err := svc.SendRegistrationCodeForSite(t.Context(), "failure@example.com", "zh-CN", brand); err != nil {
		t.Fatal("delivery failure blocked retry", err)
	}
	if _, _, _, err := svc.Register("other@example.com", "secret123", sender.code, true, true); err == nil {
		t.Fatal("code was accepted for a different email")
	}
	if err := db.Model(&userdomain.User{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("wrong-email code created account")
	}
}
