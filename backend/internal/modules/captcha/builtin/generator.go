package builtin

import (
	"fmt"

	"github.com/wenlng/go-captcha-assets/resources/imagesv2"
	"github.com/wenlng/go-captcha-assets/resources/tiles"
	"github.com/wenlng/go-captcha/v2/slide"
)

type generated struct {
	MasterImage  string
	TileImage    string
	MasterWidth  int
	MasterHeight int
	TileWidth    int
	TileHeight   int
	TileX        int
	TileY        int
	AnswerX      int
	AnswerY      int
}

type generator struct {
	captcha slide.Captcha
}

func newGenerator() (*generator, error) {
	backgrounds, err := imagesv2.GetImages()
	if err != nil {
		return nil, fmt.Errorf("captcha: load backgrounds: %w", err)
	}

	tileImages, err := tiles.GetTiles()
	if err != nil {
		return nil, fmt.Errorf("captcha: load tiles: %w", err)
	}

	graphs := make([]*slide.GraphImage, 0, len(tileImages))
	for _, tile := range tileImages {
		graphs = append(graphs, &slide.GraphImage{
			OverlayImage: tile.OverlayImage,
			ShadowImage:  tile.ShadowImage,
			MaskImage:    tile.MaskImage,
		})
	}

	builder := slide.NewBuilder()
	builder.SetResources(slide.WithBackgrounds(backgrounds), slide.WithGraphImages(graphs))

	return &generator{captcha: builder.Make()}, nil
}

func (g *generator) generate() (generated, error) {
	data, err := g.captcha.Generate()
	if err != nil {
		return generated{}, fmt.Errorf("captcha: generate: %w", err)
	}

	block := data.GetData()
	if block == nil {
		return generated{}, fmt.Errorf("captcha: generate: empty block")
	}

	masterImage, err := data.GetMasterImage().ToBase64()
	if err != nil {
		return generated{}, fmt.Errorf("captcha: encode master image: %w", err)
	}

	tileImage, err := data.GetTileImage().ToBase64()
	if err != nil {
		return generated{}, fmt.Errorf("captcha: encode tile image: %w", err)
	}

	size := g.captcha.GetOptions().GetImageSize()

	return generated{
		MasterImage:  masterImage,
		TileImage:    tileImage,
		MasterWidth:  size.Width,
		MasterHeight: size.Height,
		TileWidth:    block.Width,
		TileHeight:   block.Height,
		TileX:        block.DX,
		TileY:        block.DY,
		AnswerX:      block.X,
		AnswerY:      block.Y,
	}, nil
}
