package user

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("user ID parsing", func() {
	DescribeTable("accepts only canonical UUIDv7 IDs",
		func(input string, valid bool) {
			Expect(validUUIDv7(input)).To(Equal(valid))
		},
		Entry("canonical UUIDv7", "0192f8a0-7b00-7000-8000-000000000001", true),
		Entry("missing", "", false),
		Entry("wrong version", "0192f8a0-7b00-6000-8000-000000000001", false),
		Entry("uppercase", "0192F8A0-7B00-7000-8000-000000000001", false),
		Entry("non-uuid", "abc", false),
	)
})
