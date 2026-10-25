package user

import "time"

type ProfileResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	CreatedAt   string `json:"createdAt"`
}

func ToProfileResponse(value User) ProfileResponse {
	return ProfileResponse{
		ID:          value.ID,
		Username:    value.Username,
		DisplayName: value.DisplayName,
		Role:        string(value.Role),
		CreatedAt:   value.CreatedAt.UTC().Format(time.RFC3339),
	}
}
