package dynamodb

import (
	"context"
	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	feed "github.com/antoniuk-oleksandr/social-media/internal/feed"
)

type AdRepository struct{ source ad.Repository }

func NewAdRepository(source ad.Repository) feed.AdSource { return &AdRepository{source: source} }
func (r *AdRepository) List(ctx context.Context) ([]ad.Advertisement, error) {
	items := make([]ad.Advertisement, 0)
	query := ad.ListQuery{Status: ad.StatusActive, Limit: 50, UserID: "feed"}
	for {
		page, err := r.source.List(ctx, query)
		if err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if !page.HasMore {
			return items, nil
		}
		if page.NextCursor == "" {
			return nil, ad.ErrDependencyUnavailable
		}
		query.Cursor = page.NextCursor
	}
}
