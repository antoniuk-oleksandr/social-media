package config

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

type UsersConfig struct {
	UsersTable   string
	UniquesTable string
}

func Load() (aws.Config, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	return awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
}

func LoadUsersConfig() UsersConfig {
	return UsersConfig{UsersTable: value("USERS_TABLE", "social-media-local-users"), UniquesTable: value("UNIQUES_TABLE", "social-media-local-uniques")}
}

func value(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
