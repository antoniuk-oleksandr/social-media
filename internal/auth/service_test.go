package auth_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"time"
)

type store struct {
	r auth.Registration
	e error
}

func (s *store) Register(_ context.Context, r auth.Registration) error { s.r = r; return s.e }

type hasher struct{}

func (hasher) Hash(string) (string, error) { return "hash", nil }

type ids struct{ n int }

func (i *ids) New() (string, error) { i.n++; return time.Unix(int64(i.n), 0).Format("150405"), nil }

type clk struct{}

func (clk) Now() time.Time { return time.Unix(100, 0) }

type access struct{}

func (access) Issue(context.Context, user.User, time.Time) (string, error) { return "access", nil }

type opaque struct{}

func (opaque) Generate() (string, string, error) { return "refresh", "hash", nil }
func valid() auth.RegisterCommand {
	return auth.RegisterCommand{Username: "alice", Email: " A@Example.COM ", Password: "123456789012", DisplayName: "Alice"}
}

var _ = Describe("RegisterNormalizesAndPersistsAtomicPayload", func() {
	It("runs the test", func() {
		s := &store{}
		svc := auth.Service{Store: s, Hasher: hasher{}, IDs: &ids{}, Clock: clk{}, Access: access{}, Opaque: opaque{}, AccessTTL: 15 * time.Minute, RefreshTTL: 7 * 24 * time.Hour}
		got, e := svc.Register(context.Background(), valid())
		if e != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(e))
		}
		if got.User.Email != "a@example.com" || got.User.Role != user.RoleUser || s.r.User.PasswordHash != "hash" || s.r.Refresh.TokenHash != "hash" {
			Expect(false).To(BeTrue(), fmt.Sprintf("bad registration: %#v %#v", got, s.r))
		}
		if got.Tokens.ExpiresIn != 900 || got.Tokens.RefreshExpiresIn != 604800 {
			Expect(false).To(BeTrue(), fmt.Sprint(got.Tokens))
		}

	})
})
var _ = Describe("RegisterValidationPreventsPersistence", func() {
	It("runs the test", func() {
		s := &store{}
		svc := auth.Service{Store: s, Hasher: hasher{}, IDs: &ids{}, Clock: clk{}, Access: access{}, Opaque: opaque{}}
		_, e := svc.Register(context.Background(), auth.RegisterCommand{Username: "AB", Email: "bad", Password: "short", DisplayName: ""})
		if !errors.Is(e, auth.ErrValidation) {
			Expect(false).To(BeTrue(), fmt.Sprintf("%v", e))
		}
		if s.r.User.ID != "" {
			Expect(false).To(BeTrue(), fmt.Sprint("persisted invalid registration"))
		}

	})
})
