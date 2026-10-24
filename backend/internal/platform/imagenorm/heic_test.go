// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imagenorm

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"os"
	"testing"
)

func readHEICFixture(t *testing.T) []byte {
	t.Helper()
	src, err := os.ReadFile("testdata/card.heic")
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func TestHEICToJPEGDecodesARealHEICFile(t *testing.T) {
	out, err := HEICToJPEG(context.Background(), readHEICFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("the output must be a JPEG every model binding can read: %v", err)
	}
	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		t.Fatalf("empty picture: %v", img.Bounds())
	}
}

func TestHEICToJPEGRefusesBytesThatAreNotHEIC(t *testing.T) {
	for name, src := range map[string][]byte{
		"empty":     nil,
		"text":      []byte("not an image"),
		"truncated": readHEICFixture(t)[:200],
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := HEICToJPEG(context.Background(), src); !errors.Is(err, ErrUnsupported) {
				t.Fatalf("got %v, want ErrUnsupported", err)
			}
		})
	}
}

func TestFitWithinShrinksTheLongEdgeAndKeepsTheAspect(t *testing.T) {
	got := fitWithin(image.NewRGBA(image.Rect(0, 0, 4000, 1000)), 2000).Bounds()
	if got.Dx() != 2000 || got.Dy() != 500 {
		t.Fatalf("got %v, want 2000x500", got)
	}
	small := image.NewRGBA(image.Rect(0, 0, 100, 50))
	if fitWithin(small, 2000).Bounds() != small.Bounds() {
		t.Fatal("a picture inside the edge must be returned unchanged")
	}
}

func TestHEICToJPEGRefusesAnImageOverThePixelCapWithoutDecodingIt(t *testing.T) {
	declared := func(w, h int) func(io.Reader) (image.Config, error) {
		return func(io.Reader) (image.Config, error) { return image.Config{Width: w, Height: h}, nil }
	}
	decoded := false
	oldConfig, oldDecode := heicDecodeConfig, heicDecode
	t.Cleanup(func() { heicDecodeConfig, heicDecode = oldConfig, oldDecode })
	heicDecode = func(io.Reader) (image.Image, error) {
		decoded = true
		return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
	}

	heicDecodeConfig = declared(4000, 3000)
	if _, err := HEICToJPEG(context.Background(), readHEICFixture(t)); err != nil || !decoded {
		t.Fatalf("a 12 Mpx photo must decode: err %v, decoded %v", err, decoded)
	}
	decoded = false
	heicDecodeConfig = declared(6000, 4000)
	if _, err := HEICToJPEG(context.Background(), readHEICFixture(t)); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("got %v, want ErrUnsupported for a 24 Mpx declaration", err)
	}
	if decoded {
		t.Fatal("an image over the cap was decoded")
	}
}

func TestHEICToJPEGStopsWaitingForADecodeSlotWhenTheRequestEnds(t *testing.T) {
	for range maxConcurrentHEICDecodes {
		heicSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for range maxConcurrentHEICDecodes {
			<-heicSlots
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := HEICToJPEG(ctx, readHEICFixture(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want the context's error", err)
	}
}
