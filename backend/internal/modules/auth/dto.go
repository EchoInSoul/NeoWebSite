package auth

type User struct {
	ID       string
	Email    string
	Nickname string
}

type LoginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
	Password string `json:"password"`
}

type PasswordChangeRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type UserResponse struct {
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
