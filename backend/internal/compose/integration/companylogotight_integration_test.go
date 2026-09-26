// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Which logo reads skip the decode. A list screen asks for one logo per row,
// so the serve path must send bytes it already knows are final without reading
// them into memory and scanning every pixel first.
//
// Each case proves the skip the only way it is visible from outside: it swaps
// letterboxed bytes in under a key the handler believes is tight, and the
// response comes back letterboxed. A handler that still decoded would crop them.

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func letterboxedLogo(t *testing.T) []byte {
	t.Helper()
	wide := image.NewNRGBA(image.Rect(0, 0, 32, 8))
	for y := range 8 {
		for x := range 32 {
			wide.SetNRGBA(x, y, color.NRGBA{R: 255, G: 90, A: 255})
		}
	}
	square, err := imagenorm.SquarePNG(wide, 32)
	if err != nil {
		t.Fatalf("encoding a letterboxed logo: %v", err)
	}
	return square
}

func overwriteObject(ctx context.Context, t *testing.T, blob blobstore.Store, key string, logo []byte) {
	t.Helper()
	if err := blob.Put(ctx, key, bytes.NewReader(logo), int64(len(logo)), imagenorm.ContentType); err != nil {
		t.Fatalf("overwriting %q: %v", key, err)
	}
}

func getLogo(ctx context.Context, t *testing.T, handlers contacts.Handlers, companyID ids.CompanyID) []byte {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/companies/"+companyID.String()+"/logo", nil).WithContext(ctx)
	handlers.GetCompanyLogo(rec, req, crmcontracts.Id(companyID.UUID))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET logo = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func TestAMarkStoredTrimmedIsServedWithoutADecode(t *testing.T) {
	e := Setup(t)
	blob := newCountingBlobstore()
	handlers := contacts.NewHandlers(e.DB()).WithBlobstore(blob)
	ctx := e.Admin()
	company, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: "Voltaq Systems GmbH", Source: "manual"})
	if err != nil {
		t.Fatalf("seed company: %v", err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	base := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](e.WS), "company_logo", companyID.String()+"/"+ids.NewV7().String())
	key, err := contacts.PutLogo(ctx, blob, base, logoPNG(t))
	if err != nil {
		t.Fatalf("PutLogo: %v", err)
	}
	if _, _, err := e.Contacts.SetCompanyLogo(ctx, companyID, key, "https://voltaq.test/touch.png"); err != nil {
		t.Fatalf("SetCompanyLogo: %v", err)
	}
	letterboxed := letterboxedLogo(t)
	overwriteObject(ctx, t, blob, key, letterboxed)
	puts := blob.putCount(key)

	if !bytes.Equal(getLogo(ctx, t, handlers, companyID), letterboxed) {
		t.Fatal("a key PutLogo stored was decoded and cropped; it must be served as stored")
	}
	if got := blob.putCount(key); got != puts {
		t.Fatalf("serving a trimmed key wrote to it %d time(s)", got-puts)
	}
}

func TestALegacyMarkIsDecodedOncePerProcess(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	handlers := contacts.NewHandlers(e.DB()).WithBlobstore(blob)
	ctx := e.Admin()
	companyID := seedLoggedCompany(ctx, t, e, blob, logoPNG(t))
	key, err := e.Contacts.CompanyLogoKey(ctx, companyID, contacts.LogoWide)
	if err != nil {
		t.Fatalf("read the stored logo key: %v", err)
	}

	if !bytes.Equal(getLogo(ctx, t, handlers, companyID), logoPNG(t)) {
		t.Fatal("the first read of an already tight legacy mark changed its bytes")
	}
	letterboxed := letterboxedLogo(t)
	overwriteObject(ctx, t, blob, key, letterboxed)
	if !bytes.Equal(getLogo(ctx, t, handlers, companyID), letterboxed) {
		t.Fatal("a legacy key this process already found tight was decoded again")
	}

	// A fresh process has checked nothing yet, so it still crops.
	fresh := contacts.NewHandlers(e.DB()).WithBlobstore(blob)
	if bytes.Equal(getLogo(ctx, t, fresh, companyID), letterboxed) {
		t.Fatal("a process that never checked this legacy key served it uncropped")
	}
}
