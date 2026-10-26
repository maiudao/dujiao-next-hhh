package web

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestInitialStorefrontHTMLAndStatusCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	index := `<html><head><title>old</title><meta name="description" content="old"><link rel="canonical" href="old"></head><body><div id="app"></div><script src="/assets/app.js"></script></body></html>`
	r := gin.New()
	err := RegisterUser(r, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(index)}}, func(req *http.Request) (StorefrontPage, error) {
		switch req.URL.Path {
		case "/products/real":
			return StorefrontPage{Title: `Public <script>alert(1)</script>`, Description: "Description", Canonical: "https://shop.example/products/real", Paragraphs: []string{`Body <img onerror=alert(1)>`}, Links: []StorefrontLink{{Title: "Catalog", Href: "/products"}}, StructuredData: map[string]any{"@type": "Product", "name": "</script><script>unsafe()</script>"}}, nil
		case "/failure":
			return StorefrontPage{}, errors.New("database error")
		default:
			return StorefrontPage{Status: 404, Title: "Missing", Noindex: true}, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		path   string
		status int
		robots string
	}{{"/products/real", 200, ""}, {"/missing", 404, "noindex"}, {"/failure", 503, "noindex"}, {"/checkout", 200, "noindex"}, {"/api/unknown", 404, ""}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", tt.path, nil))
		if w.Code != tt.status || !strings.Contains(w.Header().Get("X-Robots-Tag"), tt.robots) {
			t.Fatalf("%s: status=%d headers=%v", tt.path, w.Code, w.Header())
		}
		body := w.Body.String()
		if strings.Contains(body, "<script>alert(1)</script>") || strings.Contains(body, "<script>unsafe()</script>") || strings.Contains(body, "<img onerror") {
			t.Fatal("unescaped public content")
		}
		if tt.path == "/products/real" {
			for _, want := range []string{"<h1>Public", "Body &lt;img", "/assets/app.js", "https://shop.example/products/real", `href="/products"`} {
				if !strings.Contains(body, want) {
					t.Fatalf("initial HTML missing %q", want)
				}
			}
			if strings.Count(body, `rel="canonical"`) != 1 || strings.Count(body, "<title") != 1 {
				t.Fatal("duplicate metadata")
			}
		}
	}
}
