package user

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

func newProfileRepository(ctrl *gomock.Controller, value User) (*MockRepository, *UpdateProfileCommand, *int) {
	repository := NewMockRepository(ctrl)
	var command UpdateProfileCommand
	calls := 0
	repository.EXPECT().GetByID(gomock.Any(), gomock.Any()).AnyTimes().Return(value, nil)
	repository.EXPECT().UpdateProfile(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, update UpdateProfileCommand) (User, error) {
		calls++
		command = update
		return value, nil
	})
	return repository, &command, &calls
}

var _ = Describe("User profile service", func() {
	It("lowercases usernames before persistence", func() {
		ctrl := gomock.NewController(GinkgoT())
		repository, command, _ := newProfileRepository(ctrl, User{ID: profileID(), Username: "alice_smith", CreatedAt: time.Now()})
		username := "Alice_Smith"
		_, err := NewService(repository).UpdateProfile(context.Background(), UpdateProfileCommand{UserID: profileID(), Username: &username})

		Expect(err).NotTo(HaveOccurred())
		Expect(command.Username).NotTo(BeNil())
		Expect(*command.Username).To(Equal("alice_smith"))
	})

	It("rejects an invalid authenticated user ID before persistence", func() {
		ctrl := gomock.NewController(GinkgoT())
		repository, _, calls := newProfileRepository(ctrl, User{})
		_, err := NewService(repository).UpdateProfile(context.Background(), UpdateProfileCommand{
			UserID:      "not-a-uuid",
			DisplayName: stringPtr("Alice"),
		})

		Expect(err).To(MatchError(ErrInvalidID))
		Expect(*calls).To(BeZero())
	})

	It("forwards a valid display-name-only update", func() {
		ctrl := gomock.NewController(GinkgoT())
		repository, command, calls := newProfileRepository(ctrl, User{ID: profileID(), DisplayName: "Alice Smith"})
		_, err := NewService(repository).UpdateProfile(context.Background(), UpdateProfileCommand{
			UserID:      profileID(),
			DisplayName: stringPtr("Alice Smith"),
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(*calls).To(Equal(1))
		Expect(command.Username).To(BeNil())
		Expect(command.DisplayName).NotTo(BeNil())
		Expect(*command.DisplayName).To(Equal("Alice Smith"))
	})

	DescribeTable("rejects invalid profile commands before persistence", func(command UpdateProfileCommand) {
		ctrl := gomock.NewController(GinkgoT())
		repository, _, calls := newProfileRepository(ctrl, User{})
		_, err := NewService(repository).UpdateProfile(context.Background(), command)

		Expect(err).To(MatchError(ErrValidation))
		Expect(*calls).To(BeZero())
	},
		Entry("no fields", UpdateProfileCommand{UserID: profileID()}),
		Entry("short username", UpdateProfileCommand{UserID: profileID(), Username: stringPtr("ab")}),
		Entry("invalid username characters", UpdateProfileCommand{UserID: profileID(), Username: stringPtr("alice-name")}),
		Entry("empty display name", UpdateProfileCommand{UserID: profileID(), DisplayName: stringPtr("")}),
	)
})

func stringPtr(value string) *string { return &value }
