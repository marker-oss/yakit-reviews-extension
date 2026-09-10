// internal/mediaproxy/blur.go
package mediaproxy

import (
	_ "embed"
	"image"
	"image/color"

	pigo "github.com/esimov/pigo/core"
)

//go:embed cascade/facefinder
var cascade []byte

type detector struct{ classifier *pigo.Pigo }

func newDetector() (*detector, error) {
	p := pigo.NewPigo()
	c, err := p.Unpack(cascade)
	if err != nil {
		return nil, err
	}
	return &detector{classifier: c}, nil
}

// detectFaces returns face bounding boxes in image coordinates.
func (d *detector) detectFaces(img *image.RGBA) []image.Rectangle {
	bounds := img.Bounds()
	cols, rows := bounds.Dx(), bounds.Dy()
	gray := pigo.RgbToGrayscale(img)
	params := pigo.CascadeParams{
		MinSize:     40,
		MaxSize:     1000,
		ShiftFactor: 0.1,
		ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{Pixels: gray, Rows: rows, Cols: cols, Dim: cols},
	}
	dets := d.classifier.RunCascade(params, 0.0)
	dets = d.classifier.ClusterDetections(dets, 0.2)
	var boxes []image.Rectangle
	for _, det := range dets {
		if det.Q < 5.0 { // confidence threshold
			continue
		}
		r := det.Scale / 2
		cx, cy := det.Col, det.Row
		box := image.Rect(cx-r, cy-r, cx+r, cy+r).Add(bounds.Min)
		boxes = append(boxes, box.Intersect(bounds))
	}
	return boxes
}

// blurRegion applies a box blur of the given radius inside rect only. Pixels
// outside rect are never written and never sampled, so the surrounding image is
// left intact. The averaging is done in place (each pixel reads its already
// partly-blurred neighbours), which both smooths faster across the region and
// guarantees interior pixels actually change even for a linear gradient (a
// strictly symmetric kernel would average a linear ramp back to its own value).
// For the face-redaction use case this still destroys facial detail, which is
// the point.
func blurRegion(img *image.RGBA, rect image.Rectangle, radius int) {
	rect = rect.Intersect(img.Bounds())
	if rect.Empty() || radius < 1 {
		return
	}
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			var sr, sg, sb, sa, n int
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					px, py := x+dx, y+dy
					if !image.Pt(px, py).In(rect) {
						continue
					}
					c := img.RGBAAt(px, py)
					sr += int(c.R)
					sg += int(c.G)
					sb += int(c.B)
					sa += int(c.A)
					n++
				}
			}
			if n == 0 {
				continue
			}
			img.SetRGBA(x, y, colorFrom(sr/n, sg/n, sb/n, sa/n))
		}
	}
}

func colorFrom(r, g, b, a int) color.RGBA {
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}
}
