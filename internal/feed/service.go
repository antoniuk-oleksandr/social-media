package feed

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"sort"
	"strings"
	"time"
)

type cursor struct {
	Version    int      `json:"v"`
	KeyID      string   `json:"kid"`
	UserID     string   `json:"uid"`
	Snapshot   int64    `json:"snapshot"`
	LastEpoch  int64    `json:"lastEpoch"`
	LastPostID string   `json:"lastPostId"`
	Organic    int      `json:"organic"`
	Seed       int      `json:"seed"`
	Shown      []string `json:"shown"`
	Limit      int      `json:"limit"`
	Issued     int64    `json:"iat"`
	Expires    int64    `json:"exp"`
}

func (s *service) sign(c cursor) (string, error) {
	b, e := json.Marshal(c)
	if e != nil {
		return "", ErrInvalidCursor
	}
	h := hmac.New(sha256.New, s.key)
	_, e = h.Write(b)
	if e != nil {
		return "", ErrInvalidCursor
	}
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil)), nil
}
func (s *service) decode(value string, q Query) (cursor, error) {
	if len(value) > 4096 {
		return cursor{}, ErrInvalidCursor
	}
	p := strings.Split(value, ".")
	if len(p) != 2 {
		return cursor{}, ErrInvalidCursor
	}
	b, e := base64.RawURLEncoding.DecodeString(p[0])
	if e != nil {
		return cursor{}, ErrInvalidCursor
	}
	sig, e := base64.RawURLEncoding.DecodeString(p[1])
	if e != nil {
		return cursor{}, ErrInvalidCursor
	}
	h := hmac.New(sha256.New, s.key)
	_, e = h.Write(b)
	if e != nil || !hmac.Equal(sig, h.Sum(nil)) {
		return cursor{}, ErrInvalidCursor
	}
	var c cursor
	if json.Unmarshal(b, &c) != nil || c.Version != 1 || c.KeyID != s.keyID || c.UserID != q.UserID || c.Limit != q.Limit || c.Snapshot <= 0 || c.LastPostID == "" {
		return cursor{}, ErrInvalidCursor
	}
	if c.Expires <= s.clock.Now().Unix() {
		return cursor{}, ErrCursorExpired
	}
	return c, nil
}
func (s *service) Build(ctx context.Context, q Query) (Result, error) {
	if q.Limit == 0 {
		if q.LimitSet {
			return Result{}, ErrInvalidQuery
		}
		q.Limit = 20
	}
	if q.Limit < 1 || q.Limit > 50 {
		return Result{}, ErrInvalidQuery
	}
	now := s.clock.Now().UTC()
	c := cursor{Version: 1, KeyID: s.keyID, UserID: q.UserID, Limit: q.Limit, Snapshot: now.UnixMilli(), Issued: now.Unix()}
	if q.Cursor != "" {
		var err error
		c, err = s.decode(q.Cursor, q)
		if err != nil {
			return Result{}, err
		}
	}
	posts, err := s.posts.List(ctx, time.UnixMilli(c.Snapshot))
	if err != nil {
		return Result{}, ErrDependencyUnavailable
	}
	sort.Slice(posts, func(i, j int) bool {
		if posts[i].CreatedAtEpoch != posts[j].CreatedAtEpoch {
			return posts[i].CreatedAtEpoch > posts[j].CreatedAtEpoch
		}
		return posts[i].ID > posts[j].ID
	})
	start := 0
	if q.Cursor != "" {
		for start < len(posts) && (posts[start].CreatedAtEpoch > c.LastEpoch || (posts[start].CreatedAtEpoch == c.LastEpoch && posts[start].ID >= c.LastPostID)) {
			start++
		}
	}
	if start > len(posts) {
		start = len(posts)
	}
	end := start + q.Limit
	if end > len(posts) {
		end = len(posts)
	}
	page := posts[start:end]
	ads, err := s.ads.List(ctx)
	if err != nil {
		return Result{}, ErrDependencyUnavailable
	}
	eligible := make([]ad.Advertisement, 0, len(ads))
	for _, a := range ads {
		if a.Status == ad.StatusActive && !a.StartsAt.After(now) && !a.EndsAt.Before(now) {
			eligible = append(eligible, a)
		}
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].ID < eligible[j].ID })
	shown := map[string]bool{}
	for _, id := range c.Shown {
		shown[id] = true
	}
	items := make([]Item, 0, len(page)+len(page)/20+1)
	block := c.Organic
	authors := map[string]user.User{}
	if s.authors != nil {
		authorIDs := make([]string, 0, len(page))
		for _, p := range page {
			authorIDs = append(authorIDs, p.AuthorID)
		}
		authors, err = s.authors.GetByIDs(ctx, authorIDs)
		if err != nil {
			return Result{}, ErrDependencyUnavailable
		}
	}
	for _, p := range page {
		item := Item{Post: &p}
		if s.authors != nil {
			author, ok := authors[p.AuthorID]
			if !ok {
				return Result{}, ErrDependencyUnavailable
			}
			item.Author = &author
		}
		items = append(items, item)
		block++
		if block%20 == 0 && len(eligible) > 0 {
			idx := 0
			if c.Seed > 0 {
				idx = c.Seed % len(eligible)
			}
			chosen := -1
			for n := 0; n < len(eligible); n++ {
				candidate := (idx + n) % len(eligible)
				if !shown[eligible[candidate].ID] || len(shown) == len(eligible) {
					chosen = candidate
					break
				}
			}
			if chosen >= 0 {
				a := eligible[chosen]
				items = append(items, Item{Ad: &a})
				shown[a.ID] = true
				c.Shown = append(c.Shown, a.ID)
			}
		}
	}
	if len(page) > 0 && len(eligible) > 0 && block%20 != 0 {
		idx := 0
		if c.Seed > 0 {
			idx = c.Seed % len(eligible)
		}
		chosen := -1
		for n := 0; n < len(eligible); n++ {
			candidate := (idx + n) % len(eligible)
			if !shown[eligible[candidate].ID] || len(shown) == len(eligible) {
				chosen = candidate
				break
			}
		}
		if chosen >= 0 {
			a := eligible[chosen]
			items = append(items, Item{Ad: &a})
			shown[a.ID] = true
			c.Shown = append(c.Shown, a.ID)
		}
	}
	c.Organic = block
	hasMore := end < len(posts)
	if hasMore {
		last := page[len(page)-1]
		c.LastEpoch = last.CreatedAtEpoch
		c.LastPostID = last.ID
		c.Expires = now.Add(s.ttl).Unix()
		c.Issued = now.Unix()
		c.Seed = int(sha256.Sum256([]byte(c.LastPostID))[0])
		next, e := s.sign(c)
		if e != nil {
			return Result{}, e
		}
		return Result{Items: items, NextCursor: next, HasMore: true}, nil
	}
	return Result{Items: items, HasMore: false}, nil
}
