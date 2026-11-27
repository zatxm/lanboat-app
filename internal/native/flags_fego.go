package native

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"
	"strings"
	"sync"

	"github.com/zatxm/fego/v26/ui"
)

//go:embed flags.png
var flagPNG []byte
var flagAtlas = sync.OnceValue(func() image.Image {
	img, err := png.Decode(bytes.NewReader(flagPNG))
	if err != nil {
		panic(err)
	}
	return img
})

func (v *nativeView) countryIcon(code string) *widget {
	w := v.box("country-flag", false, 0, false)
	x, y := int(code[0]-'a')*32, int(code[1]-'a')*24
	rect := image.Rect(x, y, x+32, y+24)
	img := flagAtlas().(interface {
		SubImage(image.Rectangle) image.Image
	}).SubImage(rect)
	w.pointer.Size(24, 18).Children(ui.Image(img).Size(24, 18))
	w.setTooltip(strings.ToUpper(code))
	return w
}
