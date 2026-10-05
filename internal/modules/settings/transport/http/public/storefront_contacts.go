package publicconfighttp

import (
	"github.com/dujiao-next/internal/shared/jsonmap"
	"strings"
)

func storefrontContactMap(raw interface{}) map[string]interface{} {
	switch value := raw.(type) {
	case map[string]interface{}:
		return value
	case jsonmap.JSON:
		return map[string]interface{}(value)
	}
	return nil
}

func redactStorefrontContacts(data map[string]interface{}) {
	raw, configured := data["storefront_contacts"]
	if !configured {
		return
	} // Existing public Telegram is the legacy fallback.
	config := storefrontContactMap(raw)
	public := make(map[string]interface{}, 3)
	for _, key := range []string{"qq", "wechat", "telegram"} {
		entry := storefrontContactMap(config[key])
		enabled, _ := entry["enabled"].(bool)
		value, _ := entry["value"].(string)
		value = strings.TrimSpace(value)
		if !enabled {
			value = ""
		}
		public[key] = map[string]interface{}{"enabled": enabled && value != "", "value": value}
	}
	data["storefront_contacts"] = public
	// Do not leak a hidden Telegram through the old footer configuration.
	legacy := make(map[string]interface{})
	for key, value := range storefrontContactMap(data["contact"]) {
		legacy[key] = value
	}
	legacy["telegram"] = storefrontContactMap(public["telegram"])["value"]
	data["contact"] = legacy
}
