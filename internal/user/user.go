package user

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("user not found")

type Role string

const RoleUser Role = "user"

type Status string

const StatusActive Status = "active"

type User struct {
	ID, Username, Email, PasswordHash, DisplayName string
	Role                                           Role
	Status                                         Status
	CreatedAt                                      time.Time
}
