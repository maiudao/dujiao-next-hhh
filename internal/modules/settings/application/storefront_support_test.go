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
