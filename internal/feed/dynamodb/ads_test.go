package dynamodb

import (
	"context"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

var _ = Describe("feed ad repository", func() {
	It("lists all eligible pages", func() {
		ctrl := gomock.NewController(GinkgoT())
		source := NewMockRepository(ctrl)
		var seen []ad.ListQuery
		pages := []ad.ListResult{{Items: []ad.Advertisement{{ID: "ad-1"}}, NextCursor: "next", HasMore: true}, {Items: []ad.Advertisement{{ID: "ad-2"}}, HasMore: false}}
		source.EXPECT().List(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(_ context.Context, query ad.ListQuery) (ad.ListResult, error) {
			seen = append(seen, query)
			page := pages[0]
			pages = pages[1:]
			return page, nil
		})
		repository := NewAdRepository(source)
		items, err := repository.List(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(2))
		Expect(items[0].ID).To(Equal("ad-1"))
		Expect(items[1].ID).To(Equal("ad-2"))
		Expect(seen).To(HaveLen(2))
		Expect(seen[0].Limit).To(Equal(50))
		Expect(seen[1].Cursor).To(Equal("next"))
	})
})
