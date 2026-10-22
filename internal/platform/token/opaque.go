package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

type Opaque struct{}

func (Opaque) Generate() (string, string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", "", e
	}
	p := base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(p))
	return p, base64.RawURLEncoding.EncodeToString(h[:]), nil
}
