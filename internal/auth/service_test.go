package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"go.uber.org/mock/gomock"
)

const serviceUserID = "0192f8a0-7b00-7000-8000-000000000001"

func registrationDependencies(t *testing.T, id string, idErr error) (*MockRegistrationStore, *MockPasswordHasher, *MockIDGenerator, *MockClock, *MockAccessTokenIssuer, *MockOpaqueTokenGenerator, *auth.Registration) {
	ctrl := gomock.NewController(t)
	store := NewMockRegistrationStore(ctrl)
	hasher := NewMockPasswordHasher(ctrl)
	ids := NewMockIDGenerator(ctrl)
	clock := NewMockClock(ctrl)
	access := NewMockAccessTokenIssuer(ctrl)
	opaque := NewMockOpaqueTokenGenerator(ctrl)
	var persisted auth.Registration
	store.EXPECT().Register(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, value auth.Registration) error { persisted = value; return nil })
	hasher.EXPECT().Hash(gomock.Any()).AnyTimes().Return("hash", nil)
	ids.EXPECT().New().AnyTimes().Return(id, idErr)
	clock.EXPECT().Now().AnyTimes().Return(time.Unix(100, 0))
	access.EXPECT().Issue(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return("access", nil)
	opaque.EXPECT().Generate().AnyTimes().Return("refresh", "hash", nil)
	return store, hasher, ids, clock, access, opaque, &persisted
}

func validRegistration() auth.RegisterCommand {
	return auth.RegisterCommand{Username: "alice", Email: " A@Example.COM ", Password: "123456789012", DisplayName: "Alice"}
}

func TestRegisterNormalizesAndPersistsAtomicPayload(t *testing.T) {
	store, hasher, ids, clock, access, opaque, persisted := registrationDependencies(t, serviceUserID, nil)
	got, err := (&auth.Service{Store: store, Hasher: hasher, IDs: ids, Clock: clock, Access: access, Opaque: opaque, AccessTTL: 15 * time.Minute, RefreshTTL: 7 * 24 * time.Hour}).Register(context.Background(), validRegistration())
	if err != nil || got.User.Email != "a@example.com" || got.User.Role != user.RoleUser || persisted.User.PasswordHash != "hash" || persisted.Refresh.TokenHash != "hash" || got.Tokens.ExpiresIn != 900 || got.Tokens.RefreshExpiresIn != 604800 {
		t.Fatalf("result=%+v persisted=%+v err=%v", got, persisted, err)
	}
}

func TestRegisterPreservesCanonicalUUIDv7(t *testing.T) {
	store, hasher, ids, clock, access, opaque, persisted := registrationDependencies(t, serviceUserID, nil)
	_, err := (&auth.Service{Store: store, Hasher: hasher, IDs: ids, Clock: clock, Access: access, Opaque: opaque}).Register(context.Background(), validRegistration())
	if err != nil || persisted.User.ID != serviceUserID || persisted.Refresh.UserID != serviceUserID {
		t.Fatalf("persisted=%+v err=%v", persisted, err)
	}
}

func TestRegisterValidationPreventsPersistence(t *testing.T) {
	store, hasher, ids, clock, access, opaque, persisted := registrationDependencies(t, serviceUserID, nil)
	_, err := (&auth.Service{Store: store, Hasher: hasher, IDs: ids, Clock: clock, Access: access, Opaque: opaque}).Register(context.Background(), auth.RegisterCommand{Username: "AB", Email: "bad", Password: "short"})
	if !errors.Is(err, auth.ErrValidation) || persisted.User.ID != "" {
		t.Fatalf("persisted=%+v err=%v", persisted, err)
	}
}
