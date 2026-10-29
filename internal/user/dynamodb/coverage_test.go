package dynamodb

import (
	"testing"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/clock"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestConfiguredRepositoryWithClockAndUsernameConflict(t *testing.T) {
	fixed := clock.Fixed{T: time.Unix(0, 0)}
	repository, ok := NewConfiguredRepositoryWithClock(nil, "users", "uniques", fixed).(*Repository)
	if !ok || repository.TableName != "users" || repository.UniqueTableName != "uniques" || repository.Clock != fixed {
		t.Fatalf("repository = %#v", repository)
	}
	code := "ConditionalCheckFailed"
	if !isUsernameConflict(&types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{{}, {Code: &code}}}) {
		t.Fatal("conditional transaction failure was not recognized")
	}
	other := "ProvisionedThroughputExceeded"
	if isUsernameConflict(&types.TransactionCanceledException{CancellationReasons: []types.CancellationReason{{}, {Code: &other}}}) {
		t.Fatal("non-conflict transaction failure was recognized as a conflict")
	}
	if isUsernameConflict(nil) {
		t.Fatal("nil error was recognized as a conflict")
	}
}
