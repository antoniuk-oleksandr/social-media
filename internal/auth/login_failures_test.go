package auth_test

import (
	"context"
	"errors"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

var _ = Describe("LoginServiceInfrastructureFailures", func() {
	active := user.User{ID: "0192f8a0-7b00-7000-8000-000000000001", PasswordHash: "hash", Status: user.StatusActive}
	It("translates dependency failures", func() {
		for _, tc := range []struct {
			name string
			make func(*loginStoreFixture) *auth.Service
		}{
			{name: "missing login store", make: func(*loginStoreFixture) *auth.Service { return &auth.Service{} }},
			{name: "missing verifier", make: func(store *loginStoreFixture) *auth.Service { return &auth.Service{LoginStore: store} }},
			{name: "id generation", make: func(store *loginStoreFixture) *auth.Service {
				ids := NewMockIDGenerator(gomock.NewController(GinkgoT()))
				ids.EXPECT().New().Return("", errors.New("id failure"))
				verifier := NewMockPasswordVerifier(gomock.NewController(GinkgoT()))
				verifier.EXPECT().Compare(gomock.Any(), gomock.Any()).Return(nil)
				clock := NewMockClock(gomock.NewController(GinkgoT()))
				clock.EXPECT().Now().AnyTimes().Return(time.Unix(100, 0).UTC())
				return &auth.Service{LoginStore: store, Verifier: verifier, IDs: ids, Clock: clock}
			}},
			{name: "access issuance", make: func(store *loginStoreFixture) *auth.Service {
				access := NewMockAccessTokenIssuer(gomock.NewController(GinkgoT()))
				access.EXPECT().Issue(gomock.Any(), gomock.Any(), gomock.Any()).Return("", errors.New("token failure"))
				verifier := NewMockPasswordVerifier(gomock.NewController(GinkgoT()))
				verifier.EXPECT().Compare(gomock.Any(), gomock.Any()).Return(nil)
				ids := NewMockIDGenerator(gomock.NewController(GinkgoT()))
				ids.EXPECT().New().Return("0192f8a0-7b00-7000-8000-000000000002", nil)
				clock := NewMockClock(gomock.NewController(GinkgoT()))
				clock.EXPECT().Now().AnyTimes().Return(time.Unix(100, 0).UTC())
				return &auth.Service{LoginStore: store, Verifier: verifier, IDs: ids, Clock: clock, Access: access}
			}},
			{name: "opaque generation", make: func(store *loginStoreFixture) *auth.Service {
				opaque := NewMockOpaqueTokenGenerator(gomock.NewController(GinkgoT()))
				opaque.EXPECT().Generate().Return("", "", errors.New("random failure"))
				verifier := NewMockPasswordVerifier(gomock.NewController(GinkgoT()))
				verifier.EXPECT().Compare(gomock.Any(), gomock.Any()).Return(nil)
				ids := NewMockIDGenerator(gomock.NewController(GinkgoT()))
				ids.EXPECT().New().Return("0192f8a0-7b00-7000-8000-000000000002", nil)
				clock := NewMockClock(gomock.NewController(GinkgoT()))
				clock.EXPECT().Now().AnyTimes().Return(time.Unix(100, 0).UTC())
				access := NewMockAccessTokenIssuer(gomock.NewController(GinkgoT()))
				access.EXPECT().Issue(gomock.Any(), gomock.Any(), gomock.Any()).Return("access", nil)
				return &auth.Service{LoginStore: store, Verifier: verifier, IDs: ids, Clock: clock, Access: access, Opaque: opaque}
			}},
			{name: "session persistence", make: func(store *loginStoreFixture) *auth.Service {
				return newLoginService(newLoginStoreWithSessionErr(active, nil, errors.New("session failure")), nil)
			}},
		} {
			By(tc.name)
			store := newLoginStore(active, nil)
			service := tc.make(store)
			_, err := service.Login(context.Background(), auth.LoginCommand{Login: "alice", Password: "correct-password"})
			Expect(err).To(HaveOccurred())
		}
	})
})
