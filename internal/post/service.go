package post

import (
	"context"
	"strings"
	"time"
)

type Service interface {
	Create(context.Context, CreateCommand) (Post, error)
	GetByID(context.Context, string) (Post, error)
	Delete(context.Context, string, string) error
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
	content, err := NormalizeContent(command.Content)
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
	createdAt := s.clock.Now().UTC().Truncate(time.Millisecond)
	value := Post{ID: id, AuthorID: command.AuthorID, Content: strings.TrimSpace(content), CreatedAt: createdAt, CreatedAtEpoch: createdAt.UnixMilli()}
	if err := s.repository.Create(ctx, value); err != nil {
		return Post{}, err
	}
	return value, nil
}

func (s *service) GetByID(ctx context.Context, id string) (Post, error) {
	if !validUUIDv7(id) {
		return Post{}, ErrInvalidID
	}
	return s.repository.GetByID(ctx, id)
}

func (s *service) Delete(ctx context.Context, id, ownerID string) error {
	if !validUUIDv7(id) {
		return ErrInvalidID
	}
	value, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if value.AuthorID != ownerID {
		return ErrForbidden
	}
	return s.repository.Delete(ctx, id, ownerID)
}
