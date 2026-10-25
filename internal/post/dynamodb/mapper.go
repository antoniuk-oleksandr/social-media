package dynamodb

import (
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/post"
)

func toRecord(value post.Post) PostRecord {
	return PostRecord{ID: value.ID, EntityType: "post", AuthorID: value.AuthorID, Content: value.Content, Status: "active", CreatedAt: value.CreatedAt.UTC().Format(time.RFC3339), CreatedAtEpoch: value.CreatedAtEpoch, FeedPartition: "GLOBAL"}
}
