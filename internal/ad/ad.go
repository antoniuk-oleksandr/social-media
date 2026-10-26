package ad

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	platformid "github.com/antoniuk-oleksandr/social-media/internal/platform/id"
)

var (
	ErrValidation            = errors.New("invalid advertisement")
	ErrInvalidID             = errors.New("invalid advertisement id")
	ErrNotFound              = errors.New("advertisement not found")
	ErrInvalidState          = errors.New("invalid advertisement state transition")
	ErrForbidden             = errors.New("administrator role required")
	ErrDependencyUnavailable = errors.New("dependency unavailable")
)

type Status string

const (
	StatusDraft   Status = "draft"
	StatusActive  Status = "active"
	StatusPaused  Status = "paused"
	StatusExpired Status = "expired"
)

type Advertisement struct {
	ID             string
	Type           string
	Advertiser     string
	Content        string
	TargetURL      string
	StartsAt       time.Time
	EndsAt         time.Time
	Status         Status
	CreatedBy      string
	CreatedAt      time.Time
	CreatedAtEpoch int64
}

type CreateCommand struct {
	AdminID    string
	Advertiser string
	Content    string
	TargetURL  string
	StartsAt   time.Time
	EndsAt     time.Time
	Status     Status
}

type UpdateCommand struct {
	AdID       string
	Advertiser *string
	Content    *string
	TargetURL  *string
	StartsAt   *time.Time
	EndsAt     *time.Time
	Status     *Status
}

type Repository interface {
	Create(context.Context, Advertisement) error
	GetByID(context.Context, string) (Advertisement, error)
	Update(context.Context, Advertisement) error
}
type IDGenerator interface{ New() (string, error) }
type Clock interface{ Now() time.Time }

type Service interface {
	Create(context.Context, CreateCommand) (Advertisement, error)
	GetByID(context.Context, string) (Advertisement, error)
	Update(context.Context, UpdateCommand) (Advertisement, error)
}

type service struct {
	repository Repository
	ids        IDGenerator
	clock      Clock
}

func NewService(repository Repository, ids IDGenerator, clock Clock) Service {
	return &service{repository: repository, ids: ids, clock: clock}
}

func (s *service) Create(ctx context.Context, command CreateCommand) (Advertisement, error) {
	if !platformid.IsCanonicalUUIDv7(command.AdminID) || !validText(command.Advertiser, 1, 200) || !validText(command.Content, 1, 1000) {
		return Advertisement{}, ErrValidation
	}
	if !validHTTPSURL(command.TargetURL) || command.StartsAt.IsZero() || command.EndsAt.IsZero() || !command.StartsAt.Before(command.EndsAt) {
		return Advertisement{}, ErrValidation
	}
	if !validStatus(command.Status) || (command.Status == StatusActive && (s.clock.Now().Before(command.StartsAt) || s.clock.Now().After(command.EndsAt))) || (command.Status == StatusExpired && !s.clock.Now().After(command.EndsAt)) {
		return Advertisement{}, ErrValidation
	}
	id, err := s.ids.New()
	if err != nil || !platformid.IsCanonicalUUIDv7(id) {
		return Advertisement{}, ErrDependencyUnavailable
	}
	createdAt := s.clock.Now().UTC().Truncate(time.Millisecond)
	value := Advertisement{ID: id, Type: "ad", Advertiser: strings.TrimSpace(command.Advertiser), Content: strings.TrimSpace(command.Content), TargetURL: command.TargetURL, StartsAt: command.StartsAt.UTC(), EndsAt: command.EndsAt.UTC(), Status: command.Status, CreatedBy: command.AdminID, CreatedAt: createdAt, CreatedAtEpoch: createdAt.UnixMilli()}
	if err := s.repository.Create(ctx, value); err != nil {
		return Advertisement{}, err
	}
	return value, nil
}

func (s *service) GetByID(ctx context.Context, id string) (Advertisement, error) {
	if !platformid.IsCanonicalUUIDv7(id) {
		return Advertisement{}, ErrInvalidID
	}
	return s.repository.GetByID(ctx, id)
}

func (s *service) Update(ctx context.Context, command UpdateCommand) (Advertisement, error) {
	if !platformid.IsCanonicalUUIDv7(command.AdID) {
		return Advertisement{}, ErrInvalidID
	}
	if command.Advertiser == nil && command.Content == nil && command.TargetURL == nil && command.StartsAt == nil && command.EndsAt == nil && command.Status == nil {
		return Advertisement{}, ErrValidation
	}
	value, err := s.repository.GetByID(ctx, command.AdID)
	if err != nil {
		return Advertisement{}, err
	}
	previousStatus := value.Status
	if command.Advertiser != nil {
		value.Advertiser = strings.TrimSpace(*command.Advertiser)
	}
	if command.Content != nil {
		value.Content = strings.TrimSpace(*command.Content)
	}
	if command.TargetURL != nil {
		value.TargetURL = *command.TargetURL
	}
	if command.StartsAt != nil {
		value.StartsAt = command.StartsAt.UTC()
	}
	if command.EndsAt != nil {
		value.EndsAt = command.EndsAt.UTC()
	}
	if command.Status != nil {
		value.Status = *command.Status
	}
	if !validStatus(value.Status) || !validText(value.Advertiser, 1, 200) || !validText(value.Content, 1, 1000) || !validHTTPSURL(value.TargetURL) || value.StartsAt.IsZero() || value.EndsAt.IsZero() || !value.StartsAt.Before(value.EndsAt) {
		return Advertisement{}, ErrValidation
	}
	if command.Status != nil && !validTransition(previousStatus, value.Status) {
		return Advertisement{}, ErrInvalidState
	}
	now := s.clock.Now()
	if value.Status == StatusActive && (now.Before(value.StartsAt) || now.After(value.EndsAt)) {
		return Advertisement{}, ErrValidation
	}
	if value.Status == StatusExpired && !now.After(value.EndsAt) {
		return Advertisement{}, ErrValidation
	}
	if err := s.repository.Update(ctx, value); err != nil {
		return Advertisement{}, err
	}
	return value, nil
}

func validText(value string, min, max int) bool {
	n := len([]rune(strings.TrimSpace(value)))
	return n >= min && n <= max
}
func validStatus(value Status) bool {
	return value == StatusDraft || value == StatusActive || value == StatusPaused || value == StatusExpired
}
func validTransition(from, to Status) bool {
	if from == to {
		return true
	}
	switch from {
	case StatusDraft:
		return to == StatusActive
	case StatusActive:
		return to == StatusPaused || to == StatusExpired
	case StatusPaused:
		return to == StatusActive || to == StatusExpired
	default:
		return false
	}
}

func validHTTPSURL(value string) bool {
	if len(value) > 2048 || strings.TrimSpace(value) != value {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	return validHostname(u.Hostname())
}

func validHostname(hostname string) bool {
	if net.ParseIP(hostname) != nil {
		return true
	}
	if len(hostname) == 0 || len(hostname) > 253 {
		return false
	}

	labels := strings.Split(hostname, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			character := label[i]
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}
