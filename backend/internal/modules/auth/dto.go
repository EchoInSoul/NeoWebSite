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
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type TwoFactorEnableRequest struct {
	Code string `json:"code"`
}

type TwoFactorDisableRequest struct {
	Password string `json:"password"`
}

type TwoFactorSetupResponse struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauth_url"`
}

type RecoveryCodesResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

type UserResponse struct {
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
}

type GroupResponse struct {
	ID        string  `json:"id"`
	SystemKey *string `json:"system_key"`
	Name      string  `json:"name"`
}

type MeResponse struct {
	Email       string        `json:"email"`
	Nickname    string        `json:"nickname"`
	Group       GroupResponse `json:"group"`
	Permissions []string      `json:"permissions"`
}
