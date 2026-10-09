// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/margince/margince/backend/internal/platform/blobstore"
)

func storeLogo(t *testing.T, blob blobstore.Store, key string, width, height int) {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := range width {
		picture.Set(x, height/2, color.RGBA{R: 40, G: 160, B: 110, A: 255})
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatal(err)
	}
	if err := blob.Put(context.Background(), key, bytes.NewReader(encoded.Bytes()), int64(encoded.Len()), "image/png"); err != nil {
		t.Fatal(err)
	}
}

// A large stored logo goes out fitted to the 300px edge a signature embeds.
func TestTheSignatureLogoIsSizedForMail(t *testing.T) {
	t.Parallel()
	blob := blobstore.NewMemory()
	storeLogo(t, blob, "logos/wide.png", 1200, 400)
	got, ok, err := newSignatureLogo(blob).SignatureLogo(context.Background(), "logos/wide.png")
	if err != nil || !ok {
		t.Fatalf("reading the logo: ok=%v err=%v", ok, err)
	}
	sized, err := png.Decode(bytes.NewReader(got.Body))
	if err != nil {
		t.Fatalf("the embedded logo is not a PNG: %v", err)
	}
	if bounds := sized.Bounds(); bounds.Dx() > signatureLogoEdge || bounds.Dy() > signatureLogoEdge {
		t.Fatalf("embedded logo is %dx%d, want at most %d on its longest edge", bounds.Dx(), bounds.Dy(), signatureLogoEdge)
	}
}

// A logo deleted since the message was staged embeds nothing and fails nothing.
func TestADeletedLogoEmbedsNothing(t *testing.T) {
	t.Parallel()
	_, ok, err := newSignatureLogo(blobstore.NewMemory()).SignatureLogo(context.Background(), "logos/gone.png")
	if err != nil || ok {
		t.Fatalf("a deleted logo: ok=%v err=%v", ok, err)
	}
}

// Replaced logos do not pile up in memory: the cache holds a bounded set.
func TestTheSizedLogoCacheIsBounded(t *testing.T) {
	t.Parallel()
	blob := blobstore.NewMemory()
	logo := newSignatureLogo(blob)
	for i := range signatureLogoCacheSize * 2 {
		key := fmt.Sprintf("logos/%d.png", i)
		storeLogo(t, blob, key, 10, 10)
		if _, ok, err := logo.SignatureLogo(context.Background(), key); err != nil || !ok {
			t.Fatalf("reading %s: ok=%v err=%v", key, ok, err)
		}
	}
	if len(logo.sized) > signatureLogoCacheSize {
		t.Fatalf("the cache holds %d logos, want at most %d", len(logo.sized), signatureLogoCacheSize)
	}
}
