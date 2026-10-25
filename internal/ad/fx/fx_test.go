package fx

import (
	"testing"

	addb "github.com/antoniuk-oleksandr/social-media/internal/ad/dynamodb"
	platformclock "github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	platformjwt "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func TestConstructorsWireAdvertisementFeature(t *testing.T) {
	repository := NewRepository(&dynamodb.Client{}, platformconfig.AdsConfig{AdsTable: "ads"})
	if _, ok := repository.(*addb.Repository); !ok {
		t.Fatalf("repository type = %T", repository)
	}
	service := NewService(repository, platformid.UUIDv7{}, platformclock.Real{})
	handler := NewHandler(service, nil)
	if handler == nil || NewRouter(handler, platformjwt.Access{}) == nil {
		t.Fatal("advertisement constructors returned nil")
	}
}
