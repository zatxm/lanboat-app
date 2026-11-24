package native

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"
	"strings"
	"sync"

	"github.com/zatxm/fego/v26/ui"
	"github.com/zatxm/lanboat-app/internal/ruledata"
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

func (v *nativeView) countryIcon(host, fallback string) *widget {
	w := v.box("country-flag", false, 0, false)
	w.pointer.Size(24, 18)
	w.value = "?"
	if !ruledata.ValidRegion(fallback) {
		fallback = ""
	}
	w.update = func() {
		code := v.ui.flags[host].code
		if code == "" {
			code = fallback
		}
		if w.value == code {
			return
		}
		w.value = code
		if len(code) != 2 {
			w.icon = "globe"
			w.pointer.Children(w.lineIcon(18)).Tooltip("")
			return
		}
		x, y := int(code[0]-'a')*32, int(code[1]-'a')*24
		rect := image.Rect(x, y, x+32, y+24)
		img := flagAtlas().(interface {
			SubImage(image.Rectangle) image.Image
		}).SubImage(rect)
		w.pointer.Children(ui.Image(img).Size(24, 18)).Tooltip(strings.ToUpper(code))
	}
	w.update()
	return w
}
