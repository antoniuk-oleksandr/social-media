package fx

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFeedFX(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Feed Fx suite")
}
