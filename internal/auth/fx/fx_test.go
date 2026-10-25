package fx

import (
	platformclock "github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	platformjwt "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	platformlogging "github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	platformpassword "github.com/antoniuk-oleksandr/social-media/internal/platform/password"
	platformtoken "github.com/antoniuk-oleksandr/social-media/internal/platform/token"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"time"
)

var _ = Describe("Constructors", func() {
	It("builds the auth graph with UUIDv7 IDs", func() {
		client := dynamodb.New(dynamodb.Options{})
		store := NewRegistrationStore(client)
		loginStore := NewLoginStore(client)
		logoutStore := NewLogoutStore(client)
		refreshStore := NewRefreshStore(client)
		users := NewUserLookup(client)
		service := NewService(store, loginStore, logoutStore, refreshStore, users, platformpassword.Bcrypt{Cost: 4}, platformid.UUIDv7{}, platformclock.Real{}, platformjwt.Access{}, platformtoken.Opaque{})
		Expect(service).NotTo(BeNil())
		port := NewServicePort(store, loginStore, logoutStore, refreshStore, users, platformpassword.Bcrypt{Cost: 4}, platformid.UUIDv7{}, platformclock.Real{}, platformjwt.Access{}, platformtoken.Opaque{})
		Expect(port).NotTo(BeNil())
		Expect(service.AccessTTL).To(Equal(15 * time.Minute))
		Expect(service.RefreshTTL).To(Equal(7 * 24 * time.Hour))
		logger, err := platformlogging.New()
		Expect(err).NotTo(HaveOccurred())
		var router http.Handler = NewRouter(NewHandler(service, logger))
		Expect(router).NotTo(BeNil())
	})
})
