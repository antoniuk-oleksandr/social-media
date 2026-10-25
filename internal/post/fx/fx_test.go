package fx

import (
	"testing"

	platformclock "github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	platformjwt "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	postdb "github.com/antoniuk-oleksandr/social-media/internal/post/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func TestConstructorsWirePostFeature(t *testing.T) {
	repository := NewRepository(&dynamodb.Client{}, platformconfig.PostsConfig{PostsTable: "posts"})
	if _, ok := repository.(*postdb.Repository); !ok {
		t.Fatalf("repository type = %T", repository)
	}
	service := NewService(repository, platformid.UUIDv7{}, platformclock.Real{})
	handler := NewHandler(service, nil)
	if handler == nil || NewRouter(handler, platformjwt.Access{}) == nil {
		t.Fatal("post constructors returned nil")
	}
}
