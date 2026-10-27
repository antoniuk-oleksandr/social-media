package feed

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	"github.com/antoniuk-oleksandr/social-media/internal/post"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type failingPosts struct{ err error }

func (f failingPosts) List(context.Context, time.Time) ([]post.Post, error) { return nil, f.err }

type failingAds struct{ err error }

func (f failingAds) List(context.Context) ([]ad.Advertisement, error) { return nil, f.err }

type snapshotPosts struct{ items []post.Post }

func (s *snapshotPosts) List(_ context.Context, snapshot time.Time) ([]post.Post, error) {
	var visible []post.Post
	for _, item := range s.items {
		if !item.CreatedAt.After(snapshot) {
			visible = append(visible, item)
		}
	}
	return visible, nil
}

var _ = Describe("feed contract", func() {
	It("binds continuation cursors to the authenticated user", func() {
		now := time.UnixMilli(3000)
		svc := NewService(fakePosts{makePosts(21)}, fakeAds{}, fakeClock{now}, "key", "v1")

		first, err := svc.Build(context.Background(), Query{Limit: 20, UserID: "user-a"})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.NextCursor).NotTo(BeEmpty())

		_, err = svc.Build(context.Background(), Query{Limit: 20, UserID: "user-b", Cursor: first.NextCursor})
		Expect(err).To(MatchError(ErrInvalidCursor))
	})

	It("excludes posts created after the first page snapshot", func() {
		now := time.UnixMilli(3000).UTC()
		posts := &snapshotPosts{items: makePosts(21)}
		svc := NewService(posts, fakeAds{}, fakeClock{now}, "key", "v1")

		first, err := svc.Build(context.Background(), Query{Limit: 20, UserID: "user-a"})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.HasMore).To(BeTrue())

		posts.items = append(posts.items, post.Post{
			ID:             fmtID(9999),
			AuthorID:       fmtID(1000),
			Content:        "newer post",
			CreatedAt:      now.Add(time.Millisecond),
			CreatedAtEpoch: now.Add(time.Millisecond).UnixMilli(),
		})

		continuation, err := svc.Build(context.Background(), Query{Limit: 20, UserID: "user-a", Cursor: first.NextCursor})
		Expect(err).NotTo(HaveOccurred())
		Expect(continuation.Items).To(HaveLen(1))
		Expect(continuation.Items[0].Post.ID).To(Equal(fmtID(20)))
		Expect(continuation.Items[0].Post.ID).NotTo(Equal(fmtID(9999)))
	})

	It("orders ties deterministically and completes the final page", func() {
		now := time.UnixMilli(3000)
		posts := makePosts(45)
		posts[0].CreatedAtEpoch, posts[1].CreatedAtEpoch = 2000, 2000
		posts[0].ID, posts[1].ID = fmtID(2), fmtID(1)
		svc := NewService(fakePosts{posts}, fakeAds{[]ad.Advertisement{activeAd(0)}}, fakeClock{now}, "test-key", "v1")
		var cursor string
		var postIDs []string
		var adPositions []int
		for page := 0; page < 3; page++ {
			result, err := svc.Build(context.Background(), Query{Limit: 20, Cursor: cursor})
			Expect(err).NotTo(HaveOccurred())
			wantItems := 21
			if page == 2 {
				wantItems = 6
			}
			Expect(result.Items).To(HaveLen(wantItems), "page %d", page)
			for i, item := range result.Items {
				if item.Post != nil {
					postIDs = append(postIDs, item.Post.ID)
				}
				if item.Ad != nil {
					adPositions = append(adPositions, i)
				}
			}
			Expect(adPositions).To(HaveLen(page + 1))
			Expect(result.HasMore).To(Equal(page < 2))
			cursor = result.NextCursor
		}
		Expect(postIDs).To(HaveLen(45))
		Expect(postIDs[:2]).To(Equal([]string{fmtID(2), fmtID(1)}))
	})

	It("applies query defaults and bounds", func() {
		svc := NewService(fakePosts{makePosts(1)}, fakeAds{}, fakeClock{time.UnixMilli(3000)}, "key", "v1")
		result, err := svc.Build(context.Background(), Query{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Items).To(HaveLen(1))
		for _, limit := range []int{-1, 51} {
			_, err = svc.Build(context.Background(), Query{Limit: limit})
			Expect(errors.Is(err, ErrInvalidQuery)).To(BeTrue(), "limit %d", limit)
		}
	})

	It("rejects expired cursors and translates dependency failures", func() {
		clock := fakeClock{time.Unix(3000, 0)}
		svc := NewService(fakePosts{makePosts(21)}, fakeAds{}, clock, "key", "v1")
		concrete := svc.(*service)
		first, err := svc.Build(context.Background(), Query{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		var c cursor
		parts := strings.Split(first.NextCursor, ".")
		payload, err := decodeCursorPayload(parts[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(payload, &c)).To(Succeed())
		c.Expires = clock.Now().Unix()
		expired, err := concrete.sign(c)
		Expect(err).NotTo(HaveOccurred())
		_, err = svc.Build(context.Background(), Query{Limit: 20, Cursor: expired})
		Expect(errors.Is(err, ErrCursorExpired)).To(BeTrue())
		for _, source := range []Service{NewService(failingPosts{errors.New("posts down")}, fakeAds{}, clock, "key", "v1"), NewService(fakePosts{makePosts(1)}, failingAds{errors.New("ads down")}, clock, "key", "v1")} {
			_, err = source.Build(context.Background(), Query{})
			Expect(errors.Is(err, ErrDependencyUnavailable)).To(BeTrue())
		}
	})

	It("maps handler responses according to the contract", func() {
		now := time.UnixMilli(3000)
		baseService := NewService(fakePosts{makePosts(2)}, fakeAds{}, fakeClock{now}, "key", "v1")
		handler := NewHandler(baseService)
		for _, test := range []struct {
			name, path string
			status     int
			code       string
		}{
			{"success without authentication", "/api/v1/feed", http.StatusOK, ""}, {"default limit", "/api/v1/feed?limit=20", http.StatusOK, ""}, {"zero limit", "/api/v1/feed?limit=0", http.StatusBadRequest, "INVALID_QUERY"}, {"above maximum", "/api/v1/feed?limit=51", http.StatusBadRequest, "INVALID_QUERY"}, {"invalid syntax", "/api/v1/feed?limit=nope", http.StatusBadRequest, "INVALID_QUERY"}, {"duplicate query", "/api/v1/feed?limit=1&limit=2", http.StatusBadRequest, "INVALID_QUERY"}, {"cursor whitespace", "/api/v1/feed?cursor=%20bad", http.StatusBadRequest, "INVALID_CURSOR"},
		} {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.Header.Set("X-Request-ID", "request-test")
			handler.Get(recorder, request)
			Expect(recorder.Code).To(Equal(test.status), test.name)
			if test.code != "" {
				Expect(recorder.Body.String()).To(ContainSubstring(`"code":"` + test.code + `"`))
			}
		}
		router := NewRouter(handler, fakeAuthenticator{id: "user-1"})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil)
		request.Header.Set("Authorization", "Bearer token")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		Expect(recorder.Code).To(Equal(http.StatusOK))
		request = httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil)
		request.Header.Set("Authorization", "Basic token")
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
		Expect(recorder.Body.String()).To(ContainSubstring(`"code":"INVALID_TOKEN"`))
		dependencyHandler := NewHandler(NewService(failingPosts{errors.New("database down")}, fakeAds{}, fakeClock{now}, "key", "v1"))
		recorder = httptest.NewRecorder()
		dependencyHandler.Get(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil))
		Expect(recorder.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(recorder.Body.String()).To(ContainSubstring(`"code":"SERVICE_UNAVAILABLE"`))
		cursorService := NewService(fakePosts{makePosts(21)}, fakeAds{}, fakeClock{now}, "key", "v1")
		first, err := cursorService.Build(context.Background(), Query{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		recorder = httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodGet, "/api/v1/feed?limit=20&cursor="+first.NextCursor+"tampered", nil)
		handler.Get(recorder, request)
		Expect(recorder.Code).To(Equal(http.StatusBadRequest))
		Expect(recorder.Body.String()).To(ContainSubstring(`"code":"INVALID_CURSOR"`))
		concrete := cursorService.(*service)
		parts := strings.Split(first.NextCursor, ".")
		payload, err := decodeCursorPayload(parts[0])
		Expect(err).NotTo(HaveOccurred())
		var expiredCursor cursor
		Expect(json.Unmarshal(payload, &expiredCursor)).To(Succeed())
		expiredCursor.Expires = now.Unix()
		expired, err := concrete.sign(expiredCursor)
		Expect(err).NotTo(HaveOccurred())
		recorder = httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodGet, "/api/v1/feed?limit=20&cursor="+expired, nil)
		handler.Get(recorder, request)
		Expect(recorder.Code).To(Equal(http.StatusBadRequest))
		Expect(recorder.Body.String()).To(ContainSubstring(`"code":"CURSOR_EXPIRED"`))
	})
})

func decodeCursorPayload(value string) ([]byte, error) { return decodeBase64(value) }
func decodeBase64(value string) ([]byte, error)        { return base64.RawURLEncoding.DecodeString(value) }

type fakeAuthenticator struct{ id string }

func (f fakeAuthenticator) Authenticate(context.Context, string) (string, error) { return f.id, nil }
