package fixtures

import (
	"crypto/sha256"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"github.com/google/uuid"
)

func Registration(id, username, email string) auth.Registration {
	createdAt := time.Date(2026, time.October, 22, 22, 0, 0, 0, time.UTC)
	canonicalID := fixtureUUID(id)
	return auth.Registration{
		User: user.User{
			ID:           canonicalID,
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
			SessionID: fixtureUUID("session:" + id),
			UserID:    canonicalID,
			CreatedAt: createdAt,
			ExpiresAt: createdAt.Add(7 * 24 * time.Hour),
		},
	}
}

func fixtureUUID(seed string) string {
	digest := sha256.Sum256([]byte(seed))
	id := uuid.UUID(digest[:16])
	id[6] = (id[6] & 0x0f) | 0x70
	id[8] = (id[8] & 0x3f) | 0x80
	return id.String()
}
