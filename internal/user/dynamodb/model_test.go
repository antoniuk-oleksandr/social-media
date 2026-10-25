package dynamodb

import (
	"errors"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DynamoDB user model mapping", func() {
	It("maps a canonical UUIDv7 record", func() {
		got, err := toDomain(UserRecord{ID: "0192f8a0-7b00-7000-8000-000000000001", Username: "alice", Role: "user", Status: "active", CreatedAt: "2026-10-24T12:00:00Z"})
		Expect(err).NotTo(HaveOccurred())
		Expect(got.ID).To(Equal("0192f8a0-7b00-7000-8000-000000000001"))
	})
	DescribeTable("rejects invalid persisted records", func(record UserRecord) {
		_, err := toDomain(record)
		Expect(err).To(MatchError(user.ErrDependencyUnavailable))
		Expect(errors.Is(err, user.ErrNotFound)).To(BeFalse())
	}, Entry("missing ID", UserRecord{CreatedAt: "2026-10-24T12:00:00Z"}), Entry("wrong version", UserRecord{ID: "0192f8a0-7b00-6000-8000-000000000001", CreatedAt: "2026-10-24T12:00:00Z"}), Entry("malformed timestamp", UserRecord{ID: "0192f8a0-7b00-7000-8000-000000000001", CreatedAt: "not-a-time"}))
})
