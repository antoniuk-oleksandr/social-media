package fixtures

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/user"
)

var _ = Describe("RegistrationBuildsDeterministicFixture", func() {
	It("runs the test", func() {
		got := Registration("user-id", "alice", "alice@example.com")
		if got.User.ID != "user-id" || got.User.Username != "alice" || got.User.Email != "alice@example.com" {
			Expect(false).To(BeTrue(), fmt.Sprintf("user = %#v", got.User))
		}
		if got.User.Role != user.RoleUser || got.User.Status != user.StatusActive {
			Expect(false).To(BeTrue(), fmt.Sprintf("user role/status = %s/%s", got.User.Role, got.User.Status))
		}
		created := time.Date(2026, time.October, 22, 22, 0, 0, 0, time.UTC)
		if !got.User.CreatedAt.Equal(created) || !got.Refresh.CreatedAt.Equal(created) || !got.Refresh.ExpiresAt.Equal(created.Add(7*24*time.Hour)) {
			Expect(false).To(BeTrue(), fmt.Sprintf("timestamps = %#v", got))
		}
		if got.Refresh.TokenHash != "refresh-hash-user-id" || got.Refresh.SessionID != "session-user-id" || got.Refresh.UserID != "user-id" {
			Expect(false).To(BeTrue(), fmt.Sprintf("refresh = %#v", got.Refresh))
		}

	})
})
