package dynamodb

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type cursorPayload struct {
	Version       int    `json:"v"`
	KeyID         string `json:"kid"`
	UserID        string `json:"uid"`
	Status        string `json:"status"`
	Limit         int    `json:"limit"`
	IssuedAt      int64  `json:"iat"`
	ExpiresAt     int64  `json:"exp"`
	LastCreatedAt int64  `json:"lastCreatedAt"`
	LastAdID      string `json:"lastAdId"`
}

type cursorSigner struct {
	key     []byte
	keyID   string
	version int
	now     func() time.Time
	ttl     time.Duration
}

func newCursorSigner(cfg platformconfig.AdsConfig) cursorSigner {
	version := cfg.CursorVersion
	if version == 0 {
		version = 1
	}
	return cursorSigner{key: []byte(cfg.CursorKey), keyID: cfg.CursorKeyID, version: version, now: time.Now, ttl: 15 * time.Minute}
}

func (s cursorSigner) encode(status ad.Status, limit int, userID string, key map[string]types.AttributeValue) (string, error) {
	payload := cursorPayload{Version: s.version, KeyID: s.keyID, UserID: userID, Status: string(status), Limit: limit, IssuedAt: s.now().Unix(), ExpiresAt: s.now().Add(s.ttl).Unix()}
	for name, value := range key {
		switch value.(type) {
		case *types.AttributeValueMemberS, *types.AttributeValueMemberN:
		default:
			_ = name
			return "", ad.ErrInvalidCursor
		}
	}
	if item, ok := key["createdAtEpoch"].(*types.AttributeValueMemberN); ok {
		payload.LastCreatedAt, _ = strconv.ParseInt(item.Value, 10, 64)
		if id, ok := key["adId"].(*types.AttributeValueMemberS); ok {
			payload.LastAdID = id.Value
		}
	} else if item, ok := key["adId"].(*types.AttributeValueMemberS); ok {
		payload.LastAdID = item.Value
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", ad.ErrInvalidCursor
	}
	sig := hmac.New(sha256.New, s.key)
	_, _ = sig.Write(data)
	return base64.RawURLEncoding.EncodeToString(data) + "." + base64.RawURLEncoding.EncodeToString(sig.Sum(nil)), nil
}

func (s cursorSigner) decode(value string, query ad.ListQuery) (map[string]types.AttributeValue, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return nil, ad.ErrInvalidCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ad.ErrInvalidCursor
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ad.ErrInvalidCursor
	}
	sig := hmac.New(sha256.New, s.key)
	_, _ = sig.Write(data)
	if !hmac.Equal(provided, sig.Sum(nil)) {
		return nil, ad.ErrInvalidCursor
	}
	var payload cursorPayload
	if err := json.Unmarshal(data, &payload); err != nil || payload.Version != s.version || payload.KeyID != s.keyID || payload.UserID != query.UserID || payload.Status != string(query.Status) || payload.Limit != query.Limit || payload.IssuedAt <= 0 || payload.IssuedAt > s.now().Unix() || payload.LastAdID == "" {
		return nil, ad.ErrInvalidCursor
	}
	if payload.ExpiresAt <= s.now().Unix() {
		return nil, ad.ErrCursorExpired
	}
	key := map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: payload.LastAdID}}
	if payload.LastCreatedAt != 0 {
		key["status"] = &types.AttributeValueMemberS{Value: string(query.Status)}
		key["createdAtEpoch"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(payload.LastCreatedAt, 10)}
	}
	return key, nil
}

type API interface {
	PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}
type Repository struct {
	Client    API
	TableName string
	cursor    cursorSigner
}

func NewRepository(client *dynamodb.Client, tableName string) ad.Repository {
	return &Repository{Client: client, TableName: tableName}
}
func NewConfiguredRepository(client *dynamodb.Client, cfg platformconfig.AdsConfig) ad.Repository {
	return &Repository{Client: client, TableName: cfg.AdsTable, cursor: newCursorSigner(cfg)}
}
func (r *Repository) signer() cursorSigner {
	if r.cursor.now == nil {
		return newCursorSigner(platformconfig.AdsConfig{})
	}
	return r.cursor
}
func (r *Repository) Create(ctx context.Context, value ad.Advertisement) error {
	_, err := r.Client.PutItem(ctx, putInput(r.TableName, value, "attribute_not_exists(adId)"))
	if err != nil {
		return ad.ErrDependencyUnavailable
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, value ad.Advertisement) error {
	_, err := r.Client.PutItem(ctx, putInput(r.TableName, value, "attribute_exists(adId)"))
	if err != nil {
		var conditionErr *types.ConditionalCheckFailedException
		if errors.As(err, &conditionErr) {
			return ad.ErrNotFound
		}
		return ad.ErrDependencyUnavailable
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:           aws.String(r.TableName),
		Key:                 map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: id}},
		ConditionExpression: aws.String("attribute_exists(adId)"),
	})
	if err != nil {
		var conditionErr *types.ConditionalCheckFailedException
		if errors.As(err, &conditionErr) {
			return ad.ErrNotFound
		}
		return ad.ErrDependencyUnavailable
	}
	return nil
}

func putInput(tableName string, value ad.Advertisement, condition string) *dynamodb.PutItemInput {
	return &dynamodb.PutItemInput{TableName: aws.String(tableName), Item: map[string]types.AttributeValue{
		"adId": &types.AttributeValueMemberS{Value: value.ID}, "entityType": &types.AttributeValueMemberS{Value: value.Type}, "advertiser": &types.AttributeValueMemberS{Value: value.Advertiser}, "content": &types.AttributeValueMemberS{Value: value.Content}, "targetUrl": &types.AttributeValueMemberS{Value: value.TargetURL}, "startsAt": &types.AttributeValueMemberS{Value: value.StartsAt.Format(time.RFC3339Nano)}, "endsAt": &types.AttributeValueMemberS{Value: value.EndsAt.Format(time.RFC3339Nano)}, "startsAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(value.StartsAt.UnixMilli())}, "endsAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(value.EndsAt.UnixMilli())}, "status": &types.AttributeValueMemberS{Value: string(value.Status)}, "createdBy": &types.AttributeValueMemberS{Value: value.CreatedBy}, "createdAt": &types.AttributeValueMemberS{Value: value.CreatedAt.Format(time.RFC3339Nano)}, "createdAtEpoch": &types.AttributeValueMemberN{Value: fmt.Sprint(value.CreatedAtEpoch)},
	}, ConditionExpression: aws.String(condition)}
}

func (r *Repository) List(ctx context.Context, query ad.ListQuery) (ad.ListResult, error) {
	var boundary map[string]types.AttributeValue
	var err error
	if query.Cursor != "" {
		boundary, err = r.signer().decode(query.Cursor, query)
		if err != nil {
			return ad.ListResult{}, err
		}
	}
	items, err := r.queryAll(ctx, query.Status)
	if err != nil {
		return ad.ListResult{}, err
	}
	result := ad.ListResult{Items: items}
	sort.SliceStable(result.Items, func(i, j int) bool {
		if result.Items[i].CreatedAtEpoch != result.Items[j].CreatedAtEpoch {
			return result.Items[i].CreatedAtEpoch > result.Items[j].CreatedAtEpoch
		}
		return result.Items[i].ID > result.Items[j].ID
	})
	start := 0
	if boundary != nil {
		boundaryEpoch, ok := boundary["createdAtEpoch"].(*types.AttributeValueMemberN)
		if !ok {
			return ad.ListResult{}, ad.ErrInvalidCursor
		}
		boundaryID, ok := boundary["adId"].(*types.AttributeValueMemberS)
		if !ok {
			return ad.ListResult{}, ad.ErrInvalidCursor
		}
		epoch, parseErr := strconv.ParseInt(boundaryEpoch.Value, 10, 64)
		if parseErr != nil {
			return ad.ListResult{}, ad.ErrInvalidCursor
		}
		for start < len(result.Items) && (result.Items[start].CreatedAtEpoch > epoch || (result.Items[start].CreatedAtEpoch == epoch && result.Items[start].ID >= boundaryID.Value)) {
			start++
		}
	}
	if start > 0 {
		result.Items = result.Items[start:]
	}
	result.HasMore = len(result.Items) > query.Limit
	if result.HasMore {
		page := result.Items[:query.Limit]
		result.Items = page
		last := page[len(page)-1]
		result.NextCursor, err = r.signer().encode(query.Status, query.Limit, query.UserID, map[string]types.AttributeValue{
			"status":         &types.AttributeValueMemberS{Value: string(query.Status)},
			"createdAtEpoch": &types.AttributeValueMemberN{Value: strconv.FormatInt(last.CreatedAtEpoch, 10)},
			"adId":           &types.AttributeValueMemberS{Value: last.ID},
		})
		if err != nil {
			return ad.ListResult{}, err
		}
	}
	return result, nil
}

// queryAll deliberately consumes every DynamoDB page before applying the
// application order. ActiveAdsIndex sorts only by createdAtEpoch, so using a
// user-page Limit plus a raw ExclusiveStartKey cannot define the ID tie-breaker
// required by the API. The cursor therefore stores the semantic (epoch, ID)
// boundary, while ExclusiveStartKey is used only for internal query pages.
func (r *Repository) queryAll(ctx context.Context, status ad.Status) ([]ad.Advertisement, error) {
	items := make([]ad.Advertisement, 0)
	var startKey map[string]types.AttributeValue
	for {
		output, err := r.Client.Query(ctx, &dynamodb.QueryInput{
			TableName: aws.String(r.TableName), IndexName: aws.String("ActiveAdsIndex"),
			KeyConditionExpression:    aws.String("#status = :status"),
			ExpressionAttributeNames:  map[string]string{"#status": "status"},
			ExpressionAttributeValues: map[string]types.AttributeValue{":status": &types.AttributeValueMemberS{Value: string(status)}},
			ScanIndexForward:          aws.Bool(false), ExclusiveStartKey: startKey,
		})
		if err != nil {
			return nil, ad.ErrDependencyUnavailable
		}
		for _, item := range output.Items {
			value, mapErr := mapListItem(item)
			if mapErr != nil {
				return nil, ad.ErrDependencyUnavailable
			}
			items = append(items, value)
		}
		if len(output.LastEvaluatedKey) == 0 {
			return items, nil
		}
		startKey = output.LastEvaluatedKey
	}
}

func mapListItem(item map[string]types.AttributeValue) (ad.Advertisement, error) {
	get := func(name string) (string, error) {
		value, ok := item[name].(*types.AttributeValueMemberS)
		if !ok {
			return "", errors.New("invalid advertisement record")
		}
		return value.Value, nil
	}
	getNum := func(name string) (int64, error) {
		value, ok := item[name].(*types.AttributeValueMemberN)
		if !ok {
			return 0, errors.New("invalid advertisement record")
		}
		var number int64
		if _, err := fmt.Sscan(value.Value, &number); err != nil {
			return 0, err
		}
		return number, nil
	}
	id, err := get("adId")
	if err != nil {
		return ad.Advertisement{}, err
	}
	typeValue, err := get("entityType")
	if err != nil {
		return ad.Advertisement{}, err
	}
	advertiser, err := get("advertiser")
	if err != nil {
		return ad.Advertisement{}, err
	}
	content, err := get("content")
	if err != nil {
		return ad.Advertisement{}, err
	}
	target, err := get("targetUrl")
	if err != nil {
		return ad.Advertisement{}, err
	}
	startsValue, err := get("startsAt")
	if err != nil {
		return ad.Advertisement{}, err
	}
	endsValue, err := get("endsAt")
	if err != nil {
		return ad.Advertisement{}, err
	}
	status, err := get("status")
	if err != nil {
		return ad.Advertisement{}, err
	}
	createdBy, err := get("createdBy")
	if err != nil {
		return ad.Advertisement{}, err
	}
	createdValue, err := get("createdAt")
	if err != nil {
		return ad.Advertisement{}, err
	}
	starts, err := time.Parse(time.RFC3339Nano, startsValue)
	if err != nil {
		return ad.Advertisement{}, err
	}
	ends, err := time.Parse(time.RFC3339Nano, endsValue)
	if err != nil {
		return ad.Advertisement{}, err
	}
	created, err := time.Parse(time.RFC3339Nano, createdValue)
	if err != nil {
		return ad.Advertisement{}, err
	}
	createdEpoch, err := getNum("createdAtEpoch")
	if err != nil {
		return ad.Advertisement{}, err
	}
	return ad.Advertisement{ID: id, Type: typeValue, Advertiser: advertiser, Content: content, TargetURL: target, StartsAt: starts, EndsAt: ends, Status: ad.Status(status), CreatedBy: createdBy, CreatedAt: created, CreatedAtEpoch: createdEpoch}, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (ad.Advertisement, error) {
	output, err := r.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(r.TableName), Key: map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: id}}})
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	if len(output.Item) == 0 {
		return ad.Advertisement{}, ad.ErrNotFound
	}
	item := output.Item
	getString := func(name string) (string, error) {
		value, ok := item[name].(*types.AttributeValueMemberS)
		if !ok {
			return "", fmt.Errorf("missing string attribute %s", name)
		}
		return value.Value, nil
	}
	getNumber := func(name string) (int64, error) {
		value, ok := item[name].(*types.AttributeValueMemberN)
		if !ok {
			return 0, fmt.Errorf("missing number attribute %s", name)
		}
		var result int64
		if _, err := fmt.Sscan(value.Value, &result); err != nil {
			return 0, err
		}
		return result, nil
	}
	idValue, err := getString("adId")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	typeValue, err := getString("entityType")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	advertiser, err := getString("advertiser")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	content, err := getString("content")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	targetURL, err := getString("targetUrl")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	startsAt, err := getString("startsAt")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	endsAt, err := getString("endsAt")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	status, err := getString("status")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	createdBy, err := getString("createdBy")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	createdAt, err := getString("createdAt")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	starts, err := time.Parse(time.RFC3339Nano, startsAt)
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	ends, err := time.Parse(time.RFC3339Nano, endsAt)
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	createdEpoch, err := getNumber("createdAtEpoch")
	if err != nil {
		return ad.Advertisement{}, ad.ErrDependencyUnavailable
	}
	return ad.Advertisement{ID: idValue, Type: typeValue, Advertiser: advertiser, Content: content, TargetURL: targetURL, StartsAt: starts, EndsAt: ends, Status: ad.Status(status), CreatedBy: createdBy, CreatedAt: created, CreatedAtEpoch: createdEpoch}, nil
}
