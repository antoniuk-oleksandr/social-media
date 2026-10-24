package fx

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	platformclock "github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
	platformjwt "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	platformlogging "github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	platformpassword "github.com/antoniuk-oleksandr/social-media/internal/platform/password"
	platformtoken "github.com/antoniuk-oleksandr/social-media/internal/platform/token"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

var _ = Describe("Constructors", func() {
	It("runs the test", func() {
		client := dynamodb.New(dynamodb.Options{})
		store := NewRegistrationStore(client)
		loginStore := NewLoginStore(client)
		logoutStore := NewLogoutStore(client)
		refreshStore := NewRefreshStore(client)
		users := NewUserLookup(client)
		service := NewService(store, loginStore, logoutStore, refreshStore, users, platformpassword.Bcrypt{Cost: 4}, platformid.UUIDv7{}, platformclock.Real{}, platformjwt.Access{}, platformtoken.Opaque{})
		Expect(service.Store).To(Equal(store))
		Expect(service.RefreshStore).To(Equal(refreshStore))
		Expect(service.Users).To(Equal(users))
		Expect(service.AccessTTL).To(Equal(15 * time.Minute))
		Expect(service.RefreshTTL).To(Equal(7 * 24 * time.Hour))
		logger, err := platformlogging.New()
		Expect(err).NotTo(HaveOccurred())
		handler := NewHandler(service, logger)
		var router http.Handler = NewRouter(handler)
		if router == nil {
			Expect(false).To(BeTrue(), fmt.Sprint("router is nil"))
		}
		var _ auth.RegistrationStore = store

	})
})

var _ = Describe("NewServicePortReturnsConfiguredService", func() {
	It("runs the test", func() {
		client := dynamodb.New(dynamodb.Options{})
		store := NewRegistrationStore(client)
		loginStore := NewLoginStore(client)
		logoutStore := NewLogoutStore(client)
		refreshStore := NewRefreshStore(client)
		users := NewUserLookup(client)
		service := NewServicePort(store, loginStore, logoutStore, refreshStore, users, platformpassword.Bcrypt{Cost: 4}, platformid.UUIDv7{}, platformclock.Real{}, platformjwt.Access{}, platformtoken.Opaque{})
		if service == nil {
			Expect(false).To(BeTrue(), fmt.Sprint("service port is nil"))
		}

	})
})
