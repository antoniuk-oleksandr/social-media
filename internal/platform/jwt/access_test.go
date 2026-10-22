package jwt

import (
	"context"
	"fmt"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	gjwt "github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("StaticKeyAndAccessIssue", func() {
	It("runs the test", func() {
		now := time.Now()
		a := Access{Keys: StaticKey{KID: "one", Secret: []byte("secret")}, KID: "one", Issuer: "issuer", Audience: "aud", TTL: time.Minute, ID: func() string { return "jti" }}
		token, e := a.Issue(context.Background(), user.User{ID: "u", Role: user.RoleUser}, now)
		if e != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(e))
		}
		parsed, e := gjwt.Parse(token, func(tok *gjwt.Token) (any, error) {
			if tok.Header["kid"] != "one" {
				Expect(false).To(BeTrue(), fmt.Sprint(tok.Header))
			}
			if tok.Header["typ"] != "JWT" {
				Expect(false).To(BeTrue(), fmt.Sprintf("typ = %v, want JWT", tok.Header["typ"]))
			}
			if tok.Method != gjwt.SigningMethodHS256 {
				Expect(false).To(BeTrue(), fmt.Sprintf("signing method = %v, want HS256", tok.Method.Alg()))
			}
			return []byte("secret"), nil
		})
		if e != nil || !parsed.Valid {
			Expect(false).To(BeTrue(), fmt.Sprintf("%v", e))
		}
		claims, ok := parsed.Claims.(gjwt.MapClaims)
		if !ok {
			Expect(false).To(BeTrue(), fmt.Sprintf("claims type = %T, want MapClaims", parsed.Claims))
		}
		for name, want := range map[string]string{"iss": "issuer", "sub": "u", "aud": "aud", "role": string(user.RoleUser), "jti": "jti"} {
			if got, ok := claims[name].(string); !ok || got != want {
				Expect(false).To(BeTrue(), fmt.Sprintf("claim %s = %#v, want %q", name, claims[name], want))
			}
		}
		for _, name := range []string{"iat", "nbf", "exp"} {
			if _, ok := claims[name].(float64); !ok {
				Expect(false).To(BeTrue(), fmt.Sprintf("claim %s = %#v, want numeric date", name, claims[name]))
			}
		}
		if exp, err := parsed.Claims.GetExpirationTime(); err != nil || exp == nil || exp.Time.Before(now.Add(59*time.Second)) || exp.Time.After(now.Add(61*time.Second)) {
			Expect(false).To(BeTrue(), fmt.Sprintf("exp = %v, err=%v", exp, err))
		}
		if _, e := (StaticKey{KID: "one"}).Key(context.Background(), "bad"); e == nil {
			Expect(false).To(BeTrue(), fmt.Sprint("expected unknown key"))
		}

	})
})
var _ = Describe("AccessIssuePropagatesKeyErrorAndDefaults", func() {
	It("runs the test", func() {
		a := Access{Keys: StaticKey{KID: "wanted", Secret: nil}, KID: "other"}
		if _, e := a.Issue(context.Background(), user.User{}, time.Now()); e == nil {
			Expect(false).To(BeTrue(), fmt.Sprint("expected key error"))
		}

	})
})

var _ = Describe("AccessIssueUsesDefaultTTLClockAndID", func() {
	It("runs the test", func() {
		now := time.Unix(100, 0)
		a := Access{
			Keys:     StaticKey{KID: "default", Secret: []byte("secret")},
			KID:      "default",
			Issuer:   "issuer",
			Audience: "audience",
		}
		token, err := a.Issue(context.Background(), user.User{ID: "user", Role: user.RoleUser}, now)
		if err != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(err))
		}
		parsed, _, err := new(gjwt.Parser).ParseUnverified(token, gjwt.MapClaims{})
		if err != nil {
			Expect(false).To(BeTrue(), fmt.Sprintf("parse token: %v", err))
		}
		claims, ok := parsed.Claims.(gjwt.MapClaims)
		if !ok {
			Expect(false).To(BeTrue(), fmt.Sprintf("claims type = %T, want MapClaims", parsed.Claims))
		}
		if got, want := claims["exp"], float64(now.Add(15*time.Minute).Unix()); got != want {
			Expect(false).To(BeTrue(), fmt.Sprintf("exp = %v, want %v", got, want))
		}
		if got, ok := claims["jti"].(string); !ok || got == "" {
			Expect(false).To(BeTrue(), fmt.Sprintf("jti = %v, want non-empty string", claims["jti"]))
		}

	})
})
