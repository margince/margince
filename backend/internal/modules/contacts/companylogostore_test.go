// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
)

// letterboxedWordmark is a 4:1 wordmark centred in a square transparent
// canvas — the shape older uploads were stored in.
func letterboxedWordmark(t *testing.T) []byte {
	t.Helper()
	wide := image.NewNRGBA(image.Rect(0, 0, 32, 8))
	for y := range 8 {
		for x := range 32 {
			wide.SetNRGBA(x, y, color.NRGBA{R: 255, G: 90, A: 255})
		}
	}
	square, err := imagenorm.SquarePNG(wide, 32)
	if err != nil {
		t.Fatalf("encoding a letterboxed wordmark: %v", err)
	}
	return square
}

func storedObject(t *testing.T, blob blobstore.Store, key string) []byte {
	t.Helper()
	rc, _, err := blob.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("reading %q back: %v", key, err)
	}
	stored, err := io.ReadAll(rc)
	if cerr := rc.Close(); cerr != nil {
		t.Fatalf("closing %q: %v", key, cerr)
	}
	if err != nil {
		t.Fatalf("reading %q's bytes: %v", key, err)
	}
	return stored
}

func TestPutLogoStoresTheTrimmedMarkUnderAKeyThatSaysSo(t *testing.T) {
	blob := blobstore.NewMemory()
	key, err := PutLogo(context.Background(), blob, "ws/company_logo/c/1", letterboxedWordmark(t))
	if err != nil {
		t.Fatalf("PutLogo: %v", err)
	}
	if !storedTrimmed(key) || !strings.HasPrefix(key, "ws/company_logo/c/1") {
		t.Fatalf("stored at %q, want the base key marked as trimmed", key)
	}
	stored := storedObject(t, blob, key)
	decoded, err := png.Decode(bytes.NewReader(stored))
	if err != nil {
		t.Fatalf("the stored object is not a PNG: %v", err)
	}
	if bounds := decoded.Bounds(); bounds.Dx() != 32 || bounds.Dy() != 8 {
		t.Fatalf("stored mark is %v, want the 32x8 wordmark without its canvas", bounds)
	}
	again, err := imagenorm.TrimTransparentPNG(stored)
	if err != nil {
		t.Fatalf("trimming the stored mark: %v", err)
	}
	if !bytes.Equal(again, stored) {
		t.Fatal("a second trim changed the stored bytes; the serve path relies on them being final")
	}
}

func TestPutLogoRefusesBytesThatAreNotAnImageAndStoresNothing(t *testing.T) {
	blob := blobstore.NewMemory()
	key, err := PutLogo(context.Background(), blob, "ws/company_logo/c/2", []byte("not a png"))
	if err == nil {
		t.Fatalf("PutLogo stored undecodable bytes at %q", key)
	}
	if key != "" {
		t.Fatalf("a refused trim answered key %q, want none: nothing was written to collect", key)
	}
}

func TestTightLogoKeysEmptiesRatherThanGrowPastItsCap(t *testing.T) {
	tight := newTightLogoKeys()
	for i := range tightLogoKeysCap {
		tight.add("legacy/" + strconv.Itoa(i))
	}
	if !tight.has("legacy/0") {
		t.Fatal("a key added below the cap was forgotten")
	}
	tight.add("legacy/one-more")
	if tight.has("legacy/0") {
		t.Fatal("the set kept growing past its cap")
	}
	if !tight.has("legacy/one-more") {
		t.Fatal("the key that hit the cap was not kept after the reset")
	}
}
