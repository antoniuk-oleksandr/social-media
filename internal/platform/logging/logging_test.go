package logging

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Zap logger", func() {
	It("constructs and consumes logging and sync operations", func() {
		logger, err := New()
		Expect(err).NotTo(HaveOccurred())
		Expect(logger.Info("test message", String("component", "test"))).To(Succeed())
		Expect(logger.Error("test error", Error(assertionError{}))).To(Succeed())
		Expect(logger.Sync()).To(Succeed())
	})
})

type assertionError struct{}

func (assertionError) Error() string { return "test error" }
