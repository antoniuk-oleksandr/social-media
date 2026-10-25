package user

import (
	"context"
	"errors"
	"strings"
)

type Service interface {
	GetByID(context.Context, string) (User, error)
	UpdateProfile(context.Context, UpdateProfileCommand) (User, error)
}

type service struct {
	repository Repository
}

func NewService(repository Repository) Service {
	return &service{repository: repository}
}

func (s *service) GetByID(ctx context.Context, id string) (User, error) {
	if !validUUIDv7(id) {
		return User{}, ErrInvalidID
	}
	return s.repository.GetByID(ctx, id)
}

func (s *service) UpdateProfile(ctx context.Context, command UpdateProfileCommand) (User, error) {
	if !validUUIDv7(command.UserID) {
		return User{}, ErrInvalidID
	}
	if command.Username == nil && command.DisplayName == nil {
		return User{}, ErrValidation
	}
	if command.Username != nil {
		lower := strings.ToLower(*command.Username)
		command.Username = &lower
		if !validUsername(lower) {
			return User{}, ErrValidation
		}
	}
	if command.DisplayName != nil && !validDisplayName(*command.DisplayName) {
		return User{}, ErrValidation
	}
	return s.repository.UpdateProfile(ctx, command)
}

var ErrInvalidID = errors.New("invalid user id")
