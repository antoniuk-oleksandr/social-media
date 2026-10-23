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
		service := NewService(store, platformpassword.Bcrypt{Cost: 4}, platformid.UUIDv7{}, platformclock.Real{}, platformjwt.Access{}, platformtoken.Opaque{})
		if service.Store != store || service.AccessTTL != 15*time.Minute || service.RefreshTTL != 7*24*time.Hour {
			Expect(false).To(BeTrue(), fmt.Sprintf("unexpected service: %#v", service))
		}
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
		service := NewServicePort(store, platformpassword.Bcrypt{Cost: 4}, platformid.UUIDv7{}, platformclock.Real{}, platformjwt.Access{}, platformtoken.Opaque{})
		if service == nil {
			Expect(false).To(BeTrue(), fmt.Sprint("service port is nil"))
		}

	})
})
