package web

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"html/template"
	"net/http"
	"regexp"
	"strings"
)

// StorefrontPage contains only explicitly public fields, never a product domain object.
type StorefrontPage struct {
	Status                               int
	Title, Description, Canonical, Image string
	Noindex                              bool
	Paragraphs                           []string
	Links                                []StorefrontLink
	StructuredData                       map[string]any
}
type StorefrontLink struct{ Title, Href string }
type StorefrontResolver func(*http.Request) (StorefrontPage, error)

var storefrontHead = template.Must(template.New("seo").Parse(`<title data-storefront-seo>{{.Title}}</title>
<meta data-storefront-seo name="description" content="{{.Description}}">
{{if .Canonical}}<link data-storefront-seo rel="canonical" href="{{.Canonical}}">{{end}}
<meta data-storefront-seo property="og:type" content="website">
<meta data-storefront-seo property="og:title" content="{{.Title}}">
<meta data-storefront-seo property="og:description" content="{{.Description}}">
{{if .Canonical}}<meta data-storefront-seo property="og:url" content="{{.Canonical}}">{{end}}
{{if .Image}}<meta data-storefront-seo property="og:image" content="{{.Image}}">{{end}}
<meta data-storefront-seo name="twitter:card" content="summary_large_image">
<meta data-storefront-seo name="twitter:title" content="{{.Title}}">
<meta data-storefront-seo name="twitter:description" content="{{.Description}}">
{{if .Noindex}}<meta data-storefront-seo name="robots" content="noindex, follow">{{end}}`))
var storefrontBody = template.Must(template.New("body").Parse(`<main data-storefront-initial><h1>{{.Title}}</h1>{{range .Paragraphs}}<p>{{.}}</p>{{end}}{{if .Links}}<nav aria-label="公开页面">{{range .Links}}<p><a href="{{.Href}}">{{.Title}}</a></p>{{end}}</nav>{{end}}</main>`))
var initialTitle = regexp.MustCompile(`(?is)<title\b[^>]*>.*?</title>`)
var initialSeoMeta = regexp.MustCompile(`(?is)<meta\b[^>]*(?:name\s*=\s*["'](?:description|robots|twitter:[^"']*)["']|property\s*=\s*["']og:[^"']*["'])[^>]*>`)
var initialCanonical = regexp.MustCompile(`(?is)<link\b[^>]*rel\s*=\s*["']canonical["'][^>]*>`)
var initialApp = regexp.MustCompile(`<div\s+id=["']app["']\s*>\s*</div>`)

func renderStorefront(index []byte, page StorefrontPage) ([]byte, error) {
	var head, body bytes.Buffer
	if err := storefrontHead.Execute(&head, page); err != nil {
		return nil, err
	}
	if err := storefrontBody.Execute(&body, page); err != nil {
		return nil, err
	}
	if page.StructuredData != nil {
		data, err := json.Marshal(page.StructuredData)
		if err != nil {
			return nil, err
		}
		head.WriteString(`<script data-storefront-seo type="application/ld+json">`)
		head.Write(data) // encoding/json escapes HTML delimiters, including </script>.
		head.WriteString(`</script>`)
	}
	output := initialTitle.ReplaceAll(index, nil)
	output = initialSeoMeta.ReplaceAll(output, nil)
	output = initialCanonical.ReplaceAll(output, nil)
	output = bytes.Replace(output, []byte("</head>"), append(head.Bytes(), []byte("</head>")...), 1)
	output = initialApp.ReplaceAllFunc(output, func([]byte) []byte { return []byte(`<div id="app">` + body.String() + `</div>`) })
	return output, nil
}

func serveStorefront(c *gin.Context, index []byte, resolve StorefrontResolver) {
	page, err := resolve(c.Request)
	if err != nil {
		page = StorefrontPage{Status: 503, Noindex: true, Title: "暂时无法加载页面", Paragraphs: []string{"请稍后重试。"}}
	}
	if page.Status == 0 {
		page.Status = http.StatusOK
	}
	if page.Noindex {
		c.Header("X-Robots-Tag", "noindex, follow")
	}
	if page.Status == 503 {
		c.Header("Retry-After", "60")
	}
	c.Header("Cache-Control", "private, no-store")
	if page.Title == "" {
		c.Data(page.Status, "text/html; charset=utf-8", index)
		return
	}
	body, err := renderStorefront(index, page)
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	c.Data(page.Status, "text/html; charset=utf-8", body)
}

func privateStorefrontPath(path string) bool {
	for _, prefix := range []string{"auth", "me", "cart", "checkout", "pay", "orders", "recharge-orders", "guest", "reseller", "affiliate"} {
		if path == "/"+prefix || strings.HasPrefix(path, "/"+prefix+"/") {
			return true
		}
	}
	return false
}
