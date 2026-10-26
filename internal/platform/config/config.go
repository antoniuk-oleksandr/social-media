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

type PostsConfig struct {
	PostsTable string
}

type AdsConfig struct {
	AdsTable      string
	CursorKey     string
	CursorKeyID   string
	CursorVersion int
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

func LoadPostsConfig() PostsConfig {
	return PostsConfig{PostsTable: value("POSTS_TABLE", "social-media-local-posts")}
}

func LoadAdsConfig() AdsConfig {
	return AdsConfig{AdsTable: value("ADS_TABLE", "social-media-local-ads"), CursorKey: os.Getenv("ADS_CURSOR_KEY"), CursorKeyID: value("ADS_CURSOR_KEY_ID", "v1"), CursorVersion: 1}
}

func value(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
