package jwt

import (
	"context"
	"errors"
	"fmt"
	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	gjwt "github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("StaticKeyAndAccessIssue", func() {
	It("runs the test", func() {
		now := time.Now()
		const jti = "0192f8a0-7b00-7000-8000-000000000002"
		a := Access{Keys: StaticKey{KID: "one", Secret: []byte("secret")}, KID: "one", Issuer: "issuer", Audience: "aud", TTL: time.Minute, ID: func() string { return jti }}
		token, e := a.Issue(context.Background(), user.User{ID: "0192f8a0-7b00-7000-8000-000000000001", Role: user.RoleUser}, now)
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
		for name, want := range map[string]string{"iss": "issuer", "sub": "0192f8a0-7b00-7000-8000-000000000001", "aud": "aud", "role": string(user.RoleUser), "jti": jti} {
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
		token, err := a.Issue(context.Background(), user.User{ID: "0192f8a0-7b00-7000-8000-000000000001", Role: user.RoleUser}, now)
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
		Expect(platformid.IsCanonicalUUIDv7(claims["jti"].(string))).To(BeTrue())

	})
})

var _ = Describe("AccessIssueRejectsInvalidInjectedJTI", func() {
	It("rejects a non-UUIDv7 injected jti before signing", func() {
		a := Access{Keys: StaticKey{KID: "key", Secret: []byte("secret")}, KID: "key", ID: func() string { return "not-a-uuid" }}
		_, err := a.Issue(context.Background(), user.User{ID: "0192f8a0-7b00-7000-8000-000000000001"}, time.Unix(100, 0))
		Expect(err).To(MatchError("invalid JWT ID"))
	})
})

var _ = Describe("AccessIssueValidatesUserIDBeforeSigning", func() {
	It("rejects malformed user IDs and signs a token for canonical UUIDv7 IDs", func() {
		issue := func(userID string) (string, error) {
			a := Access{
				Keys: StaticKey{KID: "key", Secret: []byte("secret")},
				KID:  "key",
				ID:   func() string { return "0192f8a0-7b00-7000-8000-000000000002" },
			}
			return a.Issue(context.Background(), user.User{ID: userID, Role: user.RoleUser}, time.Unix(100, 0))
		}

		for _, malformed := range []string{"not-a-uuid", "0192f8a0-7b00-7000-8000-00000000000A", "0192f8a0-7b00-6000-8000-000000000001"} {
			token, err := issue(malformed)
			Expect(token).To(BeEmpty(), malformed)
			Expect(errors.Is(err, auth.ErrValidation)).To(BeTrue(), malformed)
		}

		token, err := issue("0192f8a0-7b00-7000-8000-000000000001")
		Expect(err).NotTo(HaveOccurred())
		Expect(token).NotTo(BeEmpty())
	})
})

var _ = Describe("AccessAuthenticate", func() {
	const (
		userID = "0192f8a0-7b00-7000-8000-000000000001"
		jti    = "0192f8a0-7b00-7000-8000-000000000002"
	)

	newAccess := func() Access {
		return Access{Keys: StaticKey{KID: "key", Secret: []byte("secret")}, KID: "key", Issuer: "issuer", Audience: "audience"}
	}

	It("authenticates an issued user token and returns its canonical subject", func() {
		access := newAccess()
		token, err := access.Issue(context.Background(), user.User{ID: userID, Role: user.RoleUser}, time.Now())
		Expect(err).NotTo(HaveOccurred())

		got, err := access.Authenticate(context.Background(), token)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(userID))
	})

	DescribeTable("rejects tokens that violate required access-token claims", func(mutator func(gjwt.MapClaims, *gjwt.Token)) {
		access := newAccess()
		claims := gjwt.MapClaims{
			"iss": "issuer", "sub": userID, "aud": "audience", "role": string(user.RoleUser),
			"iat": float64(time.Now().Unix()), "nbf": float64(time.Now().Unix()), "exp": float64(time.Now().Add(time.Minute).Unix()), "jti": jti,
		}
		token := gjwt.NewWithClaims(gjwt.SigningMethodHS256, claims)
		token.Header["kid"] = "key"
		mutator(claims, token)
		serialized, err := token.SignedString([]byte("secret"))
		Expect(err).NotTo(HaveOccurred())

		_, err = access.Authenticate(context.Background(), serialized)
		Expect(err).To(HaveOccurred())
	},
		Entry("wrong issuer", func(claims gjwt.MapClaims, _ *gjwt.Token) { claims["iss"] = "other" }),
		Entry("wrong audience", func(claims gjwt.MapClaims, _ *gjwt.Token) { claims["aud"] = "other" }),
		Entry("invalid role", func(claims gjwt.MapClaims, _ *gjwt.Token) { claims["role"] = "moderator" }),
		Entry("missing issued-at", func(claims gjwt.MapClaims, _ *gjwt.Token) { delete(claims, "iat") }),
		Entry("invalid not-before type", func(claims gjwt.MapClaims, _ *gjwt.Token) { claims["nbf"] = "now" }),
		Entry("invalid JWT ID", func(claims gjwt.MapClaims, _ *gjwt.Token) { claims["jti"] = "not-a-uuid" }),
		Entry("invalid subject", func(claims gjwt.MapClaims, _ *gjwt.Token) { claims["sub"] = "not-a-uuid" }),
		Entry("unknown key ID", func(_ gjwt.MapClaims, token *gjwt.Token) { token.Header["kid"] = "unknown" }),
		Entry("missing key ID", func(_ gjwt.MapClaims, token *gjwt.Token) { delete(token.Header, "kid") }),
	)

	It("rejects an unsupported signing method", func() {
		access := newAccess()
		token := gjwt.NewWithClaims(gjwt.SigningMethodHS512, gjwt.MapClaims{"iss": "issuer"})
		serialized, err := token.SignedString([]byte("secret"))
		Expect(err).NotTo(HaveOccurred())
		_, err = access.Authenticate(context.Background(), serialized)
		Expect(err).To(MatchError("invalid access token"))
	})
})
