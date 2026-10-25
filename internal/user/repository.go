package user

import "context"

type Repository interface {
	GetByID(context.Context, string) (User, error)
	UpdateProfile(context.Context, UpdateProfileCommand) (User, error)
}
