package feed

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	"github.com/antoniuk-oleksandr/social-media/internal/post"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

func newFeedService(items []post.Post, ads []ad.Advertisement, now time.Time) Service {
	ctrl := gomock.NewController(GinkgoT())
	posts := NewMockPostSource(ctrl)
	postValues := append([]post.Post(nil), items...)
	posts.EXPECT().List(gomock.Any(), gomock.Any()).AnyTimes().Return(postValues, nil)
	advertisements := NewMockAdSource(ctrl)
	adValues := append([]ad.Advertisement(nil), ads...)
	advertisements.EXPECT().List(gomock.Any()).AnyTimes().Return(adValues, nil)
	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().AnyTimes().Return(now)
	return NewService(posts, advertisements, clock, "test-key", "v1")
}

func newFeedAuthenticator(id string, err error) *MockTokenAuthenticator {
	auth := NewMockTokenAuthenticator(gomock.NewController(GinkgoT()))
	auth.EXPECT().Authenticate(gomock.Any(), gomock.Any()).AnyTimes().Return(id, err)
	return auth
}

func newFeedServiceWithAuthors(items []post.Post, ads []ad.Advertisement, now time.Time, authors map[string]user.User, authorErr error) Service {
	ctrl := gomock.NewController(GinkgoT())
	posts := NewMockPostSource(ctrl)
	posts.EXPECT().List(gomock.Any(), gomock.Any()).AnyTimes().Return(append([]post.Post(nil), items...), nil)
	advertisements := NewMockAdSource(ctrl)
	advertisements.EXPECT().List(gomock.Any()).AnyTimes().Return(append([]ad.Advertisement(nil), ads...), nil)
	clock := NewMockClock(ctrl)
	clock.EXPECT().Now().AnyTimes().Return(now)
	authorSource := NewMockAuthorSource(ctrl)
	authorSource.EXPECT().GetByIDs(gomock.Any(), gomock.Any()).AnyTimes().Return(authors, authorErr)
	return NewService(posts, advertisements, clock, "key", "v1", authorSource)
}
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
			svc := newFeedService(makePosts(45), ads, now)
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
		svc := newFeedService(makePosts(5), nil, time.UnixMilli(3000))
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
		svc := newFeedService(makePosts(5), []ad.Advertisement{activeAd(0)}, time.UnixMilli(3000))
		result, err := svc.Build(context.Background(), Query{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Items).To(HaveLen(6))
		Expect(result.Items[5].Ad).NotTo(BeNil())
		Expect(result.HasMore).To(BeFalse())
	})

	It("does not invent an ad for a truly empty feed", func() {
		svc := newFeedService(nil, []ad.Advertisement{activeAd(0)}, time.UnixMilli(3000))
		result, err := svc.Build(context.Background(), Query{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Items).To(BeEmpty())
		Expect(result.NextCursor).To(BeEmpty())
		Expect(result.HasMore).To(BeFalse())
	})

	It("rejects cursor limit mismatches", func() {
		now := time.UnixMilli(3000)
		svc := newFeedService(makePosts(25), nil, now)
		r, e := svc.Build(context.Background(), Query{Limit: 20})
		Expect(e).NotTo(HaveOccurred())
		Expect(r.HasMore).To(BeTrue())
		_, e = svc.Build(context.Background(), Query{Limit: 10, Cursor: r.NextCursor})
		Expect(e).To(MatchError(ErrInvalidCursor))
		Expect(strings.TrimSpace(r.NextCursor)).To(Equal(r.NextCursor))
	})
})
