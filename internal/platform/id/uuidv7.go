package id

import "github.com/google/uuid"

type UUIDv7 struct{}

func (UUIDv7) New() (string, error) { u, e := uuid.NewV7(); return u.String(), e }
