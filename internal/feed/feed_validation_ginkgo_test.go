package feed

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("feed request validation", func() {
	It("rejects malformed, unknown, duplicate, and non-canonical raw queries", func() {
		invalid := []string{
			"limit", "limit=", "limit=1&", "&limit=1", "unknown=1",
			"limit=1&limit=2", "LIMIT=1", "limit=01", "limit=+1", "limit=%zz",
		}
		for _, raw := range invalid {
			_, err := parseQuery(raw)
			Expect(err).To(MatchError(ErrInvalidQuery), raw)
		}
		valid, err := parseQuery("limit=20&cursor=abc")
		Expect(err).NotTo(HaveOccurred())
		Expect(valid).To(Equal(map[string]string{"limit": "20", "cursor": "abc"}))
	})

	It("rejects invalid RFC token68 values before authenticating", func() {
		router := NewRouter(NewHandler(newFeedService(makePosts(1), nil, time.UnixMilli(3000))), newFeedAuthenticator("user-1", nil))
		for _, header := range []string{"Bearer", "Bearer ", "Bearer =", "Bearer ==", "Bearer token ", "Bearer tok en", "Bearer tok\ten", "Bearer token==x", "bearer token"} {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/feed", nil)
			request.Header.Set("Authorization", header)
			router.ServeHTTP(recorder, request)
			Expect(recorder.Code).To(Equal(http.StatusUnauthorized), header)
		}
	})

	It("includes the looked-up author and translates lookup failures", func() {
		author := user.User{ID: fmtID(1000), Username: "alice", DisplayName: "Alice"}
		service := newFeedServiceWithAuthors(makePosts(1), nil, time.UnixMilli(3000), map[string]user.User{author.ID: author}, nil)
		result, err := service.Build(context.Background(), Query{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Items[0].Author).To(Equal(&author))

		failed := newFeedServiceWithAuthors(makePosts(1), nil, time.UnixMilli(3000), nil, errors.New("lookup failed"))
		_, err = failed.Build(context.Background(), Query{})
		Expect(err).To(MatchError(ErrDependencyUnavailable))
	})
})
