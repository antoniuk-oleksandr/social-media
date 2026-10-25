package user

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type profileRepository struct {
	value   User
	command UpdateProfileCommand
	calls   int
}

func (r *profileRepository) GetByID(context.Context, string) (User, error) {
	return r.value, nil
}

func (r *profileRepository) UpdateProfile(_ context.Context, command UpdateProfileCommand) (User, error) {
	r.calls++
	r.command = command
	return r.value, nil
}

var _ = Describe("User profile service", func() {
	It("lowercases usernames before persistence", func() {
		repository := &profileRepository{value: User{ID: profileID(), Username: "alice_smith", CreatedAt: time.Now()}}
		username := "Alice_Smith"
		_, err := NewService(repository).UpdateProfile(context.Background(), UpdateProfileCommand{UserID: profileID(), Username: &username})

		Expect(err).NotTo(HaveOccurred())
		Expect(repository.command.Username).NotTo(BeNil())
		Expect(*repository.command.Username).To(Equal("alice_smith"))
	})

	It("rejects an invalid authenticated user ID before persistence", func() {
		repository := &profileRepository{}
		_, err := NewService(repository).UpdateProfile(context.Background(), UpdateProfileCommand{
			UserID:      "not-a-uuid",
			DisplayName: stringPtr("Alice"),
		})

		Expect(err).To(MatchError(ErrInvalidID))
		Expect(repository.calls).To(BeZero())
	})

	It("forwards a valid display-name-only update", func() {
		repository := &profileRepository{value: User{ID: profileID(), DisplayName: "Alice Smith"}}
		_, err := NewService(repository).UpdateProfile(context.Background(), UpdateProfileCommand{
			UserID:      profileID(),
			DisplayName: stringPtr("Alice Smith"),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(repository.calls).To(Equal(1))
		Expect(repository.command.Username).To(BeNil())
		Expect(repository.command.DisplayName).NotTo(BeNil())
		Expect(*repository.command.DisplayName).To(Equal("Alice Smith"))
	})

	DescribeTable("rejects invalid profile commands before persistence", func(command UpdateProfileCommand) {
		repository := &profileRepository{}
		_, err := NewService(repository).UpdateProfile(context.Background(), command)

		Expect(err).To(MatchError(ErrValidation))
		Expect(repository.calls).To(BeZero())
	},
		Entry("no fields", UpdateProfileCommand{UserID: profileID()}),
		Entry("short username", UpdateProfileCommand{UserID: profileID(), Username: stringPtr("ab")}),
		Entry("invalid username characters", UpdateProfileCommand{UserID: profileID(), Username: stringPtr("alice-name")}),
		Entry("empty display name", UpdateProfileCommand{UserID: profileID(), DisplayName: stringPtr("")}),
	)
})

func stringPtr(value string) *string { return &value }
