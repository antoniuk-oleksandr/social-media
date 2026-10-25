package id

import "github.com/google/uuid"

type UUIDv7 struct{}

func (UUIDv7) New() (string, error) { u, e := uuid.NewV7(); return u.String(), e }

func IsCanonicalUUIDv7(value string) bool {
	u, err := uuid.Parse(value)
	return err == nil && u.Version() == 7 && u.Variant() == uuid.RFC4122 && u.String() == value
}
