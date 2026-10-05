package publicconfighttp

import "testing"

func TestHiddenContactsNeverReachPublicConfig(t *testing.T) {
	data := map[string]interface{}{
		"storefront_contacts": map[string]interface{}{
			"qq":       map[string]interface{}{"enabled": false, "value": "private-qq"},
			"wechat":   map[string]interface{}{"enabled": true, "value": "public-wechat"},
			"telegram": map[string]interface{}{"enabled": false, "value": "private-telegram"},
		},
		"contact": map[string]interface{}{"telegram": "legacy-telegram", "whatsapp": "unchanged"},
	}
	redactStorefrontContacts(data)
	config := storefrontContactMap(data["storefront_contacts"])
	for _, key := range []string{"qq", "telegram"} {
		entry := storefrontContactMap(config[key])
		if entry["enabled"] != false || entry["value"] != "" {
			t.Fatalf("hidden %s leaked", key)
		}
	}
	if storefrontContactMap(config["wechat"])["value"] != "public-wechat" {
		t.Fatal("public contact lost")
	}
	legacy := storefrontContactMap(data["contact"])
	if legacy["telegram"] != "" || legacy["whatsapp"] != "unchanged" {
		t.Fatal("legacy fallback leaked or unrelated contact changed")
	}
	redactStorefrontContacts(data) // Cached responses are redacted again safely.
	if storefrontContactMap(data["contact"])["telegram"] != "" {
		t.Fatal("cached hidden contact leaked")
	}
}
