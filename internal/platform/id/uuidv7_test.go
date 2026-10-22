package id

import (
	"fmt"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UUIDv7NewReturnsValidVersionSevenUUID", func() {
	It("runs the test", func() {
		s, e := (UUIDv7{}).New()
		if e != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(e))
		}
		u, e := uuid.Parse(s)
		if e != nil || u.Version() != 7 {
			Expect(false).To(BeTrue(), fmt.Sprintf("%s %v", s, e))
		}

	})
})
