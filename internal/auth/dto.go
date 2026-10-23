package auth

type RegisterRequest struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}
type LoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type UserResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	CreatedAt   string `json:"createdAt"`
}
type RegisterResponse struct {
	User             UserResponse `json:"user"`
	AccessToken      string       `json:"accessToken"`
	RefreshToken     string       `json:"refreshToken"`
	TokenType        string       `json:"tokenType"`
	ExpiresIn        int64        `json:"expiresIn"`
	RefreshExpiresIn int64        `json:"refreshExpiresIn"`
}
type LoginUserResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}
type LoginResponse struct {
	User             LoginUserResponse `json:"user"`
	AccessToken      string            `json:"accessToken"`
	RefreshToken     string            `json:"refreshToken"`
	TokenType        string            `json:"tokenType"`
	ExpiresIn        int64             `json:"expiresIn"`
	RefreshExpiresIn int64             `json:"refreshExpiresIn"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}
