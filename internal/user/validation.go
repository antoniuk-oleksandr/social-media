package user

import (
	"regexp"
	"unicode/utf8"

	"github.com/google/uuid"
)

func validUUIDv7(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.String() == value
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,30}$`)

func validUsername(value string) bool { return usernamePattern.MatchString(value) }

func validDisplayName(value string) bool {
	return value != "" && utf8.RuneCountInString(value) <= 80
}
