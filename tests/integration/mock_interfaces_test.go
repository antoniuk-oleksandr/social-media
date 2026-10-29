//go:build integration

package integration

import (
	"context"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	"go.uber.org/mock/gomock"
)

type integrationIDGenerator interface {
	New() (string, error)
}

type integrationUserLookup interface {
	FindByID(context.Context, string) (user.User, error)
}

func newIntegrationID(value string) *MockintegrationIDGenerator {
	mock := NewMockintegrationIDGenerator(gomock.NewController(GinkgoT()))
	if !platformid.IsCanonicalUUIDv7(value) {
		value = "0192f8a0-7b00-7000-8000-000000000020"
	}
	mock.EXPECT().New().Return(value, nil).AnyTimes()
	return mock
}

func newIntegrationUserLookup() *MockintegrationUserLookup {
	mock := NewMockintegrationUserLookup(gomock.NewController(GinkgoT()))
	mock.EXPECT().FindByID(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, id string) (user.User, error) {
		return user.User{ID: id, Role: user.RoleUser, Status: user.StatusActive}, nil
	})
	return mock
}

func fixedIntegrationClock(value time.Time) clock.Fixed {
	return clock.Fixed{T: value}
}
