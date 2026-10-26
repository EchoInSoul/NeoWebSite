package builtin

import (
	"fmt"
	"strings"

	"github.com/golang/freetype/truetype"
	"github.com/wenlng/go-captcha-assets/resources/fonts/fzshengsksjw"
	"github.com/wenlng/go-captcha-assets/resources/imagesv2"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/click"
)

type clickGenerated struct {
	MasterImage  string
	PromptImage  string
	PromptText   string
	MasterWidth  int
	MasterHeight int
	Dots         []AnswerDot
}

type clickGenerator struct {
	captcha click.Captcha
}

func newClickGenerator() (*clickGenerator, error) {
	backgrounds, err := imagesv2.GetImages()
	if err != nil {
		return nil, fmt.Errorf("captcha: load click backgrounds: %w", err)
	}

	font, err := fzshengsksjw.GetFont()
	if err != nil {
		return nil, fmt.Errorf("captcha: load click font: %w", err)
	}

	builder := click.NewBuilder()
	builder.SetResources(click.WithBackgrounds(backgrounds), click.WithFonts([]*truetype.Font{font}))
	builder.SetOptions(click.WithRangeVerifyLen(option.RangeVal{Min: 3, Max: 3}))

	return &clickGenerator{captcha: builder.Make()}, nil
}

func (g *clickGenerator) generate() (clickGenerated, error) {
	data, err := g.captcha.Generate()
	if err != nil {
		return clickGenerated{}, fmt.Errorf("captcha: click generate: %w", err)
	}

	block := data.GetData()
	if len(block) != 3 {
		return clickGenerated{}, fmt.Errorf("captcha: click generate: got %d dots, want 3", len(block))
	}

	dots := make([]AnswerDot, 0, len(block))
	for i := 0; i < len(block); i++ {
		dot := block[i]
		if dot == nil {
			return clickGenerated{}, fmt.Errorf("captcha: click generate: empty dot")
		}
		dots = append(dots, AnswerDot{
			Text:   dot.Text,
			X:      dot.X,
			Y:      dot.Y,
			Width:  dot.Width,
			Height: dot.Height,
		})
	}

	masterImage, err := data.GetMasterImage().ToBase64()
	if err != nil {
		return clickGenerated{}, fmt.Errorf("captcha: encode click master image: %w", err)
	}

	promptImage, err := data.GetThumbImage().ToBase64()
	if err != nil {
		return clickGenerated{}, fmt.Errorf("captcha: encode click prompt image: %w", err)
	}

	texts := make([]string, 0, len(dots))
	for _, dot := range dots {
		texts = append(texts, dot.Text)
	}

	size := g.captcha.GetOptions().GetImageSize()

	return clickGenerated{
		MasterImage:  masterImage,
		PromptImage:  promptImage,
		PromptText:   strings.Join(texts, " "),
		MasterWidth:  size.Width,
		MasterHeight: size.Height,
		Dots:         dots,
	}, nil
}
