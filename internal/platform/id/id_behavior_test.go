package id

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("IsCanonicalUUIDv7", func() {
	DescribeTable("validates canonical UUIDv7 values", func(value string, valid bool) {
		Expect(IsCanonicalUUIDv7(value)).To(Equal(valid))
	},
		Entry("canonical", "0192f8a0-7b00-7000-8000-000000000001", true),
		Entry("uppercase", "0192F8A0-7B00-7000-8000-000000000001", false),
		Entry("wrong version", "0192f8a0-7b00-6000-8000-000000000001", false),
		Entry("malformed", "not-a-uuid", false),
	)
})
