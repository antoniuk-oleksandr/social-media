package dynamodb

import (
	"fmt"
	"time"

	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	"github.com/antoniuk-oleksandr/social-media/internal/post"
)

func toRecord(value post.Post) PostRecord {
	createdAt := value.CreatedAt.UTC().Truncate(time.Millisecond)
	createdAtEpoch := value.CreatedAtEpoch
	if !value.CreatedAt.IsZero() {
		createdAtEpoch = createdAt.UnixMilli()
	}
	return PostRecord{ID: value.ID, EntityType: "post", AuthorID: value.AuthorID, Content: value.Content, Status: "active", CreatedAt: createdAt.Format(time.RFC3339Nano), CreatedAtEpoch: createdAtEpoch, FeedPartition: "GLOBAL"}
}

func toDomain(record PostRecord, requestedID string) (post.Post, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, record.CreatedAt)
	content, contentErr := post.NormalizeContent(record.Content)
	if err != nil ||
		contentErr != nil ||
		record.CreatedAt != createdAt.UTC().Format(time.RFC3339Nano) ||
		createdAt.Nanosecond()%int(time.Millisecond) != 0 ||
		record.EntityType != "post" ||
		record.ID != requestedID ||
		!postIDIsValid(record.ID) ||
		!postIDIsValid(record.AuthorID) ||
		record.CreatedAtEpoch <= 0 ||
		createdAt.UnixMilli() != record.CreatedAtEpoch {
		return post.Post{}, fmt.Errorf("invalid post record")
	}
	return post.Post{ID: record.ID, AuthorID: record.AuthorID, Content: content, CreatedAt: createdAt, CreatedAtEpoch: record.CreatedAtEpoch}, nil
}

func postIDIsValid(value string) bool {
	return platformid.IsCanonicalUUIDv7(value)
}
