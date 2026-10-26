package settingsapp

import (
	"github.com/dujiao-next/internal/constants"
	"testing"
)

func TestSiteURLInvalidOriginIsRejectedBeforeSave(t *testing.T) {
	for _, raw := range []string{"https://example.com/path", "https://user:pass@example.com", "https://example.com?redirect=1"} {
		svc := NewService(newMockSettingRepo())
		if _, err := svc.Update(constants.SettingKeySiteConfig, map[string]interface{}{"brand": map[string]interface{}{"site_url": raw}}); err == nil {
			t.Fatalf("invalid origin accepted: %s", raw)
		}
		if saved, _ := svc.GetByKey(constants.SettingKeySiteConfig); saved != nil {
			t.Fatal("invalid origin persisted")
		}
	}
}
