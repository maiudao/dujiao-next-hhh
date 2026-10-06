package settingsapp

import (
	"reflect"
	"strings"
	"testing"
)

func TestStorefrontSupportDefaultsAndIsolation(t *testing.T) {
	for _, raw := range []interface{}{nil, "bad", map[string]interface{}{"open": nil}} {
		got := normalizeStorefrontSupport(raw)
		for _, mode := range []string{"open", "sampling"} {
			state := got[mode].(map[string]interface{})
			wantImages := 12
			if mode == "sampling" {
				wantImages = 5
			}
			if len(state["images"].([]interface{})) != wantImages || len(state["messages"].([]interface{})) != 3 {
				t.Fatalf("missing defaults for %s", mode)
			}
		}
	}
	got := normalizeSiteSetting(map[string]interface{}{"storefront_support": map[string]interface{}{
		"open": map[string]interface{}{
			"images":   []interface{}{"https://other.test/a.png", "/uploads/../db/a.png", "/uploads/%2e%2e/a.png", "/uploads/a.svg", 123, " /uploads/characters/a.webp "},
			"messages": []interface{}{"  hello  ", " ", nil, strings.Repeat("字", 190)},
		},
		"sampling": map[string]interface{}{"messages": []interface{}{"休息中"}},
	}})["storefront_support"].(map[string]interface{})
	open := got["open"].(map[string]interface{})
	if !reflect.DeepEqual(open["images"], []interface{}{"/uploads/characters/a.webp"}) {
		t.Fatalf("unsafe asset accepted: %v", open["images"])
	}
	messages := open["messages"].([]interface{})
	if len(messages) != 2 || messages[0] != "hello" || len([]rune(messages[1].(string))) != 180 {
		t.Fatalf("unexpected messages: %v", messages)
	}
	if got["sampling"].(map[string]interface{})["messages"].([]interface{})[0] != "休息中" {
		t.Fatal("states must stay independent")
	}
}

func TestStorefrontSupportLimits(t *testing.T) {
	images, messages := []interface{}{}, []interface{}{}
	for i := 0; i < 40; i++ {
		images = append(images, "/storefront/a.png")
		messages = append(messages, "hello")
	}
	state := normalizeStorefrontSupport(map[string]interface{}{"open": map[string]interface{}{"images": images, "messages": messages}})["open"].(map[string]interface{})
	if len(state["images"].([]interface{})) != 20 || len(state["messages"].([]interface{})) != 30 {
		t.Fatal("configuration limits must apply")
	}
}

func TestStorefrontOrderSupportMessages(t *testing.T) {
	for _, key := range []string{"paid_order_message", "delivered_order_message"} {
		if normalizeStorefrontSupport(nil)[key] == "" {
			t.Fatalf("missing default %s", key)
		}
		got := normalizeStorefrontSupport(map[string]interface{}{key: "  自定义订单提示  "})
		if got[key] != "自定义订单提示" {
			t.Fatalf("%s = %v", key, got[key])
		}
		got = normalizeStorefrontSupport(map[string]interface{}{key: strings.Repeat("字", 350)})
		if len([]rune(got[key].(string))) != 300 {
			t.Fatalf("limit missing for %s", key)
		}
	}
}

func TestStorefrontContactMessages(t *testing.T) {
	defaults := normalizeStorefrontSupport(nil)
	openDefault := defaults["open"].(map[string]interface{})["contact_messages"].([]interface{})
	closedDefault := defaults["sampling"].(map[string]interface{})["contact_messages"].([]interface{})
	if len(openDefault) != 1 || len(closedDefault) != 1 || openDefault[0] == closedDefault[0] {
		t.Fatal("legacy settings need separate contact defaults for each mode")
	}
	items := []interface{}{nil, 123, " ", "  营业联系提示  ", strings.Repeat("字", 200)}
	for i := 0; i < 15; i++ {
		items = append(items, "更多提示")
	}
	got := normalizeSiteSetting(map[string]interface{}{"storefront_support": map[string]interface{}{
		"open":     map[string]interface{}{"messages": []interface{}{"普通提示"}, "contact_messages": items},
		"sampling": map[string]interface{}{"contact_messages": []interface{}{}},
	}})["storefront_support"].(map[string]interface{})
	open := got["open"].(map[string]interface{})
	messages := open["contact_messages"].([]interface{})
	if len(messages) != 10 || messages[0] != "营业联系提示" || len([]rune(messages[1].(string))) != 180 {
		t.Fatalf("contact limits missing: %v", messages)
	}
	if !reflect.DeepEqual(open["messages"], []interface{}{"普通提示"}) {
		t.Fatal("contact messages must not replace ordinary messages")
	}
	closed := got["sampling"].(map[string]interface{})["contact_messages"].([]interface{})
	if len(closed) != 0 {
		t.Fatal("explicit empty contact list must remain disabled")
	}
	roundTrip := normalizeStorefrontSupport(got)
	if !reflect.DeepEqual(roundTrip["sampling"].(map[string]interface{})["contact_messages"], closed) {
		t.Fatal("disabled contact list must survive normalization")
	}
}
