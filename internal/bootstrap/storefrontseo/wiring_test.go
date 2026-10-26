package storefrontseo

import (
	"context"
	"errors"
	categorydomain "github.com/dujiao-next/internal/modules/catalog/category/domain"
	productcontract "github.com/dujiao-next/internal/modules/catalog/product/contract"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	contentapp "github.com/dujiao-next/internal/modules/content/application"
	contentcontract "github.com/dujiao-next/internal/modules/content/contract"
	contentdomain "github.com/dujiao-next/internal/modules/content/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"net/http/httptest"
	"strings"
	"testing"
)

type source struct{ calls int }

func (*source) GetByKey(string) (jsonmap.JSON, error) {
	return jsonmap.JSON{"brand": map[string]interface{}{"site_url": "shop.example", "site_name": "Shop"}}, nil
}
func (s *source) GetPublicBySlug(slug string) (*productdomain.Product, error) {
	s.calls++
	if slug == "missing" {
		return nil, productcontract.ErrNotFound
	}
	if slug == "error" {
		return nil, errors.New("temporary")
	}
	return &productdomain.Product{Slug: slug, TitleJSON: jsonmap.JSON{"zh-CN": "Plus"}, Tags: []string{"A"}, DescriptionJSON: jsonmap.JSON{"zh-CN": "公开内容"}, InstructionsJSON: jsonmap.JSON{"zh-CN": "SECRET_INSTRUCTIONS"}}, nil
}
func (*source) ListPublic(string, string, int, int) ([]productdomain.Product, int64, error) {
	return nil, 0, nil
}
func (*source) ListActive() ([]categorydomain.Category, error) { return nil, nil }

type postSource struct{}

func (postSource) ListPublic(context.Context, contentapp.PublicPostQuery) ([]contentdomain.Post, int64, error) {
	return nil, 0, nil
}
func (postSource) GetPublicBySlug(context.Context, string) (*contentdomain.Post, error) {
	return nil, contentcontract.ErrNotFound
}

func TestResolverUsesOnlyPublicCatalogOnConfiguredHost(t *testing.T) {
	s := &source{}
	resolve := New(s, s, s, postSource{})
	for _, tt := range []struct {
		url           string
		status        int
		fail, noindex bool
	}{
		{"https://shop.example/products/real", 0, false, false},
		{"https://shop.example/products/missing", 404, false, true},
		{"https://shop.example/products/error", 0, true, false},
		{"https://shop.example/random", 404, false, true},
		{"https://shop.example/categories/missing", 404, false, true},
		{"https://shop.example/blog/missing", 404, false, true},
		{"https://shop.example/privacy", 0, false, true},
		{"https://shop.example/blog", 0, false, true},
		{"https://reseller.example/products/real", 0, false, false},
	} {
		before := s.calls
		page, err := resolve(httptest.NewRequest("GET", tt.url, nil))
		if (err != nil) != tt.fail || page.Status != tt.status || page.Noindex != tt.noindex {
			t.Fatalf("%s: page=%+v err=%v", tt.url, page, err)
		}
		if strings.Contains(tt.url, "reseller.example") && s.calls != before {
			t.Fatal("reseller host accessed main catalog")
		}
		if strings.Contains(strings.Join(page.Paragraphs, " "), "SECRET_INSTRUCTIONS") {
			t.Fatal("order-only content leaked")
		}
		if tt.url == "https://shop.example/products/real" && (page.Title != "Plus · A" || page.Description != "Plus · A：公开内容" || page.Canonical != tt.url) {
			t.Fatalf("metadata mismatch %+v", page)
		}
	}
}
