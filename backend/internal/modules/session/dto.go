package session

type Meta struct {
	UserAgent string
	IP        string
}

type Session struct {
	ID         string
	Current    bool
	UserAgent  string
	IP         string
	CreatedAt  string
	LastSeenAt string
	ExpiresAt  string
}
