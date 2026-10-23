//go:build integration

package authintegration

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	authdomain "github.com/antoniuk-oleksandr/social-media/internal/auth"
	platformjwt "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/password"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/token"
	"github.com/antoniuk-oleksandr/social-media/tests/fixtures"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	jwtpkg "github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("login persistence", func() {
	It("finds by username or email, issues required JWT claims, and persists an opaque refresh session", func() {
		passwordHash, err := (password.Bcrypt{}).Hash("correct-password")
		Expect(err).NotTo(HaveOccurred())
		registration := fixtures.Registration("integration-login-user", "integration_login", "integration-login@example.com")
		registration.User.PasswordHash = passwordHash
		Expect(integrationStore().Register(context.Background(), registration)).To(Succeed())

		access := platformjwt.Access{Keys: platformjwt.StaticKey{KID: "test-key", Secret: []byte("test-secret")}, KID: "test-key", Issuer: "social-media", Audience: "social-media-api", TTL: 15 * time.Minute, ID: func() string { return "login-jti" }}
		svc := authdomain.Service{LoginStore: integrationStore(), Verifier: password.Bcrypt{}, IDs: fixedID("login-session"), Clock: fixedClock{at: registration.User.CreatedAt}, Access: access, Opaque: token.Opaque{}, AccessTTL: 15 * time.Minute, RefreshTTL: 7 * 24 * time.Hour}
		for _, login := range []string{registration.User.Username, strings.ToUpper(registration.User.Email)} {
			result, loginErr := svc.Login(context.Background(), authdomain.LoginCommand{Login: login, Password: "correct-password"})
			Expect(loginErr).NotTo(HaveOccurred())
			Expect(result.Tokens.RefreshToken).NotTo(BeEmpty())
			Expect(len(result.Tokens.RefreshToken)).To(BeNumerically(">=", 43))
			Expect(result.Tokens.ExpiresIn).To(Equal(int64(900)))
			Expect(result.Tokens.RefreshExpiresIn).To(Equal(int64(604800)))
			parsed, parseErr := jwtpkg.Parse(result.Tokens.AccessToken, func(token *jwtpkg.Token) (any, error) { return []byte("test-secret"), nil }, jwtpkg.WithValidMethods([]string{"HS256"}))
			Expect(parseErr).NotTo(HaveOccurred())
			claims, ok := parsed.Claims.(jwtpkg.MapClaims)
			Expect(ok).To(BeTrue())
			for _, claim := range []string{"iss", "sub", "aud", "role", "iat", "nbf", "exp", "jti"} {
				Expect(claims).To(HaveKey(claim))
			}
			Expect(claims["sub"]).To(Equal(registration.User.ID))
			Expect(claims["role"]).To(Equal(string(registration.User.Role)))
			digest := sha256.Sum256([]byte(result.Tokens.RefreshToken))
			stored, getErr := integrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String(integrationSuite.RefreshTable), Key: map[string]types.AttributeValue{"tokenHash": &types.AttributeValueMemberS{Value: base64.RawURLEncoding.EncodeToString(digest[:])}}})
			Expect(getErr).NotTo(HaveOccurred())
			Expect(stored.Item).NotTo(BeEmpty())
			Expect(stored.Item).To(HaveKey("expiresAt"))
			Expect(stored.Item).To(HaveKey("ttl"))
			Expect(stored.Item["tokenHash"]).NotTo(Equal(&types.AttributeValueMemberS{Value: result.Tokens.RefreshToken}))
		}
	})

	It("returns generic invalid credentials for missing users and wrong passwords", func() {
		store := integrationStore()
		hash, err := (password.Bcrypt{}).Hash("correct-password")
		Expect(err).NotTo(HaveOccurred())
		registration := fixtures.Registration("integration-invalid-user", "integration_invalid", "integration-invalid@example.com")
		registration.User.PasswordHash = hash
		Expect(store.Register(context.Background(), registration)).To(Succeed())
		svc := authdomain.Service{LoginStore: store, Verifier: password.Bcrypt{}, IDs: fixedID("invalid-session"), Clock: fixedClock{at: time.Now().UTC()}, Access: platformjwt.Access{Keys: platformjwt.StaticKey{KID: "k", Secret: []byte("secret")}}, Opaque: token.Opaque{}, RefreshTTL: 7 * 24 * time.Hour}
		_, missingErr := svc.Login(context.Background(), authdomain.LoginCommand{Login: "nobody@example.com", Password: "correct-password"})
		_, wrongErr := svc.Login(context.Background(), authdomain.LoginCommand{Login: registration.User.Email, Password: "wrong-password"})
		Expect(errors.Is(missingErr, authdomain.ErrInvalidCredentials)).To(BeTrue())
		Expect(errors.Is(wrongErr, authdomain.ErrInvalidCredentials)).To(BeTrue())
	})
})

type fixedID string

func (i fixedID) New() (string, error) { return string(i), nil }

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }
