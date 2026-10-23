package auth_test

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
)

var _ = Describe("HashRefreshToken", func() {
	It("runs the test", func() {
		const token = "refresh-token"
		sum := sha256.Sum256([]byte(token))
		if got, want := auth.HashRefreshToken(token), base64.RawURLEncoding.EncodeToString(sum[:]); got != want {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %q, want %q", got, want))
		}

	})
})
