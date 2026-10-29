package ad

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GET /api/v1/ads", func() {
	const adminID = "0192f8a0-7b00-7000-8000-000000000901"

	newRouter := func(repository *repositoryFixture, authError error) http.Handler {
		service := NewService(repository, newAdIDs(adminID, nil), newAdClock(time.Time{}))
		return NewRouter(NewHandler(service), newAdminAuth(adminID, authError))
	}

	It("requires an administrator and returns 403 for a valid regular-user token", func() {
		repository := newRepositoryFixture(Advertisement{}, nil, ListResult{})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/ads", nil)
		request.Header.Set("Authorization", "Bearer user-token")

		response := httptest.NewRecorder()
		newRouter(repository, auth.ErrForbidden).ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusForbidden))
		Expect(response.Body.String()).To(ContainSubstring(`"code":"FORBIDDEN"`))
		Expect(repository.listCalls).To(Equal(0))
	})

	It("uses active status and the default limit when query parameters are omitted", func() {
		repository := newRepositoryFixture(Advertisement{}, nil, ListResult{Items: []Advertisement{}})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/ads", nil)
		request.Header.Set("Authorization", "Bearer admin-token")

		response := httptest.NewRecorder()
		newRouter(repository, nil).ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(repository.listQuery).To(Equal(ListQuery{Status: StatusActive, Limit: 20, UserID: adminID}))
		Expect(response.Body.String()).To(MatchJSON(`{"items":[],"hasMore":false}`))
	})

	DescribeTable("rejects invalid status and limit values", func(query string) {
		repository := newRepositoryFixture(Advertisement{}, nil, ListResult{})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/ads?"+query, nil)
		request.Header.Set("Authorization", "Bearer admin-token")

		response := httptest.NewRecorder()
		newRouter(repository, nil).ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusBadRequest))
		Expect(response.Body.String()).To(ContainSubstring(`"code":"INVALID_QUERY"`))
		Expect(repository.listCalls).To(Equal(0))
	},
		Entry("unknown status", "status=unknown"),
		Entry("unknown parameter", "unexpected=value"),
		Entry("duplicate status", "status=active&status=paused"),
		Entry("duplicate limit", "limit=2&limit=3"),
		Entry("zero limit", "limit=0"),
		Entry("over maximum limit", "limit=101"),
		Entry("non-numeric limit", "limit=not-a-number"),
	)

	It("passes a valid status, limit, and cursor to the service and returns pagination metadata", func() {
		cursor := validListCursor(adminID, StatusPaused, 7)
		repository := newRepositoryFixture(Advertisement{ID: adminID, Type: "ad", Status: StatusActive}, nil, ListResult{Items: []Advertisement{{ID: adminID, Type: "ad", Status: StatusActive}}, NextCursor: cursor, HasMore: true})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/ads?status=paused&limit=7&cursor="+cursor, nil)
		request.Header.Set("Authorization", "Bearer admin-token")

		response := httptest.NewRecorder()
		newRouter(repository, nil).ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusOK))
		Expect(repository.listQuery).To(Equal(ListQuery{Status: StatusPaused, Limit: 7, Cursor: cursor, UserID: adminID}))
		Expect(response.Body.String()).To(ContainSubstring(`"nextCursor":"` + cursor + `"`))
		Expect(response.Body.String()).To(ContainSubstring(`"hasMore":true`))
	})

	DescribeTable("rejects authentication and raw-query boundary failures", func(authorization, query, code string, status int) {
		repository := newRepositoryFixture(Advertisement{}, nil, ListResult{})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/ads?"+query, nil)
		request.Header.Set("Authorization", authorization)
		request.Header.Set("X-Request-ID", "req-ad-list-boundary")

		response := httptest.NewRecorder()
		authError := error(nil)
		if strings.HasPrefix(authorization, "Bearer ") && authorization != "Bearer admin-token" {
			authError = errors.New("invalid access token")
		}
		newRouter(repository, authError).ServeHTTP(response, request)

		messages := map[string]string{"UNAUTHORIZED": "Authentication required", "INVALID_TOKEN": "Invalid access token", "INVALID_QUERY": "Invalid query parameters"}
		Expect(response.Code).To(Equal(status))
		Expect(response.Header().Get("Content-Type")).To(Equal("application/json"))
		Expect(response.Body.String()).To(MatchJSON(`{"error":{"code":"` + code + `","message":"` + messages[code] + `","requestId":"req-ad-list-boundary"}}`))
		Expect(repository.listCalls).To(Equal(0))
	},
		Entry("missing authorization", "", "", "UNAUTHORIZED", http.StatusUnauthorized),
		Entry("malformed bearer scheme", "Basic token", "", "UNAUTHORIZED", http.StatusUnauthorized),
		Entry("empty bearer token", "Bearer ", "", "UNAUTHORIZED", http.StatusUnauthorized),
		Entry("invalid bearer token", "Bearer malformed", "", "INVALID_TOKEN", http.StatusUnauthorized),
		Entry("malformed raw query", "Bearer admin-token", "limit=%zz", "INVALID_QUERY", http.StatusBadRequest),
		Entry("empty query value", "Bearer admin-token", "limit=", "INVALID_QUERY", http.StatusBadRequest),
		Entry("duplicate cursor", "Bearer admin-token", "cursor=one&cursor=two", "INVALID_QUERY", http.StatusBadRequest),
	)

	It("rejects a regular user with a structured forbidden response and safe log fields", func() {
		logger := newAdLogger(nil)
		service := NewService(newRepositoryFixture(Advertisement{}, nil, ListResult{}), newAdIDs("", nil), newAdClock(time.Time{}))
		router := NewRouter(NewHandler(service, logger), newAdminAuth("", auth.ErrForbidden))
		request := httptest.NewRequest(http.MethodGet, "/api/v1/ads", nil)
		request.Header.Set("Authorization", "Bearer user-token")
		request.Header.Set("X-Request-ID", "req-ad-list-forbidden")
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusForbidden))
		Expect(response.Body.String()).To(MatchJSON(`{"error":{"code":"FORBIDDEN","message":"Administrator role required","requestId":"req-ad-list-forbidden"}}`))
		Expect(logger.errorMessage).To(Equal("ad request failed"))
		Expect(logger.errorFields).To(ContainElement(logging.String("request_id", "req-ad-list-forbidden")))
		Expect(logger.errorFields).To(ContainElement(logging.String("route", "GET /api/v1/ads")))
		Expect(logger.errorFields).To(ContainElement(logging.String("status", "403")))
		Expect(fieldsText(logger.errorFields)).NotTo(ContainSubstring("user-token"))
	})

	DescribeTable("maps list errors to documented responses", func(query string, repositoryErr error, code string, status int) {
		repository := newRepositoryFixture(Advertisement{}, repositoryErr, ListResult{})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/ads?"+query, nil)
		request.Header.Set("Authorization", "Bearer admin-token")
		response := httptest.NewRecorder()
		newRouter(repository, nil).ServeHTTP(response, request)
		Expect(response.Code).To(Equal(status))
		Expect(response.Body.String()).To(ContainSubstring(`"code":"` + code + `"`))
	},
		Entry("invalid cursor", "cursor=%25%25%25", nil, "INVALID_CURSOR", http.StatusBadRequest),
		Entry("dependency failure", "", ErrDependencyUnavailable, "SERVICE_UNAVAILABLE", http.StatusServiceUnavailable),
	)
})

func validListCursor(userID string, status Status, limit int) string {
	now := time.Now().Unix()
	payload, err := json.Marshal(struct {
		Version   int    `json:"v"`
		KeyID     string `json:"kid"`
		UserID    string `json:"uid"`
		Status    string `json:"status"`
		Limit     int    `json:"limit"`
		IssuedAt  int64  `json:"iat"`
		ExpiresAt int64  `json:"exp"`
		LastAdID  string `json:"lastAdId"`
	}{1, "local", userID, string(status), limit, now, now + 900, "0192f8a0-7b00-7000-8000-000000000999"})
	Expect(err).NotTo(HaveOccurred())
	signer := hmac.New(sha256.New, []byte("local-development-ads-cursor-key-change-me"))
	_, err = signer.Write(payload)
	Expect(err).NotTo(HaveOccurred())
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signer.Sum(nil))
}

var _ = Describe("advertisement list service validation", func() {
	It("defaults an empty status to active and rejects invalid query values before persistence", func() {
		repository := newRepositoryFixture(Advertisement{}, nil, ListResult{Items: []Advertisement{}})
		service := NewService(repository, newAdIDs("", nil), newAdClock(time.Time{}))

		_, err := service.List(context.Background(), ListQuery{Limit: 3})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.listQuery.Status).To(Equal(StatusActive))

		for _, query := range []ListQuery{{Status: Status("invalid"), Limit: 3}, {Status: StatusActive, Limit: 0}, {Status: StatusActive, Limit: 101}} {
			_, err = service.List(context.Background(), query)
			Expect(err).To(MatchError(ErrInvalidQuery))
		}
		Expect(repository.listCalls).To(Equal(1))
	})

	It("rejects malformed, whitespace-padded, and oversized cursors", func() {
		service := NewService(newRepositoryFixture(Advertisement{}, nil, ListResult{}), newAdIDs("", nil), newAdClock(time.Time{}))
		for _, cursor := range []string{"%%%", " eyJ4IjoxfQ", strings.Repeat("a", 4097)} {
			_, err := service.List(context.Background(), ListQuery{Status: StatusActive, Limit: 20, Cursor: cursor})
			Expect(errors.Is(err, ErrInvalidCursor)).To(BeTrue(), cursor)
		}
	})
})
