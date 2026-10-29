package fx

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	jwtkey "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	platformobservability "github.com/antoniuk-oleksandr/social-media/internal/platform/observability"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/password"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/token"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

func LoadAWSConfig(ctx context.Context) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx, config.WithRegion(Value("AWS_REGION", "us-east-1")))
}

func NewDynamoDBClient(cfg aws.Config) *dynamodb.Client {
	cfg.APIOptions = append(cfg.APIOptions, platformobservability.DynamoDBAPIOption())
	if endpoint := os.Getenv("DYNAMODB_ENDPOINT"); endpoint != "" {
		cfg.BaseEndpoint = &endpoint
		return dynamodb.NewFromConfig(cfg, func(options *dynamodb.Options) {
			// DynamoDB Local may emit an invalid CRC32 response header. Keep
			// checksum validation enabled for real AWS clients.
			options.DisableValidateResponseChecksum = true
		})
	}
	return dynamodb.NewFromConfig(cfg)
}

func NewClock() clock.Real {
	return clock.Real{}
}

func NewIDGenerator() id.UUIDv7 {
	return id.UUIDv7{}
}

func NewPasswordHasher() password.Bcrypt {
	return password.Bcrypt{Cost: 12}
}

func NewAccessTokenIssuer() jwtkey.Access {
	kid := Value("JWT_KID", "local")
	return jwtkey.Access{
		Keys:     jwtkey.StaticKey{KID: kid, Secret: []byte(os.Getenv("JWT_ACCESS_SECRET"))},
		KID:      kid,
		Issuer:   Value("JWT_ISSUER", "social-media-local"),
		Audience: Value("JWT_AUDIENCE", "social-media-api"),
		TTL:      15 * time.Minute,
	}
}

// NewConfiguredAccessTokenIssuer resolves the signing key during cold-start.
// Local development may explicitly provide JWT_ACCESS_SECRET; deployed
// environments must provide an SSM SecureString parameter name.
func NewConfiguredAccessTokenIssuer(cfg aws.Config) (jwtkey.Access, error) {
	kid := os.Getenv("JWT_KID")
	if kid == "" {
		return jwtkey.Access{}, fmt.Errorf("JWT_KID is required")
	}
	secret := os.Getenv("JWT_ACCESS_SECRET")
	if os.Getenv("APP_ENV") != "local" {
		parameter := os.Getenv("JWT_ACCESS_SECRET_PARAMETER")
		if parameter == "" {
			return jwtkey.Access{}, fmt.Errorf("JWT_ACCESS_SECRET_PARAMETER is required")
		}
		out, err := ssm.NewFromConfig(cfg).GetParameter(context.Background(), &ssm.GetParameterInput{Name: aws.String(parameter), WithDecryption: aws.Bool(true)})
		if err != nil || out.Parameter == nil || out.Parameter.Value == nil || *out.Parameter.Value == "" {
			if err == nil {
				err = fmt.Errorf("empty signing key")
			}
			return jwtkey.Access{}, fmt.Errorf("load JWT signing key: %w", err)
		}
		secret = *out.Parameter.Value
	}
	if secret == "" {
		return jwtkey.Access{}, fmt.Errorf("JWT_ACCESS_SECRET is required for local environment")
	}
	return jwtkey.Access{Keys: jwtkey.StaticKey{KID: kid, Secret: []byte(secret)}, KID: kid, Issuer: Value("JWT_ISSUER", "social-media-api"), Audience: Value("JWT_AUDIENCE", "social-media-api"), TTL: 15 * time.Minute}, nil
}

// NewConfiguredAdsConfig resolves the Ads cursor key once during cold start.
func NewConfiguredAdsConfig(cfg aws.Config) (platformconfig.AdsConfig, error) {
	ads := platformconfig.LoadAdsConfig()
	if os.Getenv("APP_ENV") != "local" {
		parameter := os.Getenv("ADS_CURSOR_KEY_PARAMETER")
		if parameter == "" {
			return platformconfig.AdsConfig{}, fmt.Errorf("ADS_CURSOR_KEY_PARAMETER is required")
		}
		out, err := ssm.NewFromConfig(cfg).GetParameter(context.Background(), &ssm.GetParameterInput{Name: aws.String(parameter), WithDecryption: aws.Bool(true)})
		if err != nil || out.Parameter == nil || out.Parameter.Value == nil || *out.Parameter.Value == "" {
			if err == nil {
				err = fmt.Errorf("empty cursor signing key")
			}
			return platformconfig.AdsConfig{}, fmt.Errorf("load Ads cursor signing key: %w", err)
		}
		ads.CursorKey = *out.Parameter.Value
	}
	if ads.CursorKey == "" {
		return platformconfig.AdsConfig{}, fmt.Errorf("ADS_CURSOR_KEY is required for local environment")
	}
	return ads, nil
}

func NewOpaqueTokenGenerator() token.Opaque {
	return token.Opaque{}
}

func Value(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
