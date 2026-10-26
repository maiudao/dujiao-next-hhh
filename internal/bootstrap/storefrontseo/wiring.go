// Package storefrontseo adapts current public read services to initial HTML.
package storefrontseo

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/htmltext"
	categorydomain "github.com/dujiao-next/internal/modules/catalog/category/domain"
	productcontract "github.com/dujiao-next/internal/modules/catalog/product/contract"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	contentapp "github.com/dujiao-next/internal/modules/content/application"
	contentcontract "github.com/dujiao-next/internal/modules/content/contract"
	contentdomain "github.com/dujiao-next/internal/modules/content/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/siteurl"
	"github.com/dujiao-next/internal/web"
)

type Settings interface {
	GetByKey(string) (jsonmap.JSON, error)
}
type Products interface {
	GetPublicBySlug(string) (*productdomain.Product, error)
	ListPublic(string, string, int, int) ([]productdomain.Product, int64, error)
}
type Categories interface {
	ListActive() ([]categorydomain.Category, error)
}
type Posts interface {
	ListPublic(context.Context, contentapp.PublicPostQuery) ([]contentdomain.Post, int64, error)
	GetPublicBySlug(context.Context, string) (*contentdomain.Post, error)
}

func record(raw any) map[string]interface{} {
	switch value := raw.(type) {
	case map[string]interface{}:
		return value
	case jsonmap.JSON:
		return map[string]interface{}(value)
	}
	return nil
}
func localized(raw any) string {
	if value, ok := raw.(string); ok {
		return strings.TrimSpace(value)
	}
	m := record(raw)
	for _, locale := range []string{"zh-CN", "en-US", "zh-TW"} {
		if value, ok := m[locale].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
func plain(raw string) string {
	return strings.Join(strings.Fields(htmltext.StripToPlainText(raw)), " ")
}
func excerpt(raw string) string {
	r := []rune(plain(raw))
	if len(r) > 160 {
		r = r[:160]
	}
	return string(r)
}

// ProductMetadata mirrors frontend/user/src/utils/productSeo.ts using only public fields.
func ProductMetadata(p *productdomain.Product) (string, string) {
	name := plain(localized(p.TitleJSON))
	title := plain(localized(p.SeoMetaJSON["title"]))
	if title == "" {
		title = name
		count := 0
		for _, raw := range p.Tags {
			tag := plain(raw)
			if tag != "" && !strings.Contains(name, tag) {
				title += " · " + tag
				count++
				if count == 2 {
					break
				}
			}
		}
	}
	summary := plain(localized(p.SeoMetaJSON["description"]))
	if summary == "" {
		summary = plain(localized(p.DescriptionJSON))
	}
	if summary == "" {
		summary = plain(localized(p.ContentJSON))
	}
	if summary == "" {
		summary = title
	} else if !strings.HasPrefix(summary, title) {
		summary = title + "：" + summary
	}
	return title, excerpt(summary)
}

func notFound() web.StorefrontPage {
	return web.StorefrontPage{Status: 404, Title: "页面不存在", Noindex: true, Paragraphs: []string{"这份公开内容不存在或已下架。"}, Links: []web.StorefrontLink{{Title: "返回商品列表", Href: "/products"}}}
}

func New(settings Settings, products Products, categories Categories, posts Posts) web.StorefrontResolver {
	return func(r *http.Request) (web.StorefrontPage, error) {
		config, err := settings.GetByKey(constants.SettingKeySiteConfig)
		if err != nil {
			return web.StorefrontPage{}, err
		}
		brand := record(config["brand"])
		origin, err := siteurl.Normalize(localized(brand["site_url"]))
		if err != nil {
			return web.StorefrontPage{Noindex: true}, nil
		}
		u, _ := url.Parse(origin)
		// Reseller hosts keep their own tenant-aware SPA. Never expose the main catalog there.
		if !strings.EqualFold(r.Host, u.Host) {
			return web.StorefrontPage{}, nil
		}
		path := strings.TrimSuffix(r.URL.Path, "/")
		if path == "" || path == "/index.html" {
			path = "/"
		}
		seo := record(config["seo"])
		page := web.StorefrontPage{Title: localized(seo["title"]), Description: excerpt(localized(seo["description"])), Canonical: origin + path}
		if page.Title == "" {
			page.Title = localized(brand["site_name"])
		}
		if page.Title == "" {
			page.Title = "商品"
		}
		if path == "/" || path == "/products" || strings.HasPrefix(path, "/categories/") {
			categoryID := ""
			categoryName := ""
			if strings.HasPrefix(path, "/categories/") {
				rows, err := categories.ListActive()
				if err != nil {
					return page, err
				}
				for _, category := range rows {
					if path == "/categories/"+category.Slug {
						categoryID = strconv.FormatUint(uint64(category.ID), 10)
						categoryName = localized(category.NameJSON)
						break
					}
				}
				if categoryID == "" {
					return notFound(), nil
				}
			}
			if categoryName != "" {
				page.Title = categoryName + " · " + page.Title
			}
			rows, _, err := products.ListPublic(categoryID, "", 1, 100)
			if err != nil {
				return page, err
			}
			if len(rows) == 0 {
				page.Noindex = true
			}
			page.Paragraphs = []string{page.Description}
			for _, p := range rows {
				title, _ := ProductMetadata(&p)
				page.Links = append(page.Links, web.StorefrontLink{Title: title, Href: "/products/" + url.PathEscape(p.Slug)})
			}
			if path == "/" {
				page.Links = append(page.Links, web.StorefrontLink{Title: "浏览全部商品", Href: "/products"})
			}
			if r.URL.RawQuery != "" {
				page.Noindex = true
			}
			return page, nil
		}
		if strings.HasPrefix(path, "/products/") && strings.Count(path, "/") == 2 {
			p, err := products.GetPublicBySlug(strings.TrimPrefix(path, "/products/"))
			if errors.Is(err, productcontract.ErrNotFound) {
				return notFound(), nil
			}
			if err != nil {
				return page, err
			}
			if p == nil {
				return notFound(), nil
			}
			page.Title, page.Description = ProductMetadata(p)
			page.Canonical = origin + "/products/" + url.PathEscape(p.Slug)
			page.Paragraphs = []string{plain(localized(p.DescriptionJSON)), plain(localized(p.ContentJSON))}
			page.Links = []web.StorefrontLink{{Title: "浏览全部商品", Href: "/products"}}
			if len(p.Images) > 0 {
				image, err := url.Parse(p.Images[0])
				if err == nil {
					image = u.ResolveReference(image)
					if (image.Scheme == "https" || image.Scheme == "http") && image.User == nil {
						page.Image = image.String()
					}
				}
			}
			page.StructuredData = map[string]any{"@context": "https://schema.org", "@type": "Product", "name": page.Title, "description": page.Description, "url": page.Canonical}
			if page.Image != "" {
				page.StructuredData["image"] = []string{page.Image}
			}
			return page, nil
		}
		if path == "/blog" || path == "/notice" {
			kind := strings.TrimPrefix(path, "/")
			rows, _, err := posts.ListPublic(r.Context(), contentapp.PublicPostQuery{Type: kind, Page: 1, PageSize: 100})
			if err != nil {
				return page, err
			}
			page.Title = map[string]string{"blog": "博客", "notice": "公告"}[kind]
			page.Noindex = len(rows) == 0 || r.URL.RawQuery != ""
			for _, post := range rows {
				page.Links = append(page.Links, web.StorefrontLink{Title: localized(post.TitleJSON), Href: "/blog/" + url.PathEscape(post.Slug)})
			}
			return page, nil
		}
		if strings.HasPrefix(path, "/blog/") && strings.Count(path, "/") == 2 {
			post, err := posts.GetPublicBySlug(r.Context(), strings.TrimPrefix(path, "/blog/"))
			if errors.Is(err, contentcontract.ErrNotFound) {
				return notFound(), nil
			}
			if err != nil {
				return page, err
			}
			if post == nil {
				return notFound(), nil
			}
			page.Title = localized(post.TitleJSON)
			page.Description = excerpt(localized(post.SummaryJSON))
			page.Paragraphs = []string{plain(localized(post.ContentJSON))}
			page.Links = []web.StorefrontLink{{Title: "博客", Href: "/blog"}, {Title: "公告", Href: "/notice"}}
			return page, nil
		}
		if path == "/terms" || path == "/privacy" {
			page.Title = map[string]string{"/terms": "服务条款", "/privacy": "隐私政策"}[path]
			content := plain(localized(record(config["legal"])[strings.TrimPrefix(path, "/")]))
			page.Noindex = content == ""
			page.Paragraphs = []string{content}
			return page, nil
		}
		if path == "/about" {
			about := record(config["about"])
			hero := record(about["hero"])
			page.Title = localized(hero["title"])
			if page.Title == "" {
				page.Title = "关于"
			}
			text := plain(localized(about["introduction"]))
			page.Noindex = text == "" && localized(hero["title"]) == ""
			page.Paragraphs = []string{text}
			return page, nil
		}
		return notFound(), nil
	}
}
