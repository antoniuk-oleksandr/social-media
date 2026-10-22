package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
)

type API interface {
	TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

type Store struct {
	Client                                 API
	UsersTable, UniquesTable, RefreshTable string
}

func (s Store) Register(ctx context.Context, r auth.Registration) error {
	u := r.User
	items := []types.TransactWriteItem{
		{Put: &types.Put{
			TableName: aws.String(s.UniquesTable),
			Item: map[string]types.AttributeValue{
				"uniqueKey": &types.AttributeValueMemberS{Value: "EMAIL#" + u.Email},
				"userId":    &types.AttributeValueMemberS{Value: u.ID},
			},
			ConditionExpression: aws.String("attribute_not_exists(uniqueKey)"),
		}},
		{Put: &types.Put{
			TableName: aws.String(s.UniquesTable),
			Item: map[string]types.AttributeValue{
				"uniqueKey": &types.AttributeValueMemberS{Value: "USERNAME#" + u.Username},
				"userId":    &types.AttributeValueMemberS{Value: u.ID},
			},
			ConditionExpression: aws.String("attribute_not_exists(uniqueKey)"),
		}},
		{Put: &types.Put{
			TableName: aws.String(s.UsersTable),
			Item: map[string]types.AttributeValue{
				"userId":       &types.AttributeValueMemberS{Value: u.ID},
				"username":     &types.AttributeValueMemberS{Value: u.Username},
				"email":        &types.AttributeValueMemberS{Value: u.Email},
				"passwordHash": &types.AttributeValueMemberS{Value: u.PasswordHash},
				"displayName":  &types.AttributeValueMemberS{Value: u.DisplayName},
				"role":         &types.AttributeValueMemberS{Value: string(u.Role)},
				"status":       &types.AttributeValueMemberS{Value: string(u.Status)},
				"createdAt":    &types.AttributeValueMemberS{Value: u.CreatedAt.UTC().Format(time.RFC3339)},
			},
			ConditionExpression: aws.String("attribute_not_exists(userId)"),
		}},
		{Put: &types.Put{
			TableName: aws.String(s.RefreshTable),
			Item: map[string]types.AttributeValue{
				"tokenHash": &types.AttributeValueMemberS{Value: r.Refresh.TokenHash},
				"sessionId": &types.AttributeValueMemberS{Value: r.Refresh.SessionID},
				"userId":    &types.AttributeValueMemberS{Value: r.Refresh.UserID},
				"status":    &types.AttributeValueMemberS{Value: "active"},
				"createdAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(r.Refresh.CreatedAt.Unix(), 10)},
				"expiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(r.Refresh.ExpiresAt.Unix(), 10)},
				"ttl":       &types.AttributeValueMemberN{Value: strconv.FormatInt(r.Refresh.ExpiresAt.Unix(), 10)},
				"version":   &types.AttributeValueMemberN{Value: "1"},
			},
			ConditionExpression: aws.String("attribute_not_exists(tokenHash)"),
		}},
	}

	_, err := s.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err == nil {
		return nil
	}

	if conflict, ok := cancellationConflict(err); ok {
		return conflict
	}
	if !isTransactionCanceled(err) {
		return wrapDependencyError("registration transaction", err)
	}

	conflict, lookupErr := s.lookupReservationConflict(ctx, u.Email, u.Username)
	if lookupErr != nil {
		return wrapDependencyError("registration conflict lookup", lookupErr)
	}
	if conflict == nil {
		return fmt.Errorf("registration transaction conflict is ambiguous")
	}
	return conflict
}

func wrapDependencyError(operation string, err error) error {
	if isDependencyUnavailable(err) {
		return fmt.Errorf("%w: %s", auth.ErrDependencyUnavailable, operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func isDependencyUnavailable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return true
	}
	var networkErr net.Error
	return errors.As(err, &networkErr)
}

func cancellationConflict(err error) (error, bool) {
	var canceled *types.TransactionCanceledException
	if !errors.As(err, &canceled) || len(canceled.CancellationReasons) < 2 {
		return nil, false
	}

	emailFailed := conditionalFailure(canceled.CancellationReasons[0])
	usernameFailed := conditionalFailure(canceled.CancellationReasons[1])
	if emailFailed == usernameFailed {
		return nil, false
	}
	if emailFailed {
		return auth.ErrEmailTaken, true
	}
	return auth.ErrUsernameTaken, true
}

func conditionalFailure(reason types.CancellationReason) bool {
	return reason.Code != nil && *reason.Code == "ConditionalCheckFailed"
}

func isTransactionCanceled(err error) bool {
	var canceled *types.TransactionCanceledException
	if errors.As(err, &canceled) {
		return true
	}
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "TransactionCanceledException"
}

func (s Store) lookupReservationConflict(ctx context.Context, email, username string) (error, error) {
	emailExists, err := s.reservationExists(ctx, "EMAIL#"+email)
	if err != nil {
		return nil, err
	}
	usernameExists, err := s.reservationExists(ctx, "USERNAME#"+username)
	if err != nil {
		return nil, err
	}
	if emailExists && usernameExists {
		// Both requested identifiers are already reserved. The public API exposes
		// one conflict code, so use a deterministic username-first precedence.
		return auth.ErrUsernameTaken, nil
	}
	if emailExists {
		return auth.ErrEmailTaken, nil
	}
	if usernameExists {
		return auth.ErrUsernameTaken, nil
	}
	return nil, nil
}

func (s Store) reservationExists(ctx context.Context, key string) (bool, error) {
	out, err := s.Client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.UniquesTable),
		Key:       map[string]types.AttributeValue{"uniqueKey": &types.AttributeValueMemberS{Value: key}},
	})
	if err != nil {
		return false, err
	}
	return len(out.Item) > 0, nil
}
