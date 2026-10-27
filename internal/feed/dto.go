package feed

import (
	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	"github.com/antoniuk-oleksandr/social-media/internal/post"
	"time"
)

type PostData struct {
	ID        string     `json:"id"`
	Author    AuthorData `json:"author"`
	Content   string     `json:"content"`
	CreatedAt string     `json:"createdAt"`
}
type AuthorData struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}
type AdData struct {
	ID         string `json:"id"`
	Advertiser string `json:"advertiser"`
	Content    string `json:"content"`
	TargetURL  string `json:"targetUrl"`
	CreatedAt  string `json:"createdAt"`
}
type ItemResponse struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}
type Response struct {
	Items      []ItemResponse `json:"items"`
	NextCursor string         `json:"nextCursor,omitempty"`
	HasMore    bool           `json:"hasMore"`
}

func toResponse(value Result) Response {
	out := Response{Items: make([]ItemResponse, 0, len(value.Items)), NextCursor: value.NextCursor, HasMore: value.HasMore}
	for _, item := range value.Items {
		if item.Post != nil {
			p := item.Post
			author := AuthorData{ID: p.AuthorID}
			if item.Author != nil {
				author = AuthorData{ID: item.Author.ID, Username: item.Author.Username, DisplayName: item.Author.DisplayName}
			}
			out.Items = append(out.Items, ItemResponse{Type: "post", Data: PostData{ID: p.ID, Author: author, Content: p.Content, CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339Nano)}})
		}
		if item.Ad != nil {
			a := item.Ad
			out.Items = append(out.Items, ItemResponse{Type: "ad", Data: AdData{ID: a.ID, Advertiser: a.Advertiser, Content: a.Content, TargetURL: a.TargetURL, CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339Nano)}})
		}
	}
	return out
}

var _ = ad.StatusActive
var _ post.Post
