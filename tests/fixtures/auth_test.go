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
		if got.User.ID == "" || got.User.Username != "alice" || got.User.Email != "alice@example.com" {
			Expect(false).To(BeTrue(), fmt.Sprintf("user = %#v", got.User))
		}
		if got.User.Role != user.RoleUser || got.User.Status != user.StatusActive {
			Expect(false).To(BeTrue(), fmt.Sprintf("user role/status = %s/%s", got.User.Role, got.User.Status))
		}
		created := time.Date(2026, time.October, 22, 22, 0, 0, 0, time.UTC)
		if !got.User.CreatedAt.Equal(created) || !got.Refresh.CreatedAt.Equal(created) || !got.Refresh.ExpiresAt.Equal(created.Add(7*24*time.Hour)) {
			Expect(false).To(BeTrue(), fmt.Sprintf("timestamps = %#v", got))
		}
		if got.Refresh.TokenHash != "refresh-hash-user-id" || got.Refresh.SessionID == "" || got.Refresh.UserID == "" {
			Expect(false).To(BeTrue(), fmt.Sprintf("refresh = %#v", got.Refresh))
		}
		Expect(got.User.ID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`))
		Expect(got.Refresh.SessionID).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`))

	})
})
