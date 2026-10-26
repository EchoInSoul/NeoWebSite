package builtin

import (
	"github.com/google/uuid"

	"website/backend/internal/common/apierror"
)

type CreateChallengeRequest struct {
	Scope string `json:"scope"`
}

type TestChallengeRequest struct {
	Mode string `json:"mode"`
}

type SolveChallengeRequest struct {
	X      *int         `json:"x"`
	Y      *int         `json:"y"`
	Clicks []ClickInput `json:"clicks"`
}

type ClickInput struct {
	X *int `json:"x"`
	Y *int `json:"y"`
}

func (r SolveChallengeRequest) clicks() ([]Point, error) {
	if r.Clicks == nil {
		if r.X == nil {
			return nil, apierror.WithParam(apierror.ErrInvalidRequest, "x")
		}
		if r.Y == nil {
			return nil, apierror.WithParam(apierror.ErrInvalidRequest, "y")
		}
		if *r.X < 0 {
			return nil, apierror.WithParam(apierror.ErrInvalidRequest, "x")
		}
		if *r.Y < 0 {
			return nil, apierror.WithParam(apierror.ErrInvalidRequest, "y")
		}
		return []Point{{X: *r.X, Y: *r.Y}}, nil
	}

	if r.X != nil || r.Y != nil {
		return nil, apierror.WithParam(apierror.ErrInvalidRequest, "clicks")
	}
	if len(r.Clicks) != clickDots {
		return nil, apierror.WithParam(apierror.ErrInvalidRequest, "clicks")
	}
	points := make([]Point, 0, len(r.Clicks))
	for _, click := range r.Clicks {
		if click.X == nil || click.Y == nil {
			return nil, apierror.WithParam(apierror.ErrInvalidRequest, "clicks")
		}
		if *click.X < 0 || *click.Y < 0 {
			return nil, apierror.WithParam(apierror.ErrInvalidRequest, "clicks")
		}
		points = append(points, Point{X: *click.X, Y: *click.Y})
	}
	return points, nil
}

const (
	statusPending = iota
	statusSolved
	statusConsumed
	statusFailed
)

type ChallengeResponse struct {
	ID             uuid.UUID `json:"id"`
	MasterImage    string    `json:"master_image"`
	TileImage      string    `json:"tile_image"`
	PromptImage    string    `json:"prompt_image,omitempty"`
	PromptText     string    `json:"prompt_text,omitempty"`
	MasterWidth    int       `json:"master_width"`
	MasterHeight   int       `json:"master_height"`
	TileWidth      int       `json:"tile_width"`
	TileHeight     int       `json:"tile_height"`
	TileX          int       `json:"tile_x"`
	TileY          int       `json:"tile_y"`
	ExpectedClicks int       `json:"expected_clicks,omitempty"`
	ExpiresAt      string    `json:"expires_at"`
}

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type AnswerDot struct {
	Text   string `json:"text"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type AnswerDots struct {
	Dots []AnswerDot `json:"dots"`
}

type SolveChallengeResponse struct {
	CaptchaToken string `json:"captcha_token"`
	ExpiresAt    string `json:"expires_at"`
}
