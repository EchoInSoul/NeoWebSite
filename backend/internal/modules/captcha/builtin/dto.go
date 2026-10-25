package builtin

import "github.com/google/uuid"

type CreateChallengeRequest struct {
	Scope string `json:"scope"`
}

type SolveChallengeRequest struct {
	X *int `json:"x"`
	Y *int `json:"y"`
}

const (
	statusPending = iota
	statusSolved
	statusConsumed
	statusFailed
)

type ChallengeResponse struct {
	ID           uuid.UUID `json:"id"`
	MasterImage  string    `json:"master_image"`
	TileImage    string    `json:"tile_image"`
	MasterWidth  int       `json:"master_width"`
	MasterHeight int       `json:"master_height"`
	TileWidth    int       `json:"tile_width"`
	TileHeight   int       `json:"tile_height"`
	TileX        int       `json:"tile_x"`
	TileY        int       `json:"tile_y"`
	ExpiresAt    string    `json:"expires_at"`
}

type SolveChallengeResponse struct {
	CaptchaToken string `json:"captcha_token"`
	ExpiresAt    string `json:"expires_at"`
}
