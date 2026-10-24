package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
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
	Clock                                  auth.Clock
}

type queryAPI interface {
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

const familyMarkerPrefix = "FAMILY#"

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
		// Preserve the domain classification while retaining the provider error
		// for structured diagnostics. The HTTP layer exposes only the domain
		// error, but logs can now identify missing tables, denied actions, timeouts,
		// and other infrastructure causes.
		return fmt.Errorf("%w: %s: %w", auth.ErrDependencyUnavailable, operation, err)
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

func (s Store) FindByLogin(ctx context.Context, login string) (user.User, error) {
	q, ok := s.Client.(queryAPI)
	if !ok {
		return user.User{}, auth.ErrDependencyUnavailable
	}
	index, field := "EmailIndex", "email"
	if !strings.Contains(login, "@") {
		index, field = "UsernameIndex", "username"
	}
	out, err := q.Query(ctx, &dynamodb.QueryInput{
		TableName: aws.String(s.UsersTable), IndexName: aws.String(index),
		KeyConditionExpression:    aws.String("#login = :login"),
		ExpressionAttributeNames:  map[string]string{"#login": field},
		ExpressionAttributeValues: map[string]types.AttributeValue{":login": &types.AttributeValueMemberS{Value: login}}, Limit: aws.Int32(1),
	})
	if err != nil {
		return user.User{}, wrapDependencyError("find user", err)
	}
	if len(out.Items) == 0 {
		return user.User{}, user.ErrNotFound
	}
	item := out.Items[0]
	created, err := time.Parse(time.RFC3339, attributeString(item, "createdAt"))
	if err != nil {
		return user.User{}, fmt.Errorf("decode user: %w", err)
	}
	return user.User{ID: attributeString(item, "userId"), Username: attributeString(item, "username"), Email: attributeString(item, "email"), PasswordHash: attributeString(item, "passwordHash"), DisplayName: attributeString(item, "displayName"), Role: user.Role(attributeString(item, "role")), Status: user.Status(attributeString(item, "status")), CreatedAt: created}, nil
}

func (s Store) FindByID(ctx context.Context, id string) (user.User, error) {
	out, err := s.Client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.UsersTable),
		Key:            map[string]types.AttributeValue{"userId": &types.AttributeValueMemberS{Value: id}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return user.User{}, wrapDependencyError("find user", err)
	}
	if len(out.Item) == 0 {
		return user.User{}, user.ErrNotFound
	}
	created, err := time.Parse(time.RFC3339, attributeString(out.Item, "createdAt"))
	if err != nil {
		return user.User{}, fmt.Errorf("decode user: %w", err)
	}
	return user.User{ID: attributeString(out.Item, "userId"), Username: attributeString(out.Item, "username"), Email: attributeString(out.Item, "email"), PasswordHash: attributeString(out.Item, "passwordHash"), DisplayName: attributeString(out.Item, "displayName"), Role: user.Role(attributeString(out.Item, "role")), Status: user.Status(attributeString(out.Item, "status")), CreatedAt: created}, nil
}

func (s Store) CreateLoginSession(ctx context.Context, r auth.RefreshSession) error {
	_, err := s.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{ConditionCheck: &types.ConditionCheck{
			TableName:           aws.String(s.RefreshTable),
			Key:                 map[string]types.AttributeValue{"tokenHash": &types.AttributeValueMemberS{Value: familyMarkerPrefix + r.SessionID}},
			ConditionExpression: aws.String("attribute_not_exists(tokenHash)"),
		}},
		{Put: &types.Put{TableName: aws.String(s.RefreshTable), Item: map[string]types.AttributeValue{
			"tokenHash": &types.AttributeValueMemberS{Value: r.TokenHash}, "sessionId": &types.AttributeValueMemberS{Value: r.SessionID}, "userId": &types.AttributeValueMemberS{Value: r.UserID}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(r.CreatedAt.Unix(), 10)}, "expiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(r.ExpiresAt.Unix(), 10)}, "ttl": &types.AttributeValueMemberN{Value: strconv.FormatInt(r.ExpiresAt.Unix(), 10)}, "version": &types.AttributeValueMemberN{Value: "1"},
		}, ConditionExpression: aws.String("attribute_not_exists(tokenHash)")}},
	}})
	if err != nil {
		return wrapDependencyError("create login session", err)
	}
	return nil
}

func (s Store) RevokeSessionFamily(ctx context.Context, tokenHash string) error {
	session, err := s.GetRefreshSession(ctx, tokenHash)
	if err != nil {
		return err
	}

	now := s.now()
	_, err = s.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{ConditionCheck: &types.ConditionCheck{
			TableName: aws.String(s.RefreshTable),
			Key:       map[string]types.AttributeValue{"tokenHash": &types.AttributeValueMemberS{Value: tokenHash}},
			ConditionExpression: aws.String(
				"#status = :active AND expiresAt > :now",
			),
			ExpressionAttributeNames: map[string]string{"#status": "status"},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":active": &types.AttributeValueMemberS{Value: "active"},
				":now":    &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Unix(), 10)},
			},
		}},
		{Put: &types.Put{
			TableName: aws.String(s.RefreshTable),
			Item: map[string]types.AttributeValue{
				"tokenHash": &types.AttributeValueMemberS{Value: familyMarkerPrefix + session.SessionID},
				"sessionId": &types.AttributeValueMemberS{Value: session.SessionID},
				"status":    &types.AttributeValueMemberS{Value: "revoked"},
				"ttl":       &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(7*24*time.Hour).Unix(), 10)},
			},
			ConditionExpression:       aws.String("attribute_not_exists(tokenHash) OR #status = :active"),
			ExpressionAttributeNames:  map[string]string{"#status": "status"},
			ExpressionAttributeValues: map[string]types.AttributeValue{":active": &types.AttributeValueMemberS{Value: "active"}},
		}},
	}})
	if err != nil {
		if isConditionalTransactionFailure(err) {
			return auth.ErrInvalidRefreshToken
		}
		return wrapDependencyError("revoke refresh session family", err)
	}
	return nil
}

func isConditionalTransactionFailure(err error) bool {
	var canceled *types.TransactionCanceledException
	if !errors.As(err, &canceled) {
		return false
	}
	for _, reason := range canceled.CancellationReasons {
		if conditionalFailure(reason) {
			return true
		}
	}
	return false
}

// GetRefreshSession is the authoritative refresh lookup. It reads both the
// token and its family marker strongly so a logout cannot be hidden by index
// or eventually-consistent read lag.
func (s Store) GetRefreshSession(ctx context.Context, tokenHash string) (auth.RefreshSession, error) {
	item, err := s.Client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.RefreshTable),
		Key:            map[string]types.AttributeValue{"tokenHash": &types.AttributeValueMemberS{Value: tokenHash}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return auth.RefreshSession{}, wrapDependencyError("find refresh session", err)
	}
	if len(item.Item) == 0 ||
		attributeString(item.Item, "tokenHash") != tokenHash ||
		attributeString(item.Item, "sessionId") == "" ||
		attributeString(item.Item, "userId") == "" ||
		attributeString(item.Item, "status") != "active" {
		return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
	}
	createdAt, ok := attributeTime(item.Item, "createdAt")
	if !ok {
		return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
	}
	expiresAt, ok := attributeTime(item.Item, "expiresAt")
	if !ok || !expiresAt.After(s.now()) {
		return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
	}
	sessionID := attributeString(item.Item, "sessionId")
	marker, err := s.Client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.RefreshTable),
		Key:            map[string]types.AttributeValue{"tokenHash": &types.AttributeValueMemberS{Value: familyMarkerPrefix + sessionID}},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return auth.RefreshSession{}, wrapDependencyError("check refresh session family", err)
	}
	if len(marker.Item) > 0 {
		if attributeString(marker.Item, "sessionId") != sessionID {
			return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
		}
		switch attributeString(marker.Item, "status") {
		case "active":
		case "revoked":
			return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
		default:
			return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
		}
	}
	return auth.RefreshSession{
		TokenHash: tokenHash,
		SessionID: sessionID,
		UserID:    attributeString(item.Item, "userId"),
		Status:    attributeString(item.Item, "status"),
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
	}, nil
}

func (s Store) RotateRefreshSession(ctx context.Context, oldHash string, next auth.RefreshSession) error {
	now := s.now()
	_, err := s.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{ConditionCheck: &types.ConditionCheck{
			TableName: aws.String(s.RefreshTable),
			Key: map[string]types.AttributeValue{
				"tokenHash": &types.AttributeValueMemberS{Value: familyMarkerPrefix + next.SessionID},
			},
			ConditionExpression: aws.String("attribute_not_exists(tokenHash)"),
		}},
		{Update: &types.Update{TableName: aws.String(s.RefreshTable), Key: map[string]types.AttributeValue{"tokenHash": &types.AttributeValueMemberS{Value: oldHash}}, UpdateExpression: aws.String("SET #status = :consumed, consumedAt = :now, version = version + :one"), ConditionExpression: aws.String("#status = :active AND expiresAt > :now"), ExpressionAttributeNames: map[string]string{"#status": "status"}, ExpressionAttributeValues: map[string]types.AttributeValue{":active": &types.AttributeValueMemberS{Value: "active"}, ":consumed": &types.AttributeValueMemberS{Value: "consumed"}, ":now": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Unix(), 10)}, ":one": &types.AttributeValueMemberN{Value: "1"}}}},
		{Put: &types.Put{TableName: aws.String(s.RefreshTable), Item: map[string]types.AttributeValue{"tokenHash": &types.AttributeValueMemberS{Value: next.TokenHash}, "sessionId": &types.AttributeValueMemberS{Value: next.SessionID}, "userId": &types.AttributeValueMemberS{Value: next.UserID}, "status": &types.AttributeValueMemberS{Value: "active"}, "createdAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(next.CreatedAt.Unix(), 10)}, "expiresAt": &types.AttributeValueMemberN{Value: strconv.FormatInt(next.ExpiresAt.Unix(), 10)}, "ttl": &types.AttributeValueMemberN{Value: strconv.FormatInt(next.ExpiresAt.Unix(), 10)}, "version": &types.AttributeValueMemberN{Value: "1"}}, ConditionExpression: aws.String("attribute_not_exists(tokenHash)")}},
	}})
	if err != nil {
		if isConditionalTransactionFailure(err) {
			return auth.ErrInvalidRefreshToken
		}
		return wrapDependencyError("rotate refresh session", err)
	}
	return nil
}

func (s Store) RevokeSessionFamilyByHash(ctx context.Context, tokenHash string) error {
	item, err := s.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(s.RefreshTable), Key: map[string]types.AttributeValue{"tokenHash": &types.AttributeValueMemberS{Value: tokenHash}}, ConsistentRead: aws.Bool(true)})
	if err != nil {
		return wrapDependencyError("find refresh replay", err)
	}
	sessionID := attributeString(item.Item, "sessionId")
	if sessionID == "" {
		return auth.ErrInvalidRefreshToken
	}
	now := s.now()
	_, err = s.Client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{{Put: &types.Put{TableName: aws.String(s.RefreshTable), Item: map[string]types.AttributeValue{"tokenHash": &types.AttributeValueMemberS{Value: familyMarkerPrefix + sessionID}, "sessionId": &types.AttributeValueMemberS{Value: sessionID}, "status": &types.AttributeValueMemberS{Value: "revoked"}, "ttl": &types.AttributeValueMemberN{Value: strconv.FormatInt(now.Add(7*24*time.Hour).Unix(), 10)}}, ConditionExpression: aws.String("attribute_not_exists(tokenHash) OR #status = :active"), ExpressionAttributeNames: map[string]string{"#status": "status"}, ExpressionAttributeValues: map[string]types.AttributeValue{":active": &types.AttributeValueMemberS{Value: "active"}}}}}})
	if err != nil && !isConditionalTransactionFailure(err) {
		return wrapDependencyError("revoke refresh replay", err)
	}
	return nil
}

func (s Store) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now().UTC()
	}
	return time.Now().UTC()
}

func attributeTime(item map[string]types.AttributeValue, key string) (time.Time, bool) {
	value, ok := item[key].(*types.AttributeValueMemberN)
	if !ok {
		return time.Time{}, false
	}
	seconds, err := strconv.ParseInt(value.Value, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0).UTC(), true
}

func attributeString(item map[string]types.AttributeValue, key string) string {
	if value, ok := item[key].(*types.AttributeValueMemberS); ok {
		return value.Value
	}
	return ""
}
