package mail

type createMessageRequest struct {
	To       string `json:"to"`
	Template string `json:"template"`
}

type createMessageResponse struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Sent    bool   `json:"sent"`
}

type statusResponse struct {
	SMTPConfigured    bool `json:"smtp_configured"`
	SiteURLConfigured bool `json:"site_url_configured"`
}
