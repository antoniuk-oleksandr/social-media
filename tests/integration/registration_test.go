//go:build integration

package integration

import (
	"context"
	"errors"
	"strconv"
	"time"

	authdomain "github.com/antoniuk-oleksandr/social-media/internal/auth"
	authdb "github.com/antoniuk-oleksandr/social-media/internal/auth/dynamodb"
	"github.com/antoniuk-oleksandr/social-media/tests/fixtures"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("registration persistence", func() {
	It("persists every user, reservation, and refresh-session attribute with String keys", func() {
		r := fixtures.Registration("integration-registration-complete", "integration_registration", "registration-complete@example.com")
		Expect(integrationStore().Register(context.Background(), r)).To(Succeed())
		assertStringFields(integrationSuite.UsersTable, "userId", r.User.ID, map[string]string{
			"userId": r.User.ID, "username": r.User.Username, "email": r.User.Email,
			"passwordHash": r.User.PasswordHash, "displayName": r.User.DisplayName,
			"role": string(r.User.Role), "status": string(r.User.Status),
			"createdAt": r.User.CreatedAt.UTC().Format(time.RFC3339),
		})
		for _, key := range []string{"EMAIL#" + r.User.Email, "USERNAME#" + r.User.Username} {
			assertStringFields(integrationSuite.UniquesTable, "uniqueKey", key, map[string]string{
				"uniqueKey": key, "userId": r.User.ID,
			})
		}
		assertStringFields(integrationSuite.RefreshTable, "tokenHash", r.Refresh.TokenHash, map[string]string{
			"tokenHash": r.Refresh.TokenHash, "sessionId": r.Refresh.SessionID, "userId": r.Refresh.UserID, "status": "active",
		})
		assertNumericFields(integrationSuite.RefreshTable, "tokenHash", r.Refresh.TokenHash, map[string]string{
			"createdAt": strconv.FormatInt(r.Refresh.CreatedAt.Unix(), 10),
			"expiresAt": strconv.FormatInt(r.Refresh.ExpiresAt.Unix(), 10),
			"ttl":       strconv.FormatInt(r.Refresh.ExpiresAt.Unix(), 10), "version": "1",
		})
	})

	It("rejects duplicate email without partial writes", func() {
		first := fixtures.Registration("integration-duplicate-email-owner", "duplicate_email_owner", "duplicate@example.com")
		second := fixtures.Registration("integration-duplicate-email-rejected", "duplicate_email_rejected", first.User.Email)
		register(first)
		err := integrationStore().Register(context.Background(), second)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, authdomain.ErrEmailTaken)).To(BeTrue())
		assertRejectedRegistrationAbsent(second, first, "email")
		assertRegistrationPresent(first)
	})

	It("rejects duplicate username without partial writes", func() {
		first := fixtures.Registration("integration-duplicate-username-owner", "duplicate_username", "duplicate-username-owner@example.com")
		second := fixtures.Registration("integration-duplicate-username-rejected", first.User.Username, "duplicate-username-rejected@example.com")
		register(first)
		err := integrationStore().Register(context.Background(), second)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, authdomain.ErrUsernameTaken)).To(BeTrue())
		assertRejectedRegistrationAbsent(second, first, "username")
		assertRegistrationPresent(first)
	})

	It("translates a dependency timeout and does not write a registration", func() {
		registration := fixtures.Registration("integration-registration-timeout", "registration_timeout", "registration-timeout@example.com")
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		err := integrationStore().Register(ctx, registration)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, authdomain.ErrDependencyUnavailable)).To(BeTrue())
		assertRegistrationAbsent(registration)
	})
})

func integrationStore() authdb.Store {
	return authdb.Store{Client: integrationSuite.Client, UsersTable: integrationSuite.UsersTable, UniquesTable: integrationSuite.UniquesTable, RefreshTable: integrationSuite.RefreshTable}
}
func register(r authdomain.Registration) {
	ExpectWithOffset(1, integrationStore().Register(context.Background(), r)).To(Succeed())
}

func assertRegistrationPresent(r authdomain.Registration) {
	assertStringFields(integrationSuite.UsersTable, "userId", r.User.ID, map[string]string{"userId": r.User.ID, "email": r.User.Email, "username": r.User.Username})
	assertStringFields(integrationSuite.UniquesTable, "uniqueKey", "EMAIL#"+r.User.Email, map[string]string{"userId": r.User.ID})
	assertStringFields(integrationSuite.UniquesTable, "uniqueKey", "USERNAME#"+r.User.Username, map[string]string{"userId": r.User.ID})
	assertStringFields(integrationSuite.RefreshTable, "tokenHash", r.Refresh.TokenHash, map[string]string{"userId": r.User.ID, "status": "active"})
}

func assertRegistrationAbsent(r authdomain.Registration) {
	assertMissing(integrationSuite.UsersTable, "userId", r.User.ID)
	assertMissing(integrationSuite.UniquesTable, "uniqueKey", "EMAIL#"+r.User.Email)
	assertMissing(integrationSuite.UniquesTable, "uniqueKey", "USERNAME#"+r.User.Username)
	assertMissing(integrationSuite.RefreshTable, "tokenHash", r.Refresh.TokenHash)
}

func assertRejectedRegistrationAbsent(rejected, owner authdomain.Registration, duplicateField string) {
	assertMissing(integrationSuite.UsersTable, "userId", rejected.User.ID)
	assertMissing(integrationSuite.RefreshTable, "tokenHash", rejected.Refresh.TokenHash)

	switch duplicateField {
	case "email":
		assertStringFields(integrationSuite.UniquesTable, "uniqueKey", "EMAIL#"+owner.User.Email, map[string]string{"userId": owner.User.ID})
		assertMissing(integrationSuite.UniquesTable, "uniqueKey", "USERNAME#"+rejected.User.Username)
	case "username":
		assertStringFields(integrationSuite.UniquesTable, "uniqueKey", "USERNAME#"+owner.User.Username, map[string]string{"userId": owner.User.ID})
		assertMissing(integrationSuite.UniquesTable, "uniqueKey", "EMAIL#"+rejected.User.Email)
	default:
		Fail("unsupported duplicate field: " + duplicateField)
	}
}

func assertStringFields(table, keyName, keyValue string, expected map[string]string) {
	out, err := integrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String(table), Key: map[string]types.AttributeValue{keyName: &types.AttributeValueMemberS{Value: keyValue}}})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, out.Item).NotTo(BeEmpty())
	for name, want := range expected {
		value, ok := out.Item[name].(*types.AttributeValueMemberS)
		ExpectWithOffset(1, ok).To(BeTrue(), name)
		if !ok {
			return
		}
		ExpectWithOffset(1, value.Value).To(Equal(want), name)
	}
}
func assertNumericFields(table, keyName, keyValue string, expected map[string]string) {
	out, err := integrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String(table), Key: map[string]types.AttributeValue{keyName: &types.AttributeValueMemberS{Value: keyValue}}})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, out.Item).NotTo(BeEmpty())
	for name, want := range expected {
		value, ok := out.Item[name].(*types.AttributeValueMemberN)
		ExpectWithOffset(1, ok).To(BeTrue(), name)
		if !ok {
			return
		}
		ExpectWithOffset(1, value.Value).To(Equal(want), name)
	}
}
func assertMissing(table, keyName, keyValue string) {
	out, err := integrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String(table), Key: map[string]types.AttributeValue{keyName: &types.AttributeValueMemberS{Value: keyValue}}})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, out.Item).To(BeEmpty())
}
