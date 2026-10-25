package user

import (
	"context"
	"errors"
)

type Service interface {
	GetByID(context.Context, string) (User, error)
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

var ErrInvalidID = errors.New("invalid user id")
