package feed

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	"github.com/antoniuk-oleksandr/social-media/internal/post"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakePosts struct{ items []post.Post }

func (f fakePosts) List(context.Context, time.Time) ([]post.Post, error) {
	return append([]post.Post(nil), f.items...), nil
}

type fakeAds struct{ items []ad.Advertisement }

func (f fakeAds) List(context.Context) ([]ad.Advertisement, error) {
	return append([]ad.Advertisement(nil), f.items...), nil
}

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }
func makePosts(n int) []post.Post {
	out := make([]post.Post, n)
	for i := range out {
		t := time.UnixMilli(int64(2000 - i))
		out[i] = post.Post{ID: fmtID(i), AuthorID: fmtID(1000), Content: "post", CreatedAt: t, CreatedAtEpoch: t.UnixMilli()}
	}
	return out
}
func fmtID(i int) string { return "0192c5f4-7b2a-7abc-8def-" + fmt.Sprintf("%012d", i) }
func activeAd(i int) ad.Advertisement {
	now := time.UnixMilli(3000)
	return ad.Advertisement{ID: fmtID(2000 + i), Type: "ad", Status: ad.StatusActive, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), CreatedAt: now}
}

var _ = Describe("feed service", func() {
	It("inserts ads across pages without duplicate posts", func() {
		now := time.UnixMilli(3000)
		ads := []ad.Advertisement{activeAd(0), activeAd(1), activeAd(2)}
		collect := func() ([]string, []string) {
			svc := NewService(fakePosts{makePosts(45)}, fakeAds{ads}, fakeClock{now}, "test-key", "v1")
			var cursor string
			seenPosts := map[string]bool{}
			var postIDs, adIDs []string
			for page := 0; page < 3; page++ {
				r, e := svc.Build(context.Background(), Query{Limit: 20, Cursor: cursor})
				Expect(e).NotTo(HaveOccurred())
				Expect(r.Items).To(HaveLen([]int{21, 21, 6}[page]))
				Expect(r.Items[len(r.Items)-1].Ad).NotTo(BeNil())
				for _, item := range r.Items {
					if item.Post != nil {
						Expect(seenPosts[item.Post.ID]).To(BeFalse(), "duplicate post %s", item.Post.ID)
						seenPosts[item.Post.ID] = true
						postIDs = append(postIDs, item.Post.ID)
					}
					if item.Ad != nil {
						Expect(adIDs).NotTo(ContainElement(item.Ad.ID), "duplicate ad %s", item.Ad.ID)
						adIDs = append(adIDs, item.Ad.ID)
					}
				}
				cursor = r.NextCursor
				Expect(r.HasMore).To(Equal(page < 2))
			}
			Expect(seenPosts).To(HaveLen(45))
			return postIDs, adIDs
		}

		firstPosts, firstAds := collect()
		secondPosts, secondAds := collect()
		Expect(secondPosts).To(Equal(firstPosts))
		Expect(secondAds).To(Equal(firstAds))
		Expect(firstAds).To(HaveLen(3))
	})

	It("handles empty ads and rejects invalid cursors", func() {
		svc := NewService(fakePosts{makePosts(5)}, fakeAds{}, fakeClock{time.UnixMilli(3000)}, "test-key", "v1")
		r, e := svc.Build(context.Background(), Query{Limit: 20})
		Expect(e).NotTo(HaveOccurred())
		Expect(r.Items).To(HaveLen(5))
		for _, item := range r.Items {
			Expect(item.Ad).To(BeNil())
		}
		_, e = svc.Build(context.Background(), Query{Limit: 20, Cursor: "tampered"})
		Expect(e).To(MatchError(ErrInvalidCursor))
	})

	It("includes an eligible ad on an incomplete organic page", func() {
		svc := NewService(fakePosts{makePosts(5)}, fakeAds{[]ad.Advertisement{activeAd(0)}}, fakeClock{time.UnixMilli(3000)}, "test-key", "v1")
		result, err := svc.Build(context.Background(), Query{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Items).To(HaveLen(6))
		Expect(result.Items[5].Ad).NotTo(BeNil())
		Expect(result.HasMore).To(BeFalse())
	})

	It("does not invent an ad for a truly empty feed", func() {
		svc := NewService(fakePosts{}, fakeAds{[]ad.Advertisement{activeAd(0)}}, fakeClock{time.UnixMilli(3000)}, "test-key", "v1")
		result, err := svc.Build(context.Background(), Query{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Items).To(BeEmpty())
		Expect(result.NextCursor).To(BeEmpty())
		Expect(result.HasMore).To(BeFalse())
	})

	It("rejects cursor limit mismatches", func() {
		now := time.UnixMilli(3000)
		svc := NewService(fakePosts{makePosts(25)}, fakeAds{}, fakeClock{now}, "test-key", "v1")
		r, e := svc.Build(context.Background(), Query{Limit: 20})
		Expect(e).NotTo(HaveOccurred())
		Expect(r.HasMore).To(BeTrue())
		_, e = svc.Build(context.Background(), Query{Limit: 10, Cursor: r.NextCursor})
		Expect(e).To(MatchError(ErrInvalidCursor))
		Expect(strings.TrimSpace(r.NextCursor)).To(Equal(r.NextCursor))
	})
})
