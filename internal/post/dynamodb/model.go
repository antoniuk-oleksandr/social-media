package dynamodb

type PostRecord struct {
	ID             string `dynamodbav:"postId"`
	EntityType     string `dynamodbav:"entityType"`
	AuthorID       string `dynamodbav:"authorId"`
	Content        string `dynamodbav:"content"`
	Status         string `dynamodbav:"status"`
	CreatedAt      string `dynamodbav:"createdAt"`
	CreatedAtEpoch int64  `dynamodbav:"createdAtEpoch"`
	FeedPartition  string `dynamodbav:"feedPartition"`
}
