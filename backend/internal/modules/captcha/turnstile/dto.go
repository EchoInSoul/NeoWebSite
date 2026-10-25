package turnstile

type VerificationRequest struct {
	SecretKey string `json:"secret_key"`
	Token     string `json:"token"`
}

type VerificationResponse struct {
	Verified bool `json:"verified"`
}
