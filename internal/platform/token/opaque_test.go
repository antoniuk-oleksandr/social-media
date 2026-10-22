package token

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("OpaqueGenerateReturnsVerifiableSecretAndHash", func() {
	It("runs the test", func() {
		p, h, e := (Opaque{}).Generate()
		if e != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(e))
		}
		b, e := base64.RawURLEncoding.DecodeString(p)
		if e != nil || len(b) != 32 {
			Expect(false).To(BeTrue(), fmt.Sprintf("payload: %v len=%d", e, len(b)))
		}
		sum := sha256.Sum256([]byte(p))
		if h != base64.RawURLEncoding.EncodeToString(sum[:]) {
			Expect(false).To(BeTrue(), fmt.Sprint("hash mismatch"))
		}
		p2, _, e := (Opaque{}).Generate()
		if e != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(e))
		}
		if p == p2 {
			Expect(false).To(BeTrue(), fmt.Sprint("tokens repeated"))
		}

	})
})
