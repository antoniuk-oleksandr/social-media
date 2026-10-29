package dynamodb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"

	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

type adminAuthenticator interface {
	AuthenticateAdmin(context.Context, string) (string, error)
}

func listItem(id, status string, epoch int64) map[string]types.AttributeValue {
	created := time.UnixMilli(epoch).UTC().Format(time.RFC3339Nano)
	return map[string]types.AttributeValue{
		"adId": &types.AttributeValueMemberS{Value: id}, "entityType": &types.AttributeValueMemberS{Value: "ad"},
		"advertiser": &types.AttributeValueMemberS{Value: "Example"}, "content": &types.AttributeValueMemberS{Value: "content"},
		"targetUrl": &types.AttributeValueMemberS{Value: "https://example.com"}, "startsAt": &types.AttributeValueMemberS{Value: created},
		"endsAt": &types.AttributeValueMemberS{Value: created}, "status": &types.AttributeValueMemberS{Value: status},
		"createdBy": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000901"},
		"createdAt": &types.AttributeValueMemberS{Value: created}, "createdAtEpoch": &types.AttributeValueMemberN{Value: strconv.FormatInt(epoch, 10)},
	}
}

var _ = Describe("List", func() {
	It("globally orders equal timestamps across DynamoDB pages and resumes after the semantic boundary", func() {
		ids := []string{
			"0192f8a0-7b00-0000-0000-000000000923",
			"0192f8a0-7b00-0000-0000-000000000922",
			"0192f8a0-7b00-0000-0000-000000000921",
		}
		api := newPutItemAPIQuery(nil, []*dynamodb.QueryOutput{
			{Items: []map[string]types.AttributeValue{listItem(ids[1], "active", 5000)}, LastEvaluatedKey: map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: "page-1"}}},
			{Items: []map[string]types.AttributeValue{listItem(ids[0], "active", 5000), listItem(ids[2], "active", 5000)}},
		}, nil)
		repository := &Repository{Client: api, TableName: "ads-test", cursor: newCursorSigner(platformconfig.AdsConfig{CursorKey: "test-cursor-key", CursorKeyID: "v1", CursorVersion: 1})}

		first, err := repository.List(context.Background(), ad.ListQuery{Status: ad.StatusActive, Limit: 2})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Items).To(HaveLen(2))
		Expect([]string{first.Items[0].ID, first.Items[1].ID}).To(Equal(ids[:2]))
		Expect(first.HasMore).To(BeTrue())
		Expect(api.queryCalls).To(Equal(2))

		api.queryOutputs = []*dynamodb.QueryOutput{{Items: []map[string]types.AttributeValue{listItem(ids[2], "active", 5000)}}}
		api.queryCalls = 0
		second, err := repository.List(context.Background(), ad.ListQuery{Status: ad.StatusActive, Limit: 2, Cursor: first.NextCursor})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Items).To(HaveLen(1))
		Expect(second.Items[0].ID).To(Equal(ids[2]))
		Expect(second.HasMore).To(BeFalse())
		Expect(api.queryInput.ExclusiveStartKey).To(BeNil(), "semantic cursors must not become DynamoDB traversal keys")
	})

	It("orders non-tied timestamps before applying the cursor boundary", func() {
		items := []map[string]types.AttributeValue{
			listItem("0192f8a0-7b00-0000-0000-000000000921", "active", 1000),
			listItem("0192f8a0-7b00-0000-0000-000000000923", "active", 3000),
			listItem("0192f8a0-7b00-0000-0000-000000000922", "active", 2000),
		}
		api := newPutItemAPIQuery(nil, []*dynamodb.QueryOutput{{Items: items}}, nil)
		repository := &Repository{Client: api, TableName: "ads-test", cursor: newCursorSigner(platformconfig.AdsConfig{CursorKey: "test-cursor-key", CursorKeyID: "v1", CursorVersion: 1})}
		first, err := repository.List(context.Background(), ad.ListQuery{Status: ad.StatusActive, Limit: 2})
		Expect(err).NotTo(HaveOccurred())
		Expect([]string{first.Items[0].ID, first.Items[1].ID}).To(Equal([]string{items[1]["adId"].(*types.AttributeValueMemberS).Value, items[2]["adId"].(*types.AttributeValueMemberS).Value}))

		api.queryOutputs = []*dynamodb.QueryOutput{{Items: []map[string]types.AttributeValue{items[0]}}}
		api.queryCalls = 0
		second, err := repository.List(context.Background(), ad.ListQuery{Status: ad.StatusActive, Limit: 2, Cursor: first.NextCursor})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Items).To(HaveLen(1))
		Expect(second.Items[0].ID).To(Equal(items[0]["adId"].(*types.AttributeValueMemberS).Value))
	})

	It("queries the active index in reverse order and maps results", func() {
		api := newPutItemAPIQuery(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{
				listItem("0192f8a0-7b00-7000-8000-000000000901", "active", 1000),
				listItem("0192f8a0-7b00-8000-8000-000000000902", "active", 999),
			},
			LastEvaluatedKey: map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: "last"}},
		}, nil, nil)
		result, err := (&Repository{Client: api, TableName: "ads-test"}).List(context.Background(), ad.ListQuery{Status: ad.StatusActive, Limit: 1})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Items).To(HaveLen(1))
		Expect(result.Items[0].Status).To(Equal(ad.StatusActive))
		Expect(result.HasMore).To(BeTrue())
		Expect(result.NextCursor).NotTo(BeEmpty())
		Expect(*api.queryInput.IndexName).To(Equal("ActiveAdsIndex"))
		Expect(*api.queryInput.KeyConditionExpression).To(Equal("#status = :status"))
		Expect(api.queryInput.Limit).To(BeNil())
		Expect(*api.queryInput.ScanIndexForward).To(BeFalse())
		Expect(api.queryInput.ExclusiveStartKey).To(Equal(map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: "last"}}))
	})

	It("decodes a continuation cursor and preserves status and limit", func() {
		startKey := map[string]types.AttributeValue{
			"adId":           &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000901"},
			"createdAtEpoch": &types.AttributeValueMemberN{Value: "1000"},
		}
		cursor, err := newCursorSigner(platformconfig.AdsConfig{CursorKey: "test-cursor-key", CursorKeyID: "v1", CursorVersion: 1}).encode(ad.StatusPaused, 4, "", startKey)
		Expect(err).NotTo(HaveOccurred())
		api := newPutItemAPIQuery(&dynamodb.QueryOutput{}, nil, nil)

		_, err = (&Repository{Client: api, TableName: "ads-test", cursor: newCursorSigner(platformconfig.AdsConfig{CursorKey: "test-cursor-key", CursorKeyID: "v1", CursorVersion: 1})}).List(context.Background(), ad.ListQuery{Status: ad.StatusPaused, Limit: 4, Cursor: cursor})
		Expect(err).NotTo(HaveOccurred())
		Expect(api.queryInput.ExclusiveStartKey).To(BeNil())
		Expect(api.queryInput.ExpressionAttributeValues[":status"]).To(Equal(&types.AttributeValueMemberS{Value: "paused"}))
	})

	It("rejects incompatible cursors and maps query failures", func() {
		key := map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: "start"}}
		cursor, err := newCursorSigner(platformconfig.AdsConfig{CursorKey: "test-cursor-key", CursorKeyID: "v1", CursorVersion: 1}).encode(ad.StatusActive, 2, "", key)
		Expect(err).NotTo(HaveOccurred())

		_, err = (&Repository{Client: newPutItemAPIQuery(nil, nil, nil), TableName: "ads-test"}).List(context.Background(), ad.ListQuery{Status: ad.StatusActive, Limit: 3, Cursor: cursor})
		Expect(err).To(MatchError(ad.ErrInvalidCursor))

		_, err = (&Repository{Client: newPutItemAPIQuery(nil, nil, errors.New("query failed")), TableName: "ads-test"}).List(context.Background(), ad.ListQuery{Status: ad.StatusActive, Limit: 2})
		Expect(err).To(MatchError(ad.ErrDependencyUnavailable))
	})

	It("maps malformed persistence records to a dependency error", func() {
		api := newPutItemAPIQuery(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{{"adId": &types.AttributeValueMemberS{Value: "only-id"}}}}, nil, nil)
		_, err := (&Repository{Client: api, TableName: "ads-test"}).List(context.Background(), ad.ListQuery{Status: ad.StatusActive, Limit: 1})
		Expect(err).To(MatchError(ad.ErrDependencyUnavailable))
	})
})

var _ = Describe("cursor encoding", func() {
	It("rejects unsupported DynamoDB key types", func() {
		_, err := newCursorSigner(platformconfig.AdsConfig{CursorKey: "test-cursor-key", CursorKeyID: "v1", CursorVersion: 1}).encode(ad.StatusActive, 1, "", map[string]types.AttributeValue{"key": &types.AttributeValueMemberBOOL{Value: true}})
		Expect(err).To(MatchError(ad.ErrInvalidCursor))
	})

	It("requires a non-empty key and matching query parameters", func() {
		_, err := newCursorSigner(platformconfig.AdsConfig{CursorKey: "test-cursor-key", CursorKeyID: "v1", CursorVersion: 1}).decode("e30", ad.ListQuery{Status: ad.StatusActive, Limit: 1})
		Expect(err).To(MatchError(ad.ErrInvalidCursor))
	})

	It("uses the configured signer for continuation and rejects tampered, expired, and mismatched cursors", func() {
		issued := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
		cfg := platformconfig.AdsConfig{CursorKey: "configured-test-key", CursorKeyID: "configured", CursorVersion: 7}
		signer := newCursorSigner(cfg)
		signer.now = func() time.Time { return issued }
		key := map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000999"}}
		cursor, err := signer.encode(ad.StatusActive, 3, "0192f8a0-7b00-7000-8000-000000000901", key)
		Expect(err).NotTo(HaveOccurred())

		decoded, err := signer.decode(cursor, ad.ListQuery{Status: ad.StatusActive, Limit: 3, UserID: "0192f8a0-7b00-7000-8000-000000000901"})
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded["adId"]).To(Equal(key["adId"]))

		tampered := cursor[:len(cursor)-1] + "A"
		_, err = signer.decode(tampered, ad.ListQuery{Status: ad.StatusActive, Limit: 3, UserID: "0192f8a0-7b00-7000-8000-000000000901"})
		Expect(err).To(MatchError(ad.ErrInvalidCursor))

		_, err = signer.decode(cursor, ad.ListQuery{Status: ad.StatusPaused, Limit: 3, UserID: "0192f8a0-7b00-7000-8000-000000000901"})
		Expect(err).To(MatchError(ad.ErrInvalidCursor))

		signer.now = func() time.Time { return issued.Add(16 * time.Minute) }
		_, err = signer.decode(cursor, ad.ListQuery{Status: ad.StatusActive, Limit: 3, UserID: "0192f8a0-7b00-7000-8000-000000000901"})
		Expect(err).To(MatchError(ad.ErrCursorExpired))
	})
})

var _ = Describe("query input contract", func() {
	It("uses the configured table name", func() {
		api := newPutItemAPIQuery(&dynamodb.QueryOutput{}, nil, nil)
		_, err := (&Repository{Client: api, TableName: "configured-ads"}).List(context.Background(), ad.ListQuery{Status: ad.StatusDraft, Limit: 1})
		Expect(err).NotTo(HaveOccurred())
		Expect(aws.ToString(api.queryInput.TableName)).To(Equal("configured-ads"))
	})
})

var _ = Describe("configured advertisement repository", func() {
	It("retains configured cursor settings and handles composite continuation keys", func() {
		cfg := platformconfig.AdsConfig{AdsTable: "configured-ads", CursorKey: "configured-key", CursorKeyID: "configured-kid", CursorVersion: 9}
		repository := NewConfiguredRepository(nil, cfg).(*Repository)
		Expect(repository.TableName).To(Equal("configured-ads"))
		Expect(repository.cursor.keyID).To(Equal("configured-kid"))
		Expect(repository.cursor.version).To(Equal(9))

		key := map[string]types.AttributeValue{
			"adId":           &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000999"},
			"createdAtEpoch": &types.AttributeValueMemberN{Value: "1000"},
		}
		cursor, err := repository.cursor.encode(ad.StatusActive, 2, "admin", key)
		Expect(err).NotTo(HaveOccurred())
		decoded, err := repository.cursor.decode(cursor, ad.ListQuery{Status: ad.StatusActive, Limit: 2, UserID: "admin"})
		Expect(err).NotTo(HaveOccurred())
		Expect(decoded["createdAtEpoch"]).To(Equal(key["createdAtEpoch"]))
		Expect(newCursorSigner(platformconfig.AdsConfig{}).version).To(Equal(1))
		_, err = mapListItem(map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: "only-id"}})
		Expect(err).To(HaveOccurred())
		invalid := listItem("0192f8a0-7b00-7000-8000-000000000999", "active", 1000)
		for _, field := range []string{"entityType", "advertiser", "content", "targetUrl", "startsAt", "endsAt", "status", "createdBy", "createdAt", "createdAtEpoch"} {
			copyItem := map[string]types.AttributeValue{}
			for key, value := range invalid {
				copyItem[key] = value
			}
			delete(copyItem, field)
			_, err = mapListItem(copyItem)
			Expect(err).To(HaveOccurred(), field)
		}
	})
})

var _ = Describe("GET advertisement list configured cursor boundary", func() {
	It("uses the configured signer at the HTTP boundary for expiry and user binding", func() {
		issued := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
		cfg := platformconfig.AdsConfig{AdsTable: "configured-ads", CursorKey: "configured-http-key", CursorKeyID: "configured-http", CursorVersion: 11}
		signer := newCursorSigner(cfg)
		signer.now = func() time.Time { return issued }
		key := map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-0000-0000-000000000999"}}
		cursor, err := signer.encode(ad.StatusActive, 2, "0192f8a0-7b00-8000-0000-000000000901", key)
		Expect(err).NotTo(HaveOccurred())

		request := func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/ads?limit=2&cursor="+cursor, nil)
			r.Header.Set("Authorization", "Bearer configured-admin")
			return r
		}
		newRouter := func(authenticator *MockadminAuthenticator, current time.Time) http.Handler {
			configuredSigner := signer
			configuredSigner.now = func() time.Time { return current }
			repository := &Repository{Client: newPutItemAPIQuery(&dynamodb.QueryOutput{}, nil, nil), TableName: cfg.AdsTable, cursor: configuredSigner}
			service := ad.NewService(repository, nil, nil)
			return ad.NewRouter(ad.NewHandler(service), authenticator)
		}

		expired := httptest.NewRecorder()
		expiredAuth := NewMockadminAuthenticator(gomock.NewController(GinkgoT()))
		expiredAuth.EXPECT().AuthenticateAdmin(gomock.Any(), gomock.Any()).Return("0192f8a0-7b00-8000-0000-000000000901", nil)
		newRouter(expiredAuth, issued.Add(16*time.Minute)).ServeHTTP(expired, request())
		Expect(expired.Code).To(Equal(http.StatusBadRequest))
		Expect(expired.Body.String()).To(ContainSubstring(`"code":"CURSOR_EXPIRED"`))

		bound := httptest.NewRecorder()
		boundAuth := NewMockadminAuthenticator(gomock.NewController(GinkgoT()))
		boundAuth.EXPECT().AuthenticateAdmin(gomock.Any(), gomock.Any()).Return("0192f8a0-7b00-8000-0000-000000000902", nil)
		newRouter(boundAuth, issued).ServeHTTP(bound, request())
		Expect(bound.Code).To(Equal(http.StatusBadRequest))
		Expect(bound.Body.String()).To(ContainSubstring(`"code":"INVALID_CURSOR"`))
	})

	It("returns forbidden and invalid-token payloads through the same HTTP boundary", func() {
		for _, test := range []struct {
			authError error
			status    int
			code      string
		}{
			{authError: auth.ErrForbidden, status: http.StatusForbidden, code: "FORBIDDEN"},
			{authError: errors.New("malformed token"), status: http.StatusUnauthorized, code: "INVALID_TOKEN"},
		} {
			repository := &Repository{Client: newPutItemAPIQuery(nil, nil, nil), TableName: "configured-ads", cursor: newCursorSigner(platformconfig.AdsConfig{CursorKey: "key", CursorKeyID: "kid", CursorVersion: 1})}
			authenticator := NewMockadminAuthenticator(gomock.NewController(GinkgoT()))
			authenticator.EXPECT().AuthenticateAdmin(gomock.Any(), gomock.Any()).Return("", test.authError)
			router := ad.NewRouter(ad.NewHandler(ad.NewService(repository, nil, nil)), authenticator)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/ads", nil)
			request.Header.Set("Authorization", "Bearer token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			Expect(response.Code).To(Equal(test.status))
			Expect(response.Body.String()).To(ContainSubstring(`"code":"` + test.code + `"`))
		}
	})
})
