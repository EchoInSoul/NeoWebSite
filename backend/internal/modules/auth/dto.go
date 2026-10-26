package auth

type User struct {
	ID               string
	Email            string
	Nickname         string
	Bio              *string
	TwoFactorEnabled bool
}

type RegisterPendingResponse struct {
	Status string `json:"status"`
}

type LoginRequest struct {
	Identifier   string `json:"identifier"`
	Password     string `json:"password"`
	CaptchaToken string `json:"captcha_token"`
}

type RegisterRequest struct {
	Email        string `json:"email"`
	Nickname     string `json:"nickname"`
	Password     string `json:"password"`
	CaptchaToken string `json:"captcha_token"`
}

type EmailVerificationRequest struct {
	Token string `json:"token"`
}

type PasswordResetRequest struct {
	Email string `json:"email"`
}

type PasswordResetConfirmationRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type ProfileUpdateRequest struct {
	Nickname *string `json:"nickname"`
	Email    *string `json:"email"`
	Bio      *string `json:"bio"`
}

type PasswordChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type UserResponse struct {
	Email    string  `json:"email"`
	Nickname string  `json:"nickname"`
	Bio      *string `json:"bio"`
}

type GroupResponse struct {
	ID        string  `json:"id"`
	SystemKey *string `json:"system_key"`
	Name      string  `json:"name"`
}

type MeResponse struct {
	Email            string        `json:"email"`
	Nickname         string        `json:"nickname"`
	Bio              *string       `json:"bio"`
	Group            GroupResponse `json:"group"`
	Permissions      []string      `json:"permissions"`
	TwoFactorEnabled bool          `json:"two_factor_enabled"`
}

type SessionResponse struct {
	ID         string `json:"id"`
	Current    bool   `json:"current"`
	UserAgent  string `json:"user_agent"`
	IP         string `json:"ip"`
	CreatedAt  string `json:"created_at"`
	LastSeenAt string `json:"last_seen_at"`
	ExpiresAt  string `json:"expires_at"`
}
