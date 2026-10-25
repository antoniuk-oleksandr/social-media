package post

import "time"

type CreateRequest struct {
	Content string `json:"content"`
}

type Response struct {
	ID        string `json:"id"`
	AuthorID  string `json:"authorId"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

func ToCreateCommand(authorID string, request CreateRequest) CreateCommand {
	return CreateCommand{AuthorID: authorID, Content: request.Content}
}

func ToResponse(value Post) Response {
	return Response{ID: value.ID, AuthorID: value.AuthorID, Content: value.Content, CreatedAt: value.CreatedAt.UTC().Format(time.RFC3339Nano)}
}
