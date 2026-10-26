package builtin

import (
	"strings"
	"testing"
)

func TestClickGeneratorProducesChallenge(t *testing.T) {
	generator, err := newClickGenerator()
	if err != nil {
		t.Fatalf("newClickGenerator: %v", err)
	}

	generated, err := generator.generate()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if !strings.HasPrefix(generated.MasterImage, "data:image/jpeg;base64,") {
		t.Fatalf("master image = %.40q", generated.MasterImage)
	}
	if !strings.HasPrefix(generated.PromptImage, "data:image/png;base64,") {
		t.Fatalf("prompt image = %.40q", generated.PromptImage)
	}
	if generated.MasterWidth != 300 || generated.MasterHeight != 220 {
		t.Fatalf("master size = %dx%d, want 300x220", generated.MasterWidth, generated.MasterHeight)
	}
	if len(generated.Dots) != 3 {
		t.Fatalf("dots = %d, want 3", len(generated.Dots))
	}
	seen := map[string]bool{}
	for i, dot := range generated.Dots {
		if dot.Text == "" || seen[dot.Text] {
			t.Fatalf("dots[%d].Text = %q, want unique non-empty", i, dot.Text)
		}
		seen[dot.Text] = true
		if dot.X < 0 || dot.Y < 0 || dot.Width <= 0 || dot.Height <= 0 {
			t.Fatalf("dots[%d] = %+v, want positive box", i, dot)
		}
		if dot.X+dot.Width > generated.MasterWidth || dot.Y+dot.Height > generated.MasterHeight {
			t.Fatalf("dots[%d] box %+v outside %dx%d", i, dot, generated.MasterWidth, generated.MasterHeight)
		}
	}
	want := strings.Join([]string{generated.Dots[0].Text, generated.Dots[1].Text, generated.Dots[2].Text}, " ")
	if generated.PromptText != want {
		t.Fatalf("prompt text = %q, want %q", generated.PromptText, want)
	}
}

func TestClickGeneratorProducesDifferentTargets(t *testing.T) {
	generator, err := newClickGenerator()
	if err != nil {
		t.Fatalf("newClickGenerator: %v", err)
	}

	first, err := generator.generate()
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	second, err := generator.generate()
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}

	if first.MasterImage == second.MasterImage {
		t.Fatal("two generations produced identical master images")
	}
}
