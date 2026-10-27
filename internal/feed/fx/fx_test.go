package fx

import (
	platformclock "github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	platformjwt "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("feed Fx constructors", func() {
	It("builds the feed dependency graph components", func() {
		client := (*dynamodb.Client)(nil)
		postsConfig := platformconfig.PostsConfig{PostsTable: "posts"}
		adsConfig := platformconfig.AdsConfig{AdsTable: "ads", CursorKey: "key", CursorKeyID: "v1"}
		usersConfig := platformconfig.UsersConfig{UsersTable: "users", UniquesTable: "uniques"}

		posts := NewPosts(client, postsConfig)
		ads := NewAds(client, adsConfig)
		authors := NewAuthors(client, usersConfig)
		Expect(posts).NotTo(BeNil())
		Expect(ads).NotTo(BeNil())
		Expect(authors).NotTo(BeNil())

		service := NewService(posts, ads, authors, platformclock.Real{}, adsConfig)
		Expect(service).NotTo(BeNil())
		handler := NewHandler(service, nil)
		Expect(handler).NotTo(BeNil())
		router := NewRouter(handler, platformjwt.Access{})
		Expect(router).NotTo(BeNil())
	})
})
