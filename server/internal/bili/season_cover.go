package bili

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

func defaultSeasonCoverPNG() ([]byte, error) {
	const (
		width  = 1146
		height = 717
	)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	top := color.RGBA{R: 22, G: 163, B: 74, A: 255}
	bottom := color.RGBA{R: 30, G: 41, B: 59, A: 255}
	for y := 0; y < height; y++ {
		ratio := float64(y) / float64(height-1)
		c := color.RGBA{
			R: uint8(float64(top.R)*(1-ratio) + float64(bottom.R)*ratio),
			G: uint8(float64(top.G)*(1-ratio) + float64(bottom.G)*ratio),
			B: uint8(float64(top.B)*(1-ratio) + float64(bottom.B)*ratio),
			A: 255,
		}
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
