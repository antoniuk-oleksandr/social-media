package user

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("User domain", func() {
	It("defines the public role and active status constants", func() {
		now := time.Now()
		value := User{ID: "0192f8a0-7b00-7000-8000-000000000001", Username: "name", Email: "e", Role: RoleUser, Status: StatusActive, CreatedAt: now}

		Expect(value.Role).To(Equal(Role("user")))
		Expect(value.Status).To(Equal(Status("active")))
		Expect(value.CreatedAt).To(Equal(now))
	})
})
