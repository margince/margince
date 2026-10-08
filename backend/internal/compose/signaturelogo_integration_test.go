// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A large uploaded logo goes out fitted to the 300px edge a signature embeds,
// and a workspace with no logo embeds nothing.
func TestTheSignatureLogoIsTheWorkspaceLogoSizedForMail(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	logo := newSignatureLogo(contacts.NewStore(InstallationDB(e.Pool)), blob)

	if _, ok, err := logo.SignatureLogo(e.Admin()); err != nil || ok {
		t.Fatalf("a workspace with no logo embedded one: ok=%v err=%v", ok, err)
	}

	wide := image.NewRGBA(image.Rect(0, 0, 1200, 400))
	for x := range 1200 {
		wide.Set(x, 200, color.RGBA{R: 40, G: 160, B: 110, A: 255})
	}
	var original bytes.Buffer
	if err := png.Encode(&original, wide); err != nil {
		t.Fatal(err)
	}
	key := "logos/" + ids.NewV7().String() + ".png"
	if err := blob.Put(context.Background(), key, bytes.NewReader(original.Bytes()), int64(original.Len()), "image/png"); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `INSERT INTO company (id, display_name, is_anchor, logo_object_key, source, captured_by)
		VALUES ($1, 'Company A', true, $2, 'manual', 'human:x')`, ids.NewV7(), key)

	got, ok, err := logo.SignatureLogo(e.Admin())
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
