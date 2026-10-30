package categoryintegrationtest

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"

	categoryapp "github.com/dujiao-next/internal/modules/catalog/category/application"
	categorydomain "github.com/dujiao-next/internal/modules/catalog/category/domain"
	categorygormstore "github.com/dujiao-next/internal/modules/catalog/category/infrastructure/gormstore"
	categoryhttp "github.com/dujiao-next/internal/modules/catalog/category/transport/http"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func newCategoryServiceForTest(t *testing.T) (*categoryapp.Service, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf("file:category_service_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&categorydomain.Category{}, &productdomain.Product{}); err != nil {
		t.Fatalf("auto migrate category/product failed: %v", err)
	}

	return categoryapp.NewService(categorygormstore.NewCategoryStore(db)), db
}

func createCategoryFixture(t *testing.T, db *gorm.DB, slug string, parentID uint) categorydomain.Category {
	t.Helper()

	category := categorydomain.Category{
		ParentID: parentID,
		Slug:     slug,
		NameJSON: jsonmap.JSON{
			"zh-CN": slug,
		},
	}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("create category fixture failed: %v", err)
	}
	return category
}

func createProductFixture(t *testing.T, db *gorm.DB, categoryID uint, slug string) {
	t.Helper()

	product := productdomain.Product{
		CategoryID:  categoryID,
		Slug:        slug,
		TitleJSON:   jsonmap.JSON{"zh-CN": slug},
		PriceAmount: money.FromDecimal(decimal.NewFromInt(10)),
		IsActive:    true,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product fixture failed: %v", err)
	}
}

func TestCategoryServiceCreateSupportsSecondLevelCategory(t *testing.T) {
	svc, db := newCategoryServiceForTest(t)
	parent := createCategoryFixture(t, db, "games", 0)

	category, err := svc.Create(categoryapp.UpsertInput{
		ParentID: parent.ID,
		Slug:     "steam",
		NameJSON: map[string]interface{}{
			"zh-CN": "Steam",
		},
	})
	if err != nil {
		t.Fatalf("create second-level category failed: %v", err)
	}
	if category.ParentID != parent.ID {
		t.Fatalf("expected parent_id=%d, got %d", parent.ID, category.ParentID)
	}
}

func TestCategoryServiceCreateRejectsMissingOrSecondLevelParent(t *testing.T) {
	svc, db := newCategoryServiceForTest(t)
	parent := createCategoryFixture(t, db, "games", 0)
	child := createCategoryFixture(t, db, "steam", parent.ID)

	_, err := svc.Create(categoryapp.UpsertInput{
		ParentID: 9999,
		Slug:     "missing-parent",
		NameJSON: map[string]interface{}{"zh-CN": "missing-parent"},
	})
	if err != categoryapp.ErrParentInvalid {
		t.Fatalf("expected ErrCategoryParentInvalid for missing parent, got %v", err)
	}

	_, err = svc.Create(categoryapp.UpsertInput{
		ParentID: child.ID,
		Slug:     "steam-gift-card",
		NameJSON: map[string]interface{}{"zh-CN": "steam-gift-card"},
	})
	if err != categoryapp.ErrParentInvalid {
		t.Fatalf("expected ErrCategoryParentInvalid for second-level parent, got %v", err)
	}
}

func TestCategoryServiceUpdateRejectsInvalidParentAssignment(t *testing.T) {
	svc, db := newCategoryServiceForTest(t)
	rootA := createCategoryFixture(t, db, "games", 0)
	rootB := createCategoryFixture(t, db, "cards", 0)
	_ = createCategoryFixture(t, db, "steam", rootA.ID)

	_, err := svc.Update(fmt.Sprintf("%d", rootA.ID), categoryapp.UpsertInput{
		ParentID: rootA.ID,
		Slug:     rootA.Slug,
		NameJSON: map[string]interface{}{"zh-CN": rootA.Slug},
	})
	if err != categoryapp.ErrParentInvalid {
		t.Fatalf("expected ErrCategoryParentInvalid for self parent, got %v", err)
	}

	_, err = svc.Update(fmt.Sprintf("%d", rootA.ID), categoryapp.UpsertInput{
		ParentID: rootB.ID,
		Slug:     rootA.Slug,
		NameJSON: map[string]interface{}{"zh-CN": rootA.Slug},
	})
	if err != categoryapp.ErrParentInvalid {
		t.Fatalf("expected ErrCategoryParentInvalid when moving parent with children, got %v", err)
	}
}

func TestCategoryServiceDeleteRejectsCategoriesWithChildrenOrProducts(t *testing.T) {
	svc, db := newCategoryServiceForTest(t)
	parent := createCategoryFixture(t, db, "games", 0)
	child := createCategoryFixture(t, db, "steam", parent.ID)

	if err := svc.Delete(fmt.Sprintf("%d", parent.ID)); err != categoryapp.ErrInUse {
		t.Fatalf("expected ErrCategoryInUse for category with children, got %v", err)
	}

	createProductFixture(t, db, child.ID, "steam-product")
	if err := svc.Delete(fmt.Sprintf("%d", child.ID)); err != categoryapp.ErrInUse {
		t.Fatalf("expected ErrCategoryInUse for category with products, got %v", err)
	}
}

func TestCategoryServiceListSortOrderDescending(t *testing.T) {
	svc, db := newCategoryServiceForTest(t)

	high := categorydomain.Category{
		Slug:      "high",
		NameJSON:  jsonmap.JSON{"zh-CN": "high"},
		SortOrder: 100,
	}
	low := categorydomain.Category{
		Slug:      "low",
		NameJSON:  jsonmap.JSON{"zh-CN": "low"},
		SortOrder: 1,
	}
	if err := db.Create(&high).Error; err != nil {
		t.Fatalf("create high sort category failed: %v", err)
	}
	if err := db.Create(&low).Error; err != nil {
		t.Fatalf("create low sort category failed: %v", err)
	}

	rows, err := svc.List()
	if err != nil {
		t.Fatalf("list categories failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(rows))
	}
	if rows[0].Slug != "high" || rows[1].Slug != "low" {
		t.Fatalf("expected high sort_order first, got %s then %s", rows[0].Slug, rows[1].Slug)
	}
}

type iconCategoryQueries struct{}

func (iconCategoryQueries) ListActive() ([]categorydomain.Category, error) {
	return []categorydomain.Category{
		{ID: 1, Slug: "gpt", Icon: "/uploads/gpt.png"},
		{ID: 2, Slug: "grok", Icon: "/uploads/missing.png"},
	}, nil
}

func TestPublicCategoriesIncludeSmallIconWithoutChangingOriginalURL(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir("uploads", 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(root, "uploads", "gpt.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewNRGBA(image.Rect(0, 0, 512, 512))); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	handler := categoryhttp.NewPublicHandler(iconCategoryQueries{})
	router := gin.New()
	router.GET("/categories", handler.List)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/categories", nil))
	var response struct {
		Data []struct {
			ID        uint   `json:"id"`
			Icon      string `json:"icon"`
			Thumbnail string `json:"icon_thumbnail"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || len(response.Data) != 2 {
		t.Fatalf("response: %s", recorder.Body.String())
	}
	if response.Data[0].Icon != "/uploads/gpt.png" || !strings.HasPrefix(response.Data[0].Thumbnail, "data:image/png;base64,") {
		t.Fatal("category image contract changed")
	}
	if response.Data[1].Thumbnail != "" || response.Data[1].Icon != "/uploads/missing.png" {
		t.Fatal("missing icon broke category response")
	}
}
