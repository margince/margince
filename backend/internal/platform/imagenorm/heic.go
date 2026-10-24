// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imagenorm

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/gen2brain/heic"
	xdraw "golang.org/x/image/draw"
)

// maxHEICPixels bounds a HEIC decode by its declared size. A phone sensor is
// up to 48 Mpx, so this sits well above the 8 Mpx a company mark may be.
const maxHEICPixels = 64 << 20

// photoJPEGEdge is the longest edge of the JPEG a HEIC photo becomes. Card
// print stays legible at this size. The file stays inside one model request.
const photoJPEGEdge = 2560

// photoJPEGQuality favours legibility of small print over file size.
const photoJPEGQuality = 90

// HEICToJPEG converts a HEIC or HEIF photo to a JPEG, so a model binding that
// cannot read HEIC can read the photo. A multi-image file yields its first
// frame. Bytes that do not decode, or that declare more than maxHEICPixels,
// are ErrUnsupported.
func HEICToJPEG(src []byte) ([]byte, error) {
	cfg, err := heic.DecodeConfig(bytes.NewReader(src))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxHEICPixels {
		return nil, ErrUnsupported
	}
	img, err := heic.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, ErrUnsupported
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, fitWithin(img, photoJPEGEdge), &jpeg.Options{Quality: photoJPEGQuality}); err != nil {
		return nil, fmt.Errorf("imagenorm: encoding the converted photo: %w", err)
	}
	return out.Bytes(), nil
}

// fitWithin shrinks img so neither edge exceeds edge, keeping the aspect ratio.
// A smaller image is returned as it is.
func fitWithin(img image.Image, edge int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= edge && h <= edge {
		return img
	}
	if w >= h {
		h = h * edge / w
		w = edge
	} else {
		w = w * edge / h
		h = edge
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	return dst
}
