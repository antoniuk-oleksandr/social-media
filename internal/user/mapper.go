package user

import "time"

type UpdateProfileRequest struct {
	DisplayName *string `json:"displayName"`
	Username    *string `json:"username"`
}

func ToUpdateProfileCommand(userID string, request UpdateProfileRequest) UpdateProfileCommand {
	return UpdateProfileCommand{UserID: userID, Username: request.Username, DisplayName: request.DisplayName}
}

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
