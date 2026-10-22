package clock

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("RealNowIsUTCAndCurrent", func() {
	It("runs the test", func() {
		before := time.Now().UTC()
		got := (Real{}).Now()
		after := time.Now().UTC()
		if got.Location() != time.UTC || got.Before(before) || got.After(after) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v", got))
		}

	})
})
var _ = Describe("FixedNowReturnsConfiguredTime", func() {
	It("runs the test", func() {
		want := time.Date(2026, 10, 19, 1, 2, 3, 4, time.FixedZone("x", 3600))
		if got := (Fixed{T: want}).Now(); !got.Equal(want) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v", got))
		}

	})
})
