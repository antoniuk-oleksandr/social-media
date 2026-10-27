//go:build integration

package feedintegration

import (
	"context"
	"fmt"
	"testing"
	"time"

	addb "github.com/antoniuk-oleksandr/social-media/internal/ad/dynamodb"
	feedfeature "github.com/antoniuk-oleksandr/social-media/internal/feed"
	feeddb "github.com/antoniuk-oleksandr/social-media/internal/feed/dynamodb"
	userdb "github.com/antoniuk-oleksandr/social-media/internal/user/dynamodb"
	"github.com/antoniuk-oleksandr/social-media/tests/integration/support"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var feedSuite *support.DynamoDBSuite
var feedPostsTable string
var feedAdsTable string
var feedUsersTable string

type integrationClock struct{ now time.Time }

func (c integrationClock) Now() time.Time { return c.now }

func TestFeedIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Feed integration suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var err error
	feedSuite, err = support.StartDynamoDB(ctx, "tests-feed")
	Expect(err).NotTo(HaveOccurred())
	feedPostsTable = fmt.Sprintf("tests-feed-posts-%d", time.Now().UTC().UnixNano())
	feedAdsTable = fmt.Sprintf("tests-feed-ads-%d", time.Now().UTC().UnixNano())
	feedUsersTable = fmt.Sprintf("tests-feed-users-%d", time.Now().UTC().UnixNano())
	Expect(createFeedPostsTable(ctx)).To(Succeed())
	Expect(createFeedAdsTable(ctx)).To(Succeed())
	Expect(createFeedUsersTable(ctx)).To(Succeed())
})

var _ = AfterSuite(func() {
	if feedSuite != nil {
		Expect(feedSuite.Close(context.Background())).To(Succeed())
	}
})

var _ = BeforeEach(func() {
	ctx := context.Background()
	Expect(clearFeedTable(ctx, feedPostsTable, "postId")).To(Succeed())
	Expect(clearFeedTable(ctx, feedAdsTable, "adId")).To(Succeed())
	Expect(clearFeedTable(ctx, feedUsersTable, "userId")).To(Succeed())
})

var _ = Describe("feed DynamoDB persistence", func() {
	It("rejects a continuation cursor for a different authenticated user", func() {
		ctx := context.Background()
		now := time.Date(2026, 10, 26, 14, 0, 0, 0, time.UTC)
		for i := 0; i < 21; i++ {
			putPost(ctx, 6000+i, now.Add(-time.Duration(i)*time.Minute))
		}

		service := feedfeature.NewService(
			feeddb.NewPostRepository(feedSuite.Client, feedPostsTable),
			feeddb.NewAdRepository(addb.NewRepository(feedSuite.Client, feedAdsTable)),
			integrationClock{now: now}, "integration-key", "v1",
		)
		first, err := service.Build(ctx, feedfeature.Query{Limit: 20, UserID: "user-a"})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.NextCursor).NotTo(BeEmpty())

		_, err = service.Build(ctx, feedfeature.Query{Limit: 20, UserID: "user-b", Cursor: first.NextCursor})
		Expect(err).To(MatchError(feedfeature.ErrInvalidCursor))
	})

	It("does not include a post added after the first page snapshot", func() {
		ctx := context.Background()
		now := time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)
		for i := 0; i < 21; i++ {
			putPost(ctx, 7000+i, now.Add(-time.Duration(i)*time.Minute))
		}

		service := feedfeature.NewService(
			feeddb.NewPostRepository(feedSuite.Client, feedPostsTable),
			feeddb.NewAdRepository(addb.NewRepository(feedSuite.Client, feedAdsTable)),
			integrationClock{now: now}, "integration-key", "v1",
		)
		first, err := service.Build(ctx, feedfeature.Query{Limit: 20, UserID: "user-a"})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.HasMore).To(BeTrue())

		putPost(ctx, 7999, now.Add(time.Minute))
		continuation, err := service.Build(ctx, feedfeature.Query{Limit: 20, UserID: "user-a", Cursor: first.NextCursor})
		Expect(err).NotTo(HaveOccurred())
		Expect(continuation.Items).To(HaveLen(1))
		Expect(continuation.Items[0].Post.ID).To(Equal(fmt.Sprintf("0192f8a0-7b00-7000-8000-%012d", 8000+20)))
		for _, item := range continuation.Items {
			if item.Post != nil {
				Expect(item.Post.ID).NotTo(Equal(fmt.Sprintf("0192f8a0-7b00-7000-8000-%012d", 8999)))
			}
		}
	})

	It("reads 45 posts in deterministic pages and inserts eligible ads without duplicates", func() {
		ctx := context.Background()
		now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
		for i := 0; i < 45; i++ {
			created := now.Add(-time.Duration(i) * time.Minute)
			putPost(ctx, i, created)
		}
		for i := 0; i < 51; i++ {
			putAd(ctx, fmt.Sprintf("0192f8a0-7b00-7000-8000-%012d", i+901), now)
		}

		posts := feeddb.NewPostRepository(feedSuite.Client, feedPostsTable)
		ads := feeddb.NewAdRepository(addb.NewRepository(feedSuite.Client, feedAdsTable))
		service := feedfeature.NewService(posts, ads, integrationClock{now: now}, "integration-key", "v1")
		var cursor string
		seen := map[string]bool{}
		seenAds := map[string]bool{}
		adCount := 0
		for page := 0; page < 3; page++ {
			result, err := service.Build(ctx, feedfeature.Query{Limit: 20, Cursor: cursor})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.HasMore).To(Equal(page < 2))
			Expect(result.Items).To(HaveLen([]int{21, 21, 6}[page]))
			for _, item := range result.Items {
				if item.Post != nil {
					Expect(seen[item.Post.ID]).To(BeFalse(), "duplicate post %s", item.Post.ID)
					seen[item.Post.ID] = true
				}
				if item.Ad != nil {
					Expect(seenAds[item.Ad.ID]).To(BeFalse(), "duplicate ad %s", item.Ad.ID)
					seenAds[item.Ad.ID] = true
					adCount++
				}
			}
			cursor = result.NextCursor
		}
		Expect(seen).To(HaveLen(45))
		Expect(adCount).To(Equal(3))
	})

	It("includes an eligible ad on a non-empty incomplete final page", func() {
		ctx := context.Background()
		now := time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC)
		putPost(ctx, 5000, now)
		putPost(ctx, 5001, now.Add(-time.Minute))
		putAd(ctx, "0192f8a0-7b00-7000-8000-000000009999", now)

		service := feedfeature.NewService(
			feeddb.NewPostRepository(feedSuite.Client, feedPostsTable),
			feeddb.NewAdRepository(addb.NewRepository(feedSuite.Client, feedAdsTable)),
			integrationClock{now: now}, "integration-key", "v1",
		)
		result, err := service.Build(ctx, feedfeature.Query{Limit: 20})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Items).To(HaveLen(3))
		Expect(result.Items[2].Ad).NotTo(BeNil())
		Expect(result.HasMore).To(BeFalse())
	})

	It("loads post authors through the user repository batch lookup", func() {
		ctx := context.Background()
		now := time.Date(2026, 10, 26, 16, 0, 0, 0, time.UTC)
		firstAuthor := "0192f8a0-7b00-7000-8000-000000000101"
		secondAuthor := "0192f8a0-7b00-7000-8000-000000000102"
		putUser(ctx, firstAuthor, "alice", "Alice")
		putUser(ctx, secondAuthor, "bob", "Bob")
		putPostWithAuthor(ctx, 8100, firstAuthor, now)
		putPostWithAuthor(ctx, 8101, secondAuthor, now.Add(-time.Minute))

		service := feedfeature.NewService(
			feeddb.NewPostRepository(feedSuite.Client, feedPostsTable),
			feeddb.NewAdRepository(addb.NewRepository(feedSuite.Client, feedAdsTable)),
			integrationClock{now: now}, "integration-key", "v1",
			userdb.NewConfiguredRepository(feedSuite.Client, feedUsersTable, "unused-uniques-table"),
		)

		result, err := service.Build(ctx, feedfeature.Query{})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Items).To(HaveLen(2))
		Expect(result.Items[0].Author).NotTo(BeNil())
		Expect(result.Items[0].Author.Username).To(Equal("alice"))
		Expect(result.Items[1].Author).NotTo(BeNil())
		Expect(result.Items[1].Author.Username).To(Equal("bob"))
	})

	It("translates a DynamoDB dependency failure to the feed domain error", func() {
		service := feedfeature.NewService(feeddb.NewPostRepository(feedSuite.Client, "missing-feed-table"), feeddb.NewAdRepository(addb.NewRepository(feedSuite.Client, feedAdsTable)), integrationClock{now: time.Now().UTC()}, "key", "v1")
		_, err := service.Build(context.Background(), feedfeature.Query{})
		Expect(err).To(MatchError(feedfeature.ErrDependencyUnavailable))
	})
})

func createFeedPostsTable(ctx context.Context) error {
	_, err := feedSuite.Client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(feedPostsTable),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("postId"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("feedPartition"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("createdAtEpoch"), AttributeType: types.ScalarAttributeTypeN},
		},
		KeySchema:   []types.KeySchemaElement{{AttributeName: aws.String("postId"), KeyType: types.KeyTypeHash}},
		BillingMode: types.BillingModePayPerRequest,
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{
			IndexName: aws.String("FeedPostsIndex"),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("feedPartition"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("createdAtEpoch"), KeyType: types.KeyTypeRange},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		}},
	})
	return err
}

func createFeedAdsTable(ctx context.Context) error {
	_, err := feedSuite.Client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(feedAdsTable),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("adId"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("status"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("createdAtEpoch"), AttributeType: types.ScalarAttributeTypeN},
		},
		KeySchema:   []types.KeySchemaElement{{AttributeName: aws.String("adId"), KeyType: types.KeyTypeHash}},
		BillingMode: types.BillingModePayPerRequest,
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{
			IndexName: aws.String("ActiveAdsIndex"),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("status"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("createdAtEpoch"), KeyType: types.KeyTypeRange},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		}},
	})
	return err
}

func createFeedUsersTable(ctx context.Context) error {
	_, err := feedSuite.Client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:            aws.String(feedUsersTable),
		AttributeDefinitions: []types.AttributeDefinition{{AttributeName: aws.String("userId"), AttributeType: types.ScalarAttributeTypeS}},
		KeySchema:            []types.KeySchemaElement{{AttributeName: aws.String("userId"), KeyType: types.KeyTypeHash}},
		BillingMode:          types.BillingModePayPerRequest,
	})
	return err
}

func clearFeedTable(ctx context.Context, table, key string) error {
	output, err := feedSuite.Client.Scan(ctx, &dynamodb.ScanInput{
		TableName:            aws.String(table),
		ProjectionExpression: aws.String(key),
	})
	if err != nil {
		return err
	}
	for _, item := range output.Items {
		_, err = feedSuite.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(table),
			Key:       map[string]types.AttributeValue{key: item[key]},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func putPost(ctx context.Context, index int, created time.Time) {
	putPostWithAuthor(ctx, index, "0192f8a0-7b00-7000-8000-000000000999", created)
}

func putPostWithAuthor(ctx context.Context, index int, authorID string, created time.Time) {
	id := fmt.Sprintf("0192f8a0-7b00-7000-8000-%012d", index+1000)
	_, err := feedSuite.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(feedPostsTable), Item: map[string]types.AttributeValue{
		"postId": &types.AttributeValueMemberS{Value: id}, "entityType": &types.AttributeValueMemberS{Value: "post"}, "authorId": &types.AttributeValueMemberS{Value: authorID}, "content": &types.AttributeValueMemberS{Value: fmt.Sprintf("post-%d", index)}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: created.Format(time.RFC3339Nano)}, "createdAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(created.UnixMilli())}, "feedPartition": &types.AttributeValueMemberS{Value: "GLOBAL"},
	}})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

func putUser(ctx context.Context, id, username, displayName string) {
	_, err := feedSuite.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(feedUsersTable), Item: map[string]types.AttributeValue{
		"userId": &types.AttributeValueMemberS{Value: id}, "username": &types.AttributeValueMemberS{Value: username}, "email": &types.AttributeValueMemberS{Value: username + "@example.com"}, "displayName": &types.AttributeValueMemberS{Value: displayName}, "role": &types.AttributeValueMemberS{Value: "user"}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberS{Value: "2026-10-26T00:00:00Z"},
	}})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

func putAd(ctx context.Context, id string, now time.Time) {
	_, err := feedSuite.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(feedAdsTable), Item: map[string]types.AttributeValue{
		"adId": &types.AttributeValueMemberS{Value: id}, "entityType": &types.AttributeValueMemberS{Value: "ad"}, "advertiser": &types.AttributeValueMemberS{Value: "Example"}, "content": &types.AttributeValueMemberS{Value: "Try this"}, "targetUrl": &types.AttributeValueMemberS{Value: "https://example.com"}, "startsAt": &types.AttributeValueMemberS{Value: now.Add(-time.Hour).Format(time.RFC3339Nano)}, "endsAt": &types.AttributeValueMemberS{Value: now.Add(time.Hour).Format(time.RFC3339Nano)}, "startsAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(now.Add(-time.Hour).UnixMilli())}, "endsAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(now.Add(time.Hour).UnixMilli())}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdBy": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000998"}, "createdAt": &types.AttributeValueMemberS{Value: now.Format(time.RFC3339Nano)}, "createdAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(now.UnixMilli())},
	}})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}
