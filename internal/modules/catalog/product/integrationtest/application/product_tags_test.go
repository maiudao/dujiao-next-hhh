package integrationtest

import (
	"errors"
	"strconv"
	"testing"

	categorydomain "github.com/dujiao-next/internal/modules/catalog/category/domain"
	productwrite "github.com/dujiao-next/internal/modules/catalog/product/application/write"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/shopspring/decimal"
)

func TestProductLabelsPublishAndPersistence(t *testing.T) {
	svc, db := newProductServiceForTest(t)
	category := categorydomain.Category{Slug: "labels", NameJSON: jsonmap.JSON{"zh-CN": "labels"}, IsActive: true}
	if err := db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	input := productwrite.CreateProductInput{
		CategoryID: category.ID, Slug: "manual-labels", TitleJSON: map[string]interface{}{"zh-CN": "完整商品标题"},
		PriceAmount: decimal.NewFromInt(127), FulfillmentType: "manual", PurchaseType: "member",
	}
	if _, err := svc.Write.Create(input); !errors.Is(err, productdomain.ErrProductTagsInvalid) {
		t.Fatalf("published product without labels: %v", err)
	}
	input.Tags = []string{" 菲律宾区官方代充 ", "菲律宾区官方代充"}
	product, err := svc.Write.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	id := strconv.Itoa(int(product.ID))
	var persisted productdomain.Product
	if err := db.First(&persisted, product.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(persisted.Tags) != 1 || persisted.Tags[0] != "菲律宾区官方代充" {
		t.Fatalf("labels not persisted: %v", persisted.Tags)
	}
	input.Tags = nil
	if _, err := svc.Write.Update(id, input); !errors.Is(err, productdomain.ErrProductTagsInvalid) {
		t.Fatalf("active update bypass: %v", err)
	}
	if err := db.First(&persisted, product.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(persisted.Tags) != 1 {
		t.Fatal("rejected update changed labels")
	}
	inactive := false
	input.IsActive = &inactive
	if _, err := svc.Write.Update(id, input); err != nil {
		t.Fatalf("unlisted draft: %v", err)
	}
	if _, err := svc.Admin.QuickUpdate(id, map[string]interface{}{"is_active": true}); !errors.Is(err, productdomain.ErrProductTagsInvalid) {
		t.Fatalf("quick publish bypass: %v", err)
	}
	if _, err := svc.Admin.QuickUpdate(id, map[string]interface{}{"is_active": false}); err != nil {
		t.Fatalf("legacy unpublish blocked: %v", err)
	}
	input.Tags = []string{"智利区官方代充"}
	active := true
	input.IsActive = &active
	if _, err := svc.Write.Update(id, input); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Admin.QuickUpdate(id, map[string]interface{}{"is_active": true}); err != nil {
		t.Fatal(err)
	}
}
