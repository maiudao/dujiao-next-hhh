package categoryhttp

import (
	categorydomain "github.com/dujiao-next/internal/modules/catalog/category/domain"
	categorypresenter "github.com/dujiao-next/internal/modules/catalog/category/transport/presenter"
	"github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/dujiao-next/internal/platform/imageutil"

	"github.com/gin-gonic/gin"
)

type PublicQueries interface {
	ListActive() ([]categorydomain.Category, error)
}

type PublicHandler struct {
	queries PublicQueries
	icons   *imageutil.IconThumbnails
}

func NewPublicHandler(queries PublicQueries) *PublicHandler {
	if queries == nil {
		panic("category public handler: queries are nil")
	}
	return &PublicHandler{queries: queries, icons: imageutil.NewIconThumbnails("./uploads")}
}

func (handler *PublicHandler) List(c *gin.Context) {
	categories, err := handler.queries.ListActive()
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.category_fetch_failed", err)
		return
	}
	type publicCategory struct {
		categorypresenter.Category
		IconThumbnail string `json:"icon_thumbnail,omitempty"`
	}
	result := make([]publicCategory, 0, len(categories))
	for _, category := range categories {
		result = append(result, publicCategory{
			Category:      categorypresenter.New(&category),
			IconThumbnail: handler.icons.DataURL(category.Icon),
		})
	}
	response.Success(c, result)
}
