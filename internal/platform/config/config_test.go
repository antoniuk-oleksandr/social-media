package config

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"os"
)

var _ = Describe("LoadUsesConfiguredOrDefaultRegion", func() {
	It("runs the test", func() {
		os.Unsetenv("AWS_REGION")
		cfg, err := Load()
		if err != nil || cfg.Region != "us-east-1" {
			Expect(false).To(BeTrue(), fmt.Sprintf("default region: cfg=%q err=%v", cfg.Region, err))
		}
		os.Setenv("AWS_REGION", "eu-west-1")
		defer os.Unsetenv("AWS_REGION")
		cfg, err = Load()
		if err != nil || cfg.Region != "eu-west-1" {
			Expect(false).To(BeTrue(), fmt.Sprintf("configured region: cfg=%q err=%v", cfg.Region, err))
		}

	})
})

var _ = Describe("LoadUsersConfig", func() {
	It("loads configured table names and preserves isolated defaults", func() {
		previousUsers, usersSet := os.LookupEnv("USERS_TABLE")
		previousUniques, uniquesSet := os.LookupEnv("UNIQUES_TABLE")
		Expect(os.Setenv("USERS_TABLE", "configured-users")).To(Succeed())
		Expect(os.Setenv("UNIQUES_TABLE", "configured-uniques")).To(Succeed())
		DeferCleanup(func() {
			if usersSet {
				Expect(os.Setenv("USERS_TABLE", previousUsers)).To(Succeed())
			} else {
				Expect(os.Unsetenv("USERS_TABLE")).To(Succeed())
			}
			if uniquesSet {
				Expect(os.Setenv("UNIQUES_TABLE", previousUniques)).To(Succeed())
			} else {
				Expect(os.Unsetenv("UNIQUES_TABLE")).To(Succeed())
			}
		})

		Expect(LoadUsersConfig()).To(Equal(UsersConfig{UsersTable: "configured-users", UniquesTable: "configured-uniques"}))
	})

	It("uses separate local defaults when table environment is absent", func() {
		Expect(os.Unsetenv("USERS_TABLE")).To(Succeed())
		Expect(os.Unsetenv("UNIQUES_TABLE")).To(Succeed())
		DeferCleanup(func() {
			Expect(os.Unsetenv("USERS_TABLE")).To(Succeed())
			Expect(os.Unsetenv("UNIQUES_TABLE")).To(Succeed())
		})

		Expect(LoadUsersConfig()).To(Equal(UsersConfig{UsersTable: "social-media-local-users", UniquesTable: "social-media-local-uniques"}))
	})
})
