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
	paidMessage := strings.TrimSpace(normalizeSettingTextWithRuneLimit(config["paid_order_message"], 300))
	if paidMessage == "" {
		paidMessage = "订单已付款。可以点击顶部栏的联系方式，联系店主确认处理进度；咨询时请附上订单号。打烊期间回复可能稍晚，请留意开店时间。"
	}
	result["paid_order_message"] = paidMessage
	deliveredMessage := strings.TrimSpace(normalizeSettingTextWithRuneLimit(config["delivered_order_message"], 300))
	if deliveredMessage == "" {
		deliveredMessage = "订单已交付成功，请查看本页“订单交付”里的“交付结果”，那里是店主发送给你的内容。使用方法请查看下方“使用说明”，有疑问可以联系店主。"
	}
	result["delivered_order_message"] = deliveredMessage
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
		contactMessages := []interface{}{}
		items, configured := state["contact_messages"].([]interface{})
		if !configured {
			if mode == "open" {
				contactMessages = append(contactMessages, "想了解商品、购买流程或售后？可以添加店主的联系方式，先聊清楚再下单。")
			} else {
				contactMessages = append(contactMessages, "店主暂时休息啦。可以先添加联系方式留言，咨询商品或订单，营业后会尽快回复。")
			}
		}
		for _, item := range items {
			if _, ok := item.(string); !ok {
				continue
			}
			text := strings.TrimSpace(normalizeSettingTextWithRuneLimit(item, 180))
			if text != "" {
				contactMessages = append(contactMessages, text)
			}
			if len(contactMessages) == 10 {
				break
			}
		}
		result[mode] = map[string]interface{}{"images": images, "messages": messages, "contact_messages": contactMessages}
	}
	return result
}
