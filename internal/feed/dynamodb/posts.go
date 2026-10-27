package dynamodb

import (
	"context"
	"errors"

	feed "github.com/antoniuk-oleksandr/social-media/internal/feed"
	"github.com/antoniuk-oleksandr/social-media/internal/post"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"sort"
	"strconv"
	"time"
)

type PostRepository struct {
	Client interface {
		Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	}
	TableName string
}

func NewPostRepository(client *dynamodb.Client, table string) feed.PostSource {
	return &PostRepository{Client: client, TableName: table}
}
func (r *PostRepository) List(ctx context.Context, snapshot time.Time) ([]post.Post, error) {
	var out []post.Post
	var key map[string]types.AttributeValue
	for {
		v, e := r.Client.Query(ctx, &dynamodb.QueryInput{TableName: aws.String(r.TableName), IndexName: aws.String("FeedPostsIndex"), KeyConditionExpression: aws.String("feedPartition = :partition"), ExpressionAttributeValues: map[string]types.AttributeValue{":partition": &types.AttributeValueMemberS{Value: "GLOBAL"}}, ScanIndexForward: aws.Bool(false), ExclusiveStartKey: key})
		if e != nil {
			return nil, e
		}
		for _, item := range v.Items {
			p, e := mapPost(item)
			if e != nil && e.Error() == "inactive post" {
				continue
			}
			if e != nil {
				return nil, e
			}
			if p.CreatedAt.Before(snapshot) || p.CreatedAt.Equal(snapshot) {
				out = append(out, p)
			}
		}
		if len(v.LastEvaluatedKey) == 0 {
			break
		}
		key = v.LastEvaluatedKey
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAtEpoch != out[j].CreatedAtEpoch {
			return out[i].CreatedAtEpoch > out[j].CreatedAtEpoch
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}
func mapPost(i map[string]types.AttributeValue) (post.Post, error) {
	s := func(k string) (string, error) {
		v, ok := i[k].(*types.AttributeValueMemberS)
		if !ok {
			return "", errors.New("invalid post")
		}
		return v.Value, nil
	}
	n := func(k string) (int64, error) {
		v, ok := i[k].(*types.AttributeValueMemberN)
		if !ok {
			return 0, errors.New("invalid post")
		}
		return strconv.ParseInt(v.Value, 10, 64)
	}
	id, e := s("postId")
	if e != nil {
		return post.Post{}, e
	}
	author, e := s("authorId")
	if e != nil {
		return post.Post{}, e
	}
	content, e := s("content")
	if e != nil {
		return post.Post{}, e
	}
	status, e := s("status")
	if e != nil || status != "active" {
		return post.Post{}, errors.New("inactive post")
	}
	created, e := s("createdAt")
	if e != nil {
		return post.Post{}, e
	}
	epoch, e := n("createdAtEpoch")
	if e != nil {
		return post.Post{}, e
	}
	t, e := time.Parse(time.RFC3339Nano, created)
	if e != nil {
		return post.Post{}, e
	}
	return post.Post{ID: id, AuthorID: author, Content: content, CreatedAt: t, CreatedAtEpoch: epoch}, nil
}
