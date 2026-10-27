package dynamodb

import (
	"context"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type pagedAds struct {
	pages []ad.ListResult
	seen  []ad.ListQuery
}

func (p *pagedAds) Create(context.Context, ad.Advertisement) error { return nil }
func (p *pagedAds) GetByID(context.Context, string) (ad.Advertisement, error) {
	return ad.Advertisement{}, nil
}
func (p *pagedAds) Update(context.Context, ad.Advertisement) error { return nil }
func (p *pagedAds) Delete(context.Context, string) error           { return nil }
func (p *pagedAds) List(_ context.Context, query ad.ListQuery) (ad.ListResult, error) {
	p.seen = append(p.seen, query)
	page := p.pages[0]
	p.pages = p.pages[1:]
	return page, nil
}

var _ = Describe("feed ad repository", func() {
	It("lists all eligible pages", func() {
		source := &pagedAds{pages: []ad.ListResult{{Items: []ad.Advertisement{{ID: "ad-1"}}, NextCursor: "next", HasMore: true}, {Items: []ad.Advertisement{{ID: "ad-2"}}, HasMore: false}}}
		repository := NewAdRepository(source)
		items, err := repository.List(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(2))
		Expect(items[0].ID).To(Equal("ad-1"))
		Expect(items[1].ID).To(Equal("ad-2"))
		Expect(source.seen).To(HaveLen(2))
		Expect(source.seen[0].Limit).To(Equal(50))
		Expect(source.seen[1].Cursor).To(Equal("next"))
	})
})
