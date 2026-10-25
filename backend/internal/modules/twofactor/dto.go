package twofactor

type User struct {
	Email    string
	Nickname string
	Bio      *string
}

type TwoFactorCodeRequest struct {
	Code string `json:"code"`
}

type TwoFactorSetupRequest struct {
	Password string `json:"password"`
}

type TwoFactorEnableRequest struct {
	Code     string `json:"code"`
	Password string `json:"password"`
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
