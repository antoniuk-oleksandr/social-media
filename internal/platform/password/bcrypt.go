package password

import "golang.org/x/crypto/bcrypt"

type Bcrypt struct{ Cost int }

func (b Bcrypt) Hash(s string) (string, error) {
	c := b.Cost
	if c == 0 {
		c = 12
	}
	h, e := bcrypt.GenerateFromPassword([]byte(s), c)
	return string(h), e
}
