package fixtures

import (
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
)

func Registration(id, username, email string) auth.Registration {
	createdAt := time.Date(2026, time.October, 22, 22, 0, 0, 0, time.UTC)
	return auth.Registration{
		User: user.User{
			ID:           id,
			Username:     username,
			Email:        email,
			PasswordHash: "$2a$12$deterministic-fixture-password-hash",
			DisplayName:  "Fixture " + username,
			Role:         user.RoleUser,
			Status:       user.StatusActive,
			CreatedAt:    createdAt,
		},
		Refresh: auth.RefreshSession{
			TokenHash: "refresh-hash-" + id,
			SessionID: "session-" + id,
			UserID:    id,
			CreatedAt: createdAt,
			ExpiresAt: createdAt.Add(7 * 24 * time.Hour),
		},
	}
}
