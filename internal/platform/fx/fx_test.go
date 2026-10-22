package fx

import (
	"context"
	"encoding/json"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

var _ = Describe("ConstructorsAndValues", func() {
	It("runs the test", func() {
		const key = "PLATFORM_FX_TEST_VALUE"
		os.Unsetenv(key)
		if got := Value(key, "fallback"); got != "fallback" {
			Expect(false).To(BeTrue(), fmt.Sprint(got))
		}
		os.Setenv(key, "configured")
		defer os.Unsetenv(key)
		if got := Value(key, "fallback"); got != "configured" {
			Expect(false).To(BeTrue(), fmt.Sprint(got))
		}
		os.Setenv("DYNAMODB_ENDPOINT", "http://127.0.0.1:8000")
		defer os.Unsetenv("DYNAMODB_ENDPOINT")
		if NewDynamoDBClient(aws.Config{Region: "test"}) == nil || NewClock().Now().IsZero() || NewPasswordHasher().Cost == 0 {
			Expect(false).To(BeTrue(), fmt.Sprint("invalid dependency"))
		}
		if _, err := NewIDGenerator().New(); err != nil || NewAccessTokenIssuer().KID == "" {
			Expect(false).To(BeTrue(), fmt.Sprint("invalid dependency"))
		}
		if _, _, err := NewOpaqueTokenGenerator().Generate(); err != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(err))
		}
		os.Setenv("AWS_REGION", "eu-west-1")
		defer os.Unsetenv("AWS_REGION")
		if cfg, err := LoadAWSConfig(context.Background()); err != nil || cfg.Region != "eu-west-1" {
			Expect(false).To(BeTrue(), fmt.Sprintf("config: %q %v", cfg.Region, err))
		}
		os.Setenv("JWT_KID", "configured-kid")
		os.Setenv("JWT_ISSUER", "configured-issuer")
		os.Setenv("JWT_AUDIENCE", "configured-audience")
		os.Setenv("JWT_ACCESS_SECRET", "configured-secret")
		defer func() {
			os.Unsetenv("JWT_KID")
			os.Unsetenv("JWT_ISSUER")
			os.Unsetenv("JWT_AUDIENCE")
			os.Unsetenv("JWT_ACCESS_SECRET")
		}()
		issuer := NewAccessTokenIssuer()
		if issuer.KID != "configured-kid" || issuer.Issuer != "configured-issuer" || issuer.Audience != "configured-audience" {
			Expect(false).To(BeTrue(), fmt.Sprint("configured issuer not loaded"))
		}

	})
})

var _ = Describe("NewConfiguredAccessTokenIssuerValidationAndLocalSecret", func() {
	It("runs the test", func() {
		os.Setenv("JWT_KID", "")
		DeferCleanup(func() { _ = os.Unsetenv("JWT_KID") })
		os.Setenv("APP_ENV", "local")
		DeferCleanup(func() { _ = os.Unsetenv("APP_ENV") })
		os.Setenv("JWT_ACCESS_SECRET", "secret")
		DeferCleanup(func() { _ = os.Unsetenv("JWT_ACCESS_SECRET") })
		if _, err := NewConfiguredAccessTokenIssuer(aws.Config{}); err == nil {
			Expect(false).To(BeTrue(), fmt.Sprint("expected missing key ID error"))
		}

		os.Setenv("JWT_KID", "kid")
		DeferCleanup(func() { _ = os.Unsetenv("JWT_KID") })
		os.Setenv("JWT_ACCESS_SECRET", "")
		DeferCleanup(func() { _ = os.Unsetenv("JWT_ACCESS_SECRET") })
		if _, err := NewConfiguredAccessTokenIssuer(aws.Config{}); err == nil {
			Expect(false).To(BeTrue(), fmt.Sprint("expected missing local secret error"))
		}

		os.Setenv("JWT_ACCESS_SECRET", "secret")
		DeferCleanup(func() { _ = os.Unsetenv("JWT_ACCESS_SECRET") })
		issuer, err := NewConfiguredAccessTokenIssuer(aws.Config{})
		if err != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(err))
		}
		key, err := issuer.Keys.Key(context.Background(), "kid")
		if issuer.KID != "kid" || string(key) != "secret" || err != nil {
			Expect(false).To(BeTrue(), fmt.Sprintf("issuer = %+v, key error = %v", issuer, err))
		}

		os.Setenv("APP_ENV", "production")
		DeferCleanup(func() { _ = os.Unsetenv("APP_ENV") })
		os.Setenv("JWT_ACCESS_SECRET_PARAMETER", "")
		DeferCleanup(func() { _ = os.Unsetenv("JWT_ACCESS_SECRET_PARAMETER") })
		if _, err := NewConfiguredAccessTokenIssuer(aws.Config{}); err == nil {
			Expect(false).To(BeTrue(), fmt.Sprint("expected missing parameter error"))
		}

	})
})

var _ = Describe("NewConfiguredAccessTokenIssuerLoadsSecureStringWithDecryption", func() {
	It("runs the test", func() {
		os.Setenv("APP_ENV", "production")
		DeferCleanup(func() { _ = os.Unsetenv("APP_ENV") })
		os.Setenv("JWT_KID", "ssm-kid")
		DeferCleanup(func() { _ = os.Unsetenv("JWT_KID") })
		os.Setenv("JWT_ACCESS_SECRET", "")
		DeferCleanup(func() { _ = os.Unsetenv("JWT_ACCESS_SECRET") })
		os.Setenv("JWT_ACCESS_SECRET_PARAMETER", "/social-media/jwt")
		DeferCleanup(func() { _ = os.Unsetenv("JWT_ACCESS_SECRET_PARAMETER") })
		os.Setenv("JWT_ISSUER", "test-issuer")
		DeferCleanup(func() { _ = os.Unsetenv("JWT_ISSUER") })
		os.Setenv("JWT_AUDIENCE", "test-audience")
		DeferCleanup(func() { _ = os.Unsetenv("JWT_AUDIENCE") })

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("X-Amz-Target"); got != "AmazonSSM.GetParameter" {
				Expect(false).To(BeTrue(), fmt.Sprintf("X-Amz-Target = %q", got))
			}
			var request struct {
				Name           string `json:"Name"`
				WithDecryption bool   `json:"WithDecryption"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				Expect(false).To(BeTrue(), fmt.Sprintf("decode request: %v", err))
			}
			if request.Name != "/social-media/jwt" || !request.WithDecryption {
				Expect(false).To(BeTrue(), fmt.Sprintf("request = %+v, want parameter and decryption", request))
			}
			w.Header().Set("Content-Type", "application/x-amz-json-1.1")
			if _, err := w.Write([]byte(`{"Parameter":{"Name":"/social-media/jwt","Type":"SecureString","Value":"ssm-secret","Version":1}}`)); err != nil {
				Expect(false).To(BeTrue(), fmt.Sprintf("write response: %v", err))
			}
		}))
		defer server.Close()

		cfg := aws.Config{Region: "us-east-1", BaseEndpoint: aws.String(server.URL), Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}
		issuer, err := NewConfiguredAccessTokenIssuer(cfg)
		if err != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(err))
		}
		key, err := issuer.Keys.Key(context.Background(), "ssm-kid")
		if err != nil || string(key) != "ssm-secret" {
			Expect(false).To(BeTrue(), fmt.Sprintf("key=%q err=%v", key, err))
		}
		if issuer.Issuer != "test-issuer" || issuer.Audience != "test-audience" || issuer.TTL != 15*time.Minute {
			Expect(false).To(BeTrue(), fmt.Sprintf("issuer = %+v", issuer))
		}

	})
})

var _ = Describe("NewConfiguredAccessTokenIssuerRejectsSSMFailures", func() {
	It("runs the test", func() {
		for name, response := range map[string]string{
			"service error": `{"__type":"ParameterNotFound","message":"missing"}`,
			"empty value":   `{"Parameter":{"Name":"/social-media/jwt","Type":"SecureString"}}`,
		} {
			{
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/x-amz-json-1.1")
					if name == "service error" {
						w.WriteHeader(http.StatusNotFound)
					}
					if _, err := w.Write([]byte(response)); err != nil {
						Expect(false).To(BeTrue(), fmt.Sprintf("write response: %v", err))
					}
				}))
				defer server.Close()
				os.Setenv("APP_ENV", "production")
				DeferCleanup(func() { _ = os.Unsetenv("APP_ENV") })
				os.Setenv("JWT_KID", "kid")
				DeferCleanup(func() { _ = os.Unsetenv("JWT_KID") })
				os.Setenv("JWT_ACCESS_SECRET", "")
				DeferCleanup(func() { _ = os.Unsetenv("JWT_ACCESS_SECRET") })
				os.Setenv("JWT_ACCESS_SECRET_PARAMETER", "/social-media/jwt")
				DeferCleanup(func() { _ = os.Unsetenv("JWT_ACCESS_SECRET_PARAMETER") })
				cfg := aws.Config{Region: "us-east-1", BaseEndpoint: aws.String(server.URL), Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}
				if _, err := NewConfiguredAccessTokenIssuer(cfg); err == nil {
					Expect(false).To(BeTrue(), fmt.Sprint("expected SSM failure"))
				}
			}
		}

	})
})
