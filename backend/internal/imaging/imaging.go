// Package imaging prepares photos for model calls. Models bill by image
// size, so photos are shrunk to the smallest size that keeps text legible.
package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // decoder registration

	"golang.org/x/image/draw"
)

// Fit decodes a JPEG or PNG, scales it so the long edge is at most maxEdge,
// and re-encodes it as JPEG. Clients upload upright images, so EXIF
// orientation is not applied here.
func Fit(data []byte, maxEdge int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, fmt.Errorf("empty image")
	}
	scale := 1.0
	if long := max(w, h); long > maxEdge {
		scale = float64(maxEdge) / float64(long)
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encode image: %w", err)
	}
	return out.Bytes(), nil
}

// Box is a region of an image as fractions of its size, origin top-left.
type Box struct{ X, Y, W, H float64 }

// Crop cuts a region out of an image, grown by margin (a fraction of its
// size) on every side and kept within the image, then fits it to maxEdge.
// A box covering most of the image is not worth cutting: the whole image
// is fitted instead.
func Crop(data []byte, box Box, margin float64, maxEdge int) ([]byte, error) {
	if box.W <= 0 || box.H <= 0 || box.W*box.H > 0.8 {
		return Fit(data, maxEdge)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	b := src.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	x0 := max(0, box.X-box.W*margin) * w
	y0 := max(0, box.Y-box.H*margin) * h
	x1 := min(1, box.X+box.W*(1+margin)) * w
	y1 := min(1, box.Y+box.H*(1+margin)) * h
	r := image.Rect(b.Min.X+int(x0), b.Min.Y+int(y0), b.Min.X+int(x1), b.Min.Y+int(y1))
	if r.Dx() < 32 || r.Dy() < 32 {
		return Fit(data, maxEdge)
	}
	cut := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Copy(cut, image.Point{}, src, r, draw.Src, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, cut, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("encode image: %w", err)
	}
	return Fit(out.Bytes(), maxEdge)
}
