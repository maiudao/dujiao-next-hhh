package smtp

import (
	"strings"
	"testing"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/shared/mailbrand"
)

func TestVerificationMailUsesPurposeLocaleAndResolvedSite(t *testing.T) {
	for _, locale := range []string{i18n.LocaleZH, i18n.LocaleTW, i18n.LocaleEN} {
		for _, brand := range []mailbrand.Brand{
			{SiteName: "绘画会", SiteURL: "https://www.huihuahui.xyz"},
			{SiteName: "绘画会商店", SiteURL: "https://shop.huihuahui.xyz"},
		} {
			seen := make(map[string]bool)
			for _, purpose := range []string{constants.VerifyPurposeRegister, constants.VerifyPurposeReset,
				constants.VerifyPurposeTelegramBind, constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew} {
				t.Run(locale+"/"+brand.SiteName+"/"+purpose, func(t *testing.T) {
					subject, body := buildVerifyCodeContent("123456", purpose, locale, brand)
					if !strings.HasPrefix(subject, brand.SiteName+" - ") || !strings.Contains(body, brand.SiteURL) {
						t.Fatal("mail lost the resolved site identity")
					}
					if seen[subject] {
						t.Fatal("different security operations share a subject")
					}
					seen[subject] = true
					if strings.Count(body, "123456") != 1 {
						t.Fatal("verification code missing or duplicated")
					}
					for _, forbidden := range []string{"password=", "access_token", "checkout_token", "卡密：", "Dujiao-Next"} {
						if strings.Contains(subject+body, forbidden) {
							t.Fatalf("unexpected delivery or credential field: %s", forbidden)
						}
					}
					otherURL := "https://www.huihuahui.xyz"
					if brand.SiteURL == otherURL {
						otherURL = "https://shop.huihuahui.xyz"
					}
					if strings.Contains(body, otherURL) {
						t.Fatal("mail points to the other site's operation")
					}
				})
			}
		}
	}
}
