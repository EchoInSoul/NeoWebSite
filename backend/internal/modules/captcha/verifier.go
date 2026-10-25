package captcha

import "context"

type Outcome int

const (
	Passed Outcome = iota
	Rejected
	Unavailable
)

type Verifier interface {
	Verify(ctx context.Context, secret, token, remoteIP string) Outcome
}

type unavailableVerifier struct{}

func (unavailableVerifier) Verify(context.Context, string, string, string) Outcome {
	return Unavailable
}
