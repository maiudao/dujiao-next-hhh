package integrationtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	categorydomain "github.com/dujiao-next/internal/modules/catalog/category/domain"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/gin-gonic/gin"
)

func TestAdminProductHTTPRequiresManualLabels(t *testing.T) {
	h, db := setupAdminProductHandlerTest(t)
	category := categorydomain.Category{Slug: "http-labels", NameJSON: jsonmap.JSON{"zh-CN": "labels"}, IsActive: true}
	if err := db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	request := func(method, body, id string, handler gin.HandlerFunc) map[string]interface{} {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(method, "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: id}}
		handler(c)
		var result map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode: %v, body=%s", err, w.Body.String())
		}
		return result
	}
	body := fmt.Sprintf(`{"category_id":%d,"slug":"http-labels","title":{"zh-CN":"ChatGPT Plus"},"price_amount":127,"fulfillment_type":"manual","purchase_type":"member","is_active":true`, category.ID)
	if got := request(http.MethodPost, body+`}`, "", h.CreateProduct); got["status_code"] != float64(400) {
		t.Fatalf("create bypass: %v", got)
	}
	got := request(http.MethodPost, body+`,"tags":[" 菲律宾区官方代充 "]}`, "", h.CreateProduct)
	if got["status_code"] != float64(0) {
		t.Fatalf("create: %v", got)
	}
	id := fmt.Sprintf("%.0f", got["data"].(map[string]interface{})["id"])
	if got := request(http.MethodPut, body+`}`, id, h.UpdateProduct); got["status_code"] != float64(400) {
		t.Fatalf("update bypass: %v", got)
	}
	if err := db.Model(&productdomain.Product{}).Where("id = ?", id).Update("tags", "[]").Error; err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodPatch, `{"is_active":true}`, id, h.QuickUpdateProduct); got["status_code"] != float64(400) {
		t.Fatalf("quick publish bypass: %v", got)
	}
	batch := request(http.MethodPost, `{"ids":[`+id+`],"is_active":true}`, "", h.BatchUpdateProductStatus)
	data := batch["data"].(map[string]interface{})
	if data["success_count"] != float64(0) {
		t.Fatalf("batch publish bypass: %v", batch)
	}
	failures := data["failed_items"].([]interface{})
	if len(failures) != 1 || failures[0].(map[string]interface{})["error_code"] != "product_tags_invalid" {
		t.Fatalf("missing batch label error: %v", batch)
	}
}
