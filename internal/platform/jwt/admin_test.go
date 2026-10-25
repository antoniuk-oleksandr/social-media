package jwt

import (
	"context"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AuthenticateAdmin", func() {
	It("accepts an admin token and rejects a user token", func() {
		access := Access{Keys: StaticKey{KID: "key", Secret: []byte("secret")}, KID: "key", Issuer: "issuer", Audience: "audience"}
		adminID := "0192f8a0-7b00-7000-8000-000000000301"
		adminToken, err := access.Issue(context.Background(), user.User{ID: adminID, Role: user.Role("admin")}, time.Now())
		Expect(err).NotTo(HaveOccurred())
		Expect(access.AuthenticateAdmin(context.Background(), adminToken)).To(Equal(adminID))

		userToken, err := access.Issue(context.Background(), user.User{ID: "0192f8a0-7b00-7000-8000-000000000302", Role: user.RoleUser}, time.Now())
		Expect(err).NotTo(HaveOccurred())
		_, err = access.AuthenticateAdmin(context.Background(), userToken)
		Expect(err).To(MatchError("forbidden: administrator role required"))
	})
})
