package user

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("UserDomainConstantsAndFields", func() {
	It("runs the test", func() {
		now := time.Now()
		u := User{ID: "id", Username: "name", Email: "e", Role: RoleUser, Status: StatusActive, CreatedAt: now}
		if u.Role != "user" || u.Status != "active" || !u.CreatedAt.Equal(now) {
			Expect(false).To(BeTrue(), fmt.Sprintf("%+v", u))
		}

	})
})
