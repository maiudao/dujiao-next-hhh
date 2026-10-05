package settingsapp

import (
	"regexp"
	"strings"
)

var supportImagePath = regexp.MustCompile(`(?i)^/(uploads|storefront)/[a-z0-9_./\-]+\.(png|jpe?g|webp|gif)$`)

// Contact values stay private until the merchant explicitly enables their row.
func normalizeStorefrontContacts(raw, legacy interface{}) map[string]interface{} {
	config, configured := raw.(map[string]interface{})
	contact, _ := legacy.(map[string]interface{})
	result := make(map[string]interface{}, 3)
	for _, key := range []string{"qq", "wechat", "telegram"} {
		entry, _ := config[key].(map[string]interface{})
		value := strings.TrimSpace(normalizeSettingTextWithRuneLimit(entry["value"], 180))
		enabled, _ := entry["enabled"].(bool)
		if !configured && key == "telegram" {
			value = strings.TrimSpace(normalizeSettingTextWithRuneLimit(contact[key], 180))
			enabled = value != ""
		}
		result[key] = map[string]interface{}{"value": value, "enabled": enabled}
	}
	return result
}

var defaultSupportImages = map[string][]interface{}{
	"open":     {"/storefront/characters/gpt-normal.webp", "/storefront/characters/gpt-horizontal.webp", "/storefront/characters/gpt-link.webp", "/storefront/characters/gpt-move.webp", "/storefront/characters/gpt-vertical.webp", "/storefront/characters/gpt-working.webp", "/storefront/characters/gpt-handwriting.webp", "/storefront/characters/gpt-precision.webp", "/storefront/characters/gpt-diagonal1.webp", "/storefront/characters/gpt-pin.webp", "/storefront/characters/gpt-help.webp", "/storefront/characters/gpt-person.webp"},
	"sampling": {"/storefront/characters/gpt-busy.webp", "/storefront/characters/gpt-text.webp", "/storefront/characters/gpt-unavailable.webp", "/storefront/characters/gpt-alternate.webp", "/storefront/characters/gpt-diagonal2.webp"},
}

// Keep character assets on this site's existing static/upload paths.
func normalizeStorefrontSupport(raw interface{}) map[string]interface{} {
	config, _ := raw.(map[string]interface{})
	result := make(map[string]interface{}, 2)
	for _, mode := range []string{"open", "sampling"} {
		state, _ := config[mode].(map[string]interface{})
		images := []interface{}{}
		items, _ := state["images"].([]interface{})
		for _, item := range items {
			path, ok := item.(string)
			path = strings.TrimSpace(path)
			if ok && len(path) <= 1024 && !strings.Contains(path, "..") && supportImagePath.MatchString(path) {
				images = append(images, path)
			}
			if len(images) == 20 {
				break
			}
		}
		messages := []interface{}{}
		items, _ = state["messages"].([]interface{})
		for _, item := range items {
			if _, ok := item.(string); !ok {
				continue
			}
			text := strings.TrimSpace(normalizeSettingTextWithRuneLimit(item, 180))
			if text != "" {
				messages = append(messages, text)
			}
			if len(messages) == 30 {
				break
			}
		}
		if len(images) == 0 {
			images = append(images, defaultSupportImages[mode]...)
		}
		if len(messages) == 0 {
			if mode == "open" {
				messages = []interface{}{"购买前后有什么不清楚的，都可以和客服保持联系哦 (^_^)", "想了解商品或库存？下单前可以先和客服聊聊。", "付款后记得查看订单进度，有售后问题也可以联系店主。"}
			} else {
				messages = []interface{}{"明天再来吧，营业时间是每天9：00到12：00。", "店主正在休息，营业后就能继续购买啦。", "先收藏小店吧，明天再来挑选你喜欢的商品。"}
			}
		}
		result[mode] = map[string]interface{}{"images": images, "messages": messages}
	}
	return result
}
