package logging

import (
	"errors"
	"io"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _ = Describe("Error field", func() {
	It("represents nil and non-nil errors safely", func() {
		Expect(Error(nil).Value).To(Equal("<nil>"))
		Expect(Error(errors.New("boom")).Value).To(Equal("boom"))
	})
})

var _ = Describe("Zap logger", func() {
	It("constructs and consumes logging and sync operations", func() {
		logger, err := New()
		Expect(err).NotTo(HaveOccurred())
		Expect(logger.Info("test message", String("component", "test"))).To(Succeed())
		Expect(logger.Error("test error", Error(assertionError{}))).To(Succeed())
		Expect(logger.Sync()).To(Succeed())
	})

	It("returns non-benign sync failures", func() {
		want := errors.New("sync failed")
		core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), failingSyncer{err: want}, zap.InfoLevel)
		logger := zapLogger{logger: zap.New(core)}

		Expect(logger.Sync()).To(MatchError(want))
	})
})

type assertionError struct{}

func (assertionError) Error() string { return "test error" }

type failingSyncer struct{ err error }

func (failingSyncer) Write(value []byte) (int, error) { return io.Discard.Write(value) }
func (f failingSyncer) Sync() error                   { return f.err }
