package password

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/bcrypt"
)

var _ = Describe("BcryptHashUsesConfiguredCost", func() {
	It("runs the test", func() {
		h, e := (Bcrypt{Cost: 4}).Hash("secret")
		if e != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(e))
		}
		if bcrypt.CompareHashAndPassword([]byte(h), []byte("secret")) != nil {
			Expect(false).To(BeTrue(), fmt.Sprint("password did not verify"))
		}
		c, e := bcrypt.Cost([]byte(h))
		if e != nil || c != 4 {
			Expect(false).To(BeTrue(), fmt.Sprintf("cost=%d err=%v", c, e))
		}

	})
})
var _ = Describe("BcryptDefaultCostHashes", func() {
	It("runs the test", func() {
		h, e := (Bcrypt{}).Hash("secret")
		if e != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(e))
		}
		if bcrypt.CompareHashAndPassword([]byte(h), []byte("secret")) != nil {
			Expect(false).To(BeTrue(), fmt.Sprint("password did not verify"))
		}
		c, e := bcrypt.Cost([]byte(h))
		if e != nil || c != 12 {
			Expect(false).To(BeTrue(), fmt.Sprintf("cost=%d err=%v, want 12", c, e))
		}

	})
})
