package fx

import (
	"net/http"
	"os"

	platformclock "github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	platformjwt "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	userfeature "github.com/antoniuk-oleksandr/social-media/internal/user"
	userdb "github.com/antoniuk-oleksandr/social-media/internal/user/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("User feature constructors", func() {
	It("constructs the repository, service, handler, and router", func() {
		previous := os.Getenv("USERS_TABLE")
		Expect(os.Setenv("USERS_TABLE", "users-test")).To(Succeed())
		DeferCleanup(func() { Expect(os.Setenv("USERS_TABLE", previous)).To(Succeed()) })

		client := dynamodb.New(dynamodb.Options{})
		repository := NewRepository(client)
		_, ok := repository.(*userdb.Repository)
		Expect(ok).To(BeTrue(), "repository type = %T, want *dynamodb.Repository", repository)
		service := NewService(repository)
		Expect(service).NotTo(BeNil())
		handler := NewHandler(service, nil)
		Expect(handler).NotTo(BeNil())
		var router http.Handler = NewRouter(handler, platformjwt.Access{})
		Expect(router).NotTo(BeNil())
		var _ userfeature.Repository = repository
		var _ userfeature.Service = service
	})

	It("constructs a repository from explicit users configuration", func() {
		repository := NewConfiguredRepository(dynamodb.New(dynamodb.Options{}), platformconfig.UsersConfig{UsersTable: "users", UniquesTable: "uniques"}, platformclock.Real{})
		Expect(repository).NotTo(BeNil())
	})
})
