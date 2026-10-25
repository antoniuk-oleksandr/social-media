package post

import (
	"context"
	"strings"
)

type Service interface {
	Create(context.Context, CreateCommand) (Post, error)
}

type service struct {
	repository Repository
	ids        IDGenerator
	clock      Clock
}

func NewService(repository Repository, ids IDGenerator, clock Clock) Service {
	return &service{repository: repository, ids: ids, clock: clock}
}

func (s *service) Create(ctx context.Context, command CreateCommand) (Post, error) {
	if !validUUIDv7(command.AuthorID) {
		return Post{}, ErrValidation
	}
	content, err := normalizeContent(command.Content)
	if err != nil {
		return Post{}, err
	}
	id, err := s.ids.New()
	if err != nil {
		return Post{}, ErrDependencyUnavailable
	}
	if !validUUIDv7(id) {
		return Post{}, ErrDependencyUnavailable
	}
	createdAt := s.clock.Now().UTC()
	value := Post{ID: id, AuthorID: command.AuthorID, Content: strings.TrimSpace(content), CreatedAt: createdAt, CreatedAtEpoch: createdAt.UnixMilli()}
	if err := s.repository.Create(ctx, value); err != nil {
		return Post{}, err
	}
	return value, nil
}
