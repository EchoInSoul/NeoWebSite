package mail

type createMessageRequest struct {
	To string `json:"to"`
}

type createMessageResponse struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Sent    bool   `json:"sent"`
}
