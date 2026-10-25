package builtin

import (
	"strings"
	"testing"
)

func TestGeneratorProducesSlideChallenge(t *testing.T) {
	generator := mustGenerator(t)

	got, err := generator.generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if !strings.HasPrefix(got.MasterImage, "data:image/jpeg;base64,") {
		t.Fatalf("master image = %.40q", got.MasterImage)
	}
	if !strings.HasPrefix(got.TileImage, "data:image/png;base64,") {
		t.Fatalf("tile image = %.40q", got.TileImage)
	}
	if got.MasterWidth != 300 || got.MasterHeight != 220 {
		t.Fatalf("master size = %dx%d, want 300x220", got.MasterWidth, got.MasterHeight)
	}
	if got.TileWidth <= 0 || got.TileHeight <= 0 {
		t.Fatalf("tile size = %dx%d", got.TileWidth, got.TileHeight)
	}
	if got.AnswerX < 0 || got.AnswerX+got.TileWidth > got.MasterWidth {
		t.Fatalf("answer x = %d (tile %d, master %d)", got.AnswerX, got.TileWidth, got.MasterWidth)
	}
	if got.AnswerY < 0 || got.AnswerY+got.TileHeight > got.MasterHeight {
		t.Fatalf("answer y = %d (tile %d, master %d)", got.AnswerY, got.TileHeight, got.MasterHeight)
	}
	if got.TileX < 0 || got.TileX > got.MasterWidth {
		t.Fatalf("tile x = %d", got.TileX)
	}
	if got.TileY < 0 || got.TileY > got.MasterHeight {
		t.Fatalf("tile y = %d", got.TileY)
	}
}
