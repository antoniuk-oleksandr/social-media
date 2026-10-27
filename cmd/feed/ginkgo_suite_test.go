package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFeedCommandGinkgo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Feed command suite")
}
