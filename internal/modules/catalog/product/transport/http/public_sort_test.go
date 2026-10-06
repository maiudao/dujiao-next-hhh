package producthttp

import (
	"encoding/json"
	"strconv"
	"testing"

	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
)

func TestPublicProductResponseIncludesNumericSortWeight(t *testing.T) {
	for _, weight := range []int{-2, 0, 5, 10} {
		t.Run(strconv.Itoa(weight), func(t *testing.T) {
			h := &PublicHandler{}
			product := &productdomain.Product{ID: 1, SortOrder: weight}
			response, err := h.decoratePublicProduct(product, nil)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(payload, &fields); err != nil {
				t.Fatal(err)
			}
			if got := string(fields["sort_order"]); got != strconv.Itoa(weight) {
				t.Fatalf("sort_order must include the configured numeric weight, got %q, want %d", got, weight)
			}
			for _, privateField := range []string{"cost_price_amount", "instructions"} {
				if _, present := fields[privateField]; present {
					t.Fatalf("private field %q must not enter the public response", privateField)
				}
			}
		})
	}
}
