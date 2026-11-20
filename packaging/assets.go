// Package icons provides Lanboat's application icon for native windows.
package icons

import _ "embed"

//go:embed icons/256x256/lanboat.png
var PNG []byte

//go:embed icons/32x32/lanboat.png
var TrayPNG []byte

//go:embed icons/64x64/lanboat.png
var WindowPNG []byte

//go:embed icons/16x16/lanboat.png
var MenuPNG []byte
