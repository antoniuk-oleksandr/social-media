package auth_test

import (
	"context"
	"errors"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type loginFailureID struct{}

func (loginFailureID) New() (string, error) { return "", errors.New("id failure") }

var _ = Describe("LoginServiceInfrastructureFailures", func() {
	active := user.User{ID: "user-id", PasswordHash: "hash", Status: user.StatusActive}
	It("translates dependency failures", func() {
		for _, tc := range []struct {
			name string
			make func(*loginStore) *auth.Service
		}{
			{name: "missing login store", make: func(*loginStore) *auth.Service { return &auth.Service{Verifier: loginVerifier{}} }},
			{name: "missing verifier", make: func(store *loginStore) *auth.Service { return &auth.Service{LoginStore: store} }},
			{name: "id generation", make: func(store *loginStore) *auth.Service {
				return &auth.Service{LoginStore: store, Verifier: loginVerifier{}, IDs: loginFailureID{}, Clock: loginClock{}}
			}},
			{name: "access issuance", make: func(store *loginStore) *auth.Service {
				return &auth.Service{LoginStore: store, Verifier: loginVerifier{}, IDs: loginIDs{}, Clock: loginClock{}, Access: failingAccess{}}
			}},
			{name: "opaque generation", make: func(store *loginStore) *auth.Service {
				return &auth.Service{LoginStore: store, Verifier: loginVerifier{}, IDs: loginIDs{}, Clock: loginClock{}, Access: loginAccess{}, Opaque: failingOpaque{}}
			}},
			{name: "session persistence", make: func(store *loginStore) *auth.Service {
				store.sessionErr = errors.New("session failure")
				return newLoginService(store, nil)
			}},
		} {
			By(tc.name)
			store := &loginStore{user: active}
			service := tc.make(store)
			_, err := service.Login(context.Background(), auth.LoginCommand{Login: "alice", Password: "correct-password"})
			Expect(err).To(HaveOccurred())
		}
	})
})
