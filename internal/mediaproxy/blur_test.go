// internal/mediaproxy/blur_test.go
package mediaproxy

import (
	"image"
	"image/color"
	"testing"
)

func TestBlurRegionChangesPixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for x := 0; x < 20; x++ {
		for y := 0; y < 20; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 12), G: 0, B: 0, A: 255})
		}
	}
	before := img.RGBAAt(5, 5)
	blurRegion(img, image.Rect(2, 2, 12, 12), 3)
	after := img.RGBAAt(5, 5)
	if before == after {
		t.Error("expected pixel inside region to change after blur")
	}
	// Pixel outside the region is untouched.
	if img.RGBAAt(18, 18) != (color.RGBA{R: 216, G: 0, B: 0, A: 255}) {
		t.Error("pixel outside region must be unchanged")
	}
}
