package captcha

type CaptchaVerificationRequest struct {
	SecretKey string `json:"secret_key"`
	Token     string `json:"token"`
}

type CaptchaVerificationResponse struct {
	Verified bool `json:"verified"`
}
