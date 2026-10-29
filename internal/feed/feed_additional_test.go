package feed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("feed handler and service edge cases", func() {
	It("uses documented author fields", func() {
		author := user.User{ID: fmtID(1000), Username: "alice", DisplayName: "Alice"}
		service := newFeedServiceWithAuthors(makePosts(1), nil, time.UnixMilli(3000), map[string]user.User{fmtID(1000): author}, nil)
		handler := NewHandler(service)
		recorder := httptest.NewRecorder()
		handler.Get(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil))
		Expect(recorder.Code).To(Equal(http.StatusOK))
		var response Response
		Expect(json.Unmarshal(recorder.Body.Bytes(), &response)).To(Succeed())
		data, ok := response.Items[0].Data.(map[string]any)
		Expect(ok).To(BeTrue())
		got, ok := data["author"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(got["id"]).To(Equal(author.ID))
		Expect(got["username"]).To(Equal(author.Username))
		Expect(got["displayName"]).To(Equal(author.DisplayName))
		_, exists := got["email"]
		Expect(exists).To(BeFalse())
	})

	It("distinguishes explicit zero from an omitted limit", func() {
		service := newFeedService(makePosts(1), nil, time.UnixMilli(3000))
		_, err := service.Build(context.Background(), Query{Limit: 0, LimitSet: true})
		Expect(err).To(MatchError(ErrInvalidQuery))
		result, err := service.Build(context.Background(), Query{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Items).To(HaveLen(1))
	})

	It("rejects authorization whitespace variants", func() {
		service := newFeedService(makePosts(1), nil, time.UnixMilli(3000))
		router := NewRouter(NewHandler(service), newFeedAuthenticator("user-1", nil))
		for _, header := range []string{"Bearer", "Bearer ", "Bearer  token", "Bearer token ", "Bearer tok\ten", "Bearer tok en", "bearer token", "Basic token"} {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil)
			request.Header.Set("Authorization", header)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			Expect(recorder.Code).To(Equal(http.StatusUnauthorized), header)
		}
	})

	It("pages more than fifty eligible ads without early failure", func() {
		ads := make([]ad.Advertisement, 51)
		for i := range ads {
			ads[i] = activeAd(i)
		}
		service := newFeedService(makePosts(1020), ads, time.UnixMilli(3000))
		var cursor string
		shown := map[string]bool{}
		for page := 0; page < 51; page++ {
			result, err := service.Build(context.Background(), Query{Limit: 20, Cursor: cursor})
			Expect(err).NotTo(HaveOccurred())
			for _, item := range result.Items {
				if item.Ad != nil {
					shown[item.Ad.ID] = true
				}
			}
			if !result.HasMore {
				break
			}
			cursor = result.NextCursor
		}
		Expect(shown).To(HaveLen(51))
	})
})
