package auth

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"
)

type FieldErrors map[string]string

func Validate(c RegisterCommand) (RegisterCommand, FieldErrors) {
	e := FieldErrors{}
	if len(c.Username) < 3 || len(c.Username) > 30 {
		e["username"] = "must be 3-30 characters"
	} else {
		for _, r := range c.Username {
			if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_') {
				e["username"] = "must contain lowercase ASCII letters, digits, or underscores"
				break
			}
		}
	}
	c.Email = strings.ToLower(strings.TrimSpace(c.Email))
	if len(c.Email) > 254 {
		e["email"] = "must be at most 254 bytes"
	} else if a, err := mail.ParseAddress(c.Email); err != nil || a.Address != c.Email || !strings.Contains(c.Email, "@") {
		e["email"] = "must be a valid email"
	}
	if len([]byte(c.Password)) < 12 || len([]byte(c.Password)) > 72 {
		e["password"] = "must be 12-72 bytes"
	}
	if !utf8.ValidString(c.DisplayName) || utf8.RuneCountInString(c.DisplayName) < 1 || utf8.RuneCountInString(c.DisplayName) > 80 {
		e["displayName"] = "must be 1-80 Unicode code points"
	}
	return c, e
}
func (e FieldErrors) Error() string { return fmt.Sprint(map[string]string(e)) }
