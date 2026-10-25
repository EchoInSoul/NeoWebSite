package registration

type User struct {
	Email    string
	Nickname string
	Bio      *string
}

type Result struct {
	User                *User
	VerificationPending bool
}
