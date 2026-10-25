package ad

import "time"

type CreateRequest struct {
	Advertiser string `json:"advertiser"`
	Content    string `json:"content"`
	TargetURL  string `json:"targetUrl"`
	StartsAt   string `json:"startsAt"`
	EndsAt     string `json:"endsAt"`
	Status     Status `json:"status"`
}

type Response struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Advertiser string `json:"advertiser"`
	Content    string `json:"content"`
	TargetURL  string `json:"targetUrl"`
	Status     Status `json:"status"`
	StartsAt   string `json:"startsAt"`
	EndsAt     string `json:"endsAt"`
	CreatedAt  string `json:"createdAt"`
}

func ToCreateCommand(adminID string, request CreateRequest) (CreateCommand, error) {
	startsAt, err := time.Parse(time.RFC3339, request.StartsAt)
	if err != nil {
		return CreateCommand{}, ErrValidation
	}
	endsAt, err := time.Parse(time.RFC3339, request.EndsAt)
	if err != nil {
		return CreateCommand{}, ErrValidation
	}
	return CreateCommand{AdminID: adminID, Advertiser: request.Advertiser, Content: request.Content, TargetURL: request.TargetURL, StartsAt: startsAt, EndsAt: endsAt, Status: request.Status}, nil
}

func ToResponse(value Advertisement) Response {
	return Response{ID: value.ID, Type: value.Type, Advertiser: value.Advertiser, Content: value.Content, TargetURL: value.TargetURL, Status: value.Status, StartsAt: value.StartsAt.UTC().Format(time.RFC3339), EndsAt: value.EndsAt.UTC().Format(time.RFC3339), CreatedAt: value.CreatedAt.UTC().Format(time.RFC3339Nano)}
}
