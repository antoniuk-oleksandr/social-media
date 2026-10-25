package dynamodb

import (
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"github.com/google/uuid"
)

type UserRecord struct {
	ID           string `dynamodbav:"userId"`
	Username     string `dynamodbav:"username"`
	Email        string `dynamodbav:"email"`
	PasswordHash string `dynamodbav:"passwordHash"`
	DisplayName  string `dynamodbav:"displayName"`
	Role         string `dynamodbav:"role"`
	Status       string `dynamodbav:"status"`
	CreatedAt    string `dynamodbav:"createdAt"`
	UpdatedAt    string `dynamodbav:"updatedAt"`
}

func toDomain(record UserRecord) (user.User, error) {
	id, err := uuid.Parse(record.ID)
	if err != nil || id.Version() != 7 || id.String() != record.ID {
		return user.User{}, user.ErrDependencyUnavailable
	}
	createdAt, err := time.Parse(time.RFC3339, record.CreatedAt)
	if err != nil {
		return user.User{}, user.ErrDependencyUnavailable
	}
	return user.User{ID: record.ID, Username: record.Username, Email: record.Email, PasswordHash: record.PasswordHash, DisplayName: record.DisplayName, Role: user.Role(record.Role), Status: user.Status(record.Status), CreatedAt: createdAt}, nil
}
