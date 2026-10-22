//go:build integration

package authintegration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	authdomain "github.com/antoniuk-oleksandr/social-media/internal/auth"
	authdb "github.com/antoniuk-oleksandr/social-media/internal/auth/dynamodb"
	"github.com/antoniuk-oleksandr/social-media/tests/fixtures"
	"github.com/antoniuk-oleksandr/social-media/tests/integration/support"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var integrationSuite *support.DynamoDBSuite

func TestAuthIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Auth integration suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var err error
	integrationSuite, err = support.StartDynamoDB(ctx, "tests-auth-registration")
	Expect(err).NotTo(HaveOccurred(), "DynamoDB integration suite must start")
})

var _ = AfterSuite(func() {
	if integrationSuite != nil {
		Expect(integrationSuite.Close(context.Background())).To(Succeed())
	}
})

var _ = Describe("registration persistence", func() {
	It("persists the user, uniqueness reservations, and refresh session", func() {
		registration := fixtures.Registration("integration-persisted-user", "integration_persisted", "persisted@example.com")

		Expect(integrationStore().Register(context.Background(), registration)).To(Succeed())

		assertStringFields(integrationSuite.UsersTable, "userId", registration.User.ID, map[string]string{
			"username":     registration.User.Username,
			"email":        registration.User.Email,
			"passwordHash": registration.User.PasswordHash,
			"displayName":  registration.User.DisplayName,
			"role":         string(registration.User.Role),
			"status":       string(registration.User.Status),
			"createdAt":    registration.User.CreatedAt.UTC().Format(time.RFC3339),
		})
		assertStringFields(integrationSuite.UniquesTable, "uniqueKey", "EMAIL#"+registration.User.Email, map[string]string{
			"userId": registration.User.ID,
		})
		assertStringFields(integrationSuite.UniquesTable, "uniqueKey", "USERNAME#"+registration.User.Username, map[string]string{
			"userId": registration.User.ID,
		})
		assertStringFields(integrationSuite.RefreshTable, "tokenHash", registration.Refresh.TokenHash, map[string]string{
			"sessionId": registration.Refresh.SessionID,
			"userId":    registration.Refresh.UserID,
			"status":    "active",
		})
		assertNumericFields(integrationSuite.RefreshTable, "tokenHash", registration.Refresh.TokenHash, map[string]string{
			"createdAt": fmt.Sprint(registration.Refresh.CreatedAt.Unix()),
			"expiresAt": fmt.Sprint(registration.Refresh.ExpiresAt.Unix()),
			"ttl":       fmt.Sprint(registration.Refresh.ExpiresAt.Unix()),
			"version":   "1",
		})
	})

	It("rejects duplicate email without partial writes", func() {
		first := fixtures.Registration("integration-email-owner", "integration_email_owner", "integration-duplicate@example.com")
		second := fixtures.Registration("integration-email-conflict", "integration_email_conflict", first.User.Email)
		register(first)

		err := integrationStore().Register(context.Background(), second)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, authdomain.ErrEmailTaken)).To(BeTrue())

		assertMissing(integrationSuite.UsersTable, "userId", second.User.ID)
		assertMissing(integrationSuite.UniquesTable, "uniqueKey", "USERNAME#"+second.User.Username)
		assertMissing(integrationSuite.RefreshTable, "tokenHash", second.Refresh.TokenHash)
		assertStringFields(integrationSuite.UsersTable, "userId", first.User.ID, map[string]string{
			"username": first.User.Username,
			"email":    first.User.Email,
		})
		assertStringFields(integrationSuite.UniquesTable, "uniqueKey", "EMAIL#"+first.User.Email, map[string]string{
			"userId": first.User.ID,
		})
		assertStringFields(integrationSuite.UniquesTable, "uniqueKey", "USERNAME#"+first.User.Username, map[string]string{
			"userId": first.User.ID,
		})
		assertStringFields(integrationSuite.RefreshTable, "tokenHash", first.Refresh.TokenHash, map[string]string{
			"userId": first.User.ID,
			"status": "active",
		})
	})

	It("rejects duplicate username without partial writes", func() {
		first := fixtures.Registration("integration-username-owner", "integration_duplicate_username", "integration-username-owner@example.com")
		second := fixtures.Registration("integration-username-conflict", first.User.Username, "integration-username-conflict@example.com")
		register(first)

		err := integrationStore().Register(context.Background(), second)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, authdomain.ErrUsernameTaken)).To(BeTrue())

		assertMissing(integrationSuite.UsersTable, "userId", second.User.ID)
		assertMissing(integrationSuite.UniquesTable, "uniqueKey", "EMAIL#"+second.User.Email)
		assertMissing(integrationSuite.RefreshTable, "tokenHash", second.Refresh.TokenHash)
		assertStringFields(integrationSuite.UsersTable, "userId", first.User.ID, map[string]string{
			"username": first.User.Username,
			"email":    first.User.Email,
		})
		assertStringFields(integrationSuite.UniquesTable, "uniqueKey", "EMAIL#"+first.User.Email, map[string]string{
			"userId": first.User.ID,
		})
		assertStringFields(integrationSuite.UniquesTable, "uniqueKey", "USERNAME#"+first.User.Username, map[string]string{
			"userId": first.User.ID,
		})
		assertStringFields(integrationSuite.RefreshTable, "tokenHash", first.Refresh.TokenHash, map[string]string{
			"userId": first.User.ID,
			"status": "active",
		})
	})

	It("translates a dependency timeout", func() {
		registration := fixtures.Registration("integration-timeout-user", "integration_timeout", "integration-timeout@example.com")
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		err := integrationStore().Register(ctx, registration)
		Expect(errors.Is(err, authdomain.ErrDependencyUnavailable)).To(BeTrue())
	})
})

func integrationStore() authdb.Store {
	return authdb.Store{
		Client:       integrationSuite.Client,
		UsersTable:   integrationSuite.UsersTable,
		UniquesTable: integrationSuite.UniquesTable,
		RefreshTable: integrationSuite.RefreshTable,
	}
}

func register(registration authdomain.Registration) {
	ExpectWithOffset(1, integrationStore().Register(context.Background(), registration)).To(Succeed())
}

func assertStringFields(table, keyName, keyValue string, expected map[string]string) {
	out, err := integrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(table),
		Key:       map[string]types.AttributeValue{keyName: &types.AttributeValueMemberS{Value: keyValue}},
	})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, out.Item).NotTo(BeEmpty())
	for name, want := range expected {
		value, ok := out.Item[name].(*types.AttributeValueMemberS)
		if !ok {
			Fail(fmt.Sprintf("item %q field %q should be a string", keyValue, name), 1)
			return
		}
		ExpectWithOffset(1, value.Value).To(Equal(want), "item %q field %q", keyValue, name)
	}
}

func assertMissing(table, keyName, keyValue string) {
	out, err := integrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(table),
		Key:       map[string]types.AttributeValue{keyName: &types.AttributeValueMemberS{Value: keyValue}},
	})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, out.Item).To(BeEmpty())
}

func assertNumericFields(table, keyName, keyValue string, expected map[string]string) {
	out, err := integrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(table),
		Key:       map[string]types.AttributeValue{keyName: &types.AttributeValueMemberS{Value: keyValue}},
	})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, out.Item).NotTo(BeEmpty())
	for name, want := range expected {
		value, ok := out.Item[name].(*types.AttributeValueMemberN)
		if !ok {
			Fail(fmt.Sprintf("item %q field %q should be numeric", keyValue, name), 1)
			return
		}
		ExpectWithOffset(1, value.Value).To(Equal(want), "item %q field %q", keyValue, name)
	}
}
