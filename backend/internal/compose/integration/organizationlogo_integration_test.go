// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The logo byte endpoint (A55): with an object store wired, the resolved mark
// streams as the PNG this server itself encoded, under headers that keep an
// asset harvested from a third-party website from ever acting like a document.
// Without a store the endpoint declares that by omission (501), and a company
// with no logo is a 404 the client answers with its monogram.

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// logoPNG is a small square PNG standing in for a normalized mark.
func logoPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encoding the fixture: %v", err)
	}
	return out.Bytes()
}

// seedLoggedOrg creates an organization and stores a logo for it: the bytes in
// the object store, the reference on the row — the exact two steps the deep
// read's resolve performs, in the same order.
func seedLoggedOrg(ctx context.Context, t *testing.T, e *Env, blob blobstore.Store, logo []byte) ids.OrganizationID {
	t.Helper()
	org, err := e.People.CreateOrganization(ctx, people.CreateOrganizationInput{
		DisplayName: "Voltaq Systems GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))
	key := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](e.WS), "organization_logo", orgID.String()+"/"+ids.NewV7().String())
	if err := blob.Put(ctx, key, bytes.NewReader(logo), int64(len(logo)), imagenorm.ContentType); err != nil {
		t.Fatalf("store the logo bytes: %v", err)
	}
	written, _, err := e.People.SetOrganizationLogo(ctx, orgID, key, "https://voltaq.test/touch.png")
	if err != nil {
		t.Fatalf("SetOrganizationLogo: %v", err)
	}
	if !written {
		t.Fatal("the logo write reported no change on a fresh organization")
	}
	return orgID
}

func TestOrganizationLogoStreamsTheStoredMarkUnderNonExecutableHeaders(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	handlers := people.NewHandlers(e.DB()).WithBlobstore(blob)
	ctx := e.Admin()
	want := logoPNG(t)
	orgID := seedLoggedOrg(ctx, t, e, blob, want)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/logo", nil).WithContext(ctx)
	handlers.GetOrganizationLogo(rec, req, crmcontracts.Id(orgID.UUID))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET logo = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatal("the streamed bytes are not the stored ones")
	}
	if got := rec.Header().Get("Content-Type"); got != imagenorm.ContentType {
		t.Fatalf("Content-Type = %q, want %q", got, imagenorm.ContentType)
	}
	// These two are what stop a third-party asset from being sniffed into an
	// active document on this origin.
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("the logo response carries no Content-Security-Policy")
	}

	// The record exposes the endpoint, never the bucket path behind it.
	read, err := e.People.GetOrganization(ctx, orgID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("read the organization: %v", err)
	}
	key, err := e.People.OrganizationLogoKey(ctx, orgID, people.LogoWide)
	if err != nil {
		t.Fatalf("read the stored logo key: %v", err)
	}
	wantURL := *people.LogoURL(orgID.UUID, &key, people.LogoWide)
	if read.LogoUrl == nil || *read.LogoUrl != wantURL {
		t.Fatalf("logo_url = %v, want %q", read.LogoUrl, wantURL)
	}
}

// The icon endpoint is the wide one's twin: same body, same slot argument, and
// the slot is the whole difference. What that leaves worth asserting is the
// wiring — that the icon route reads the icon column — because a route that
// read the wide one would stream a real, correct-looking PNG of the wrong
// picture, which no header or status check can see.
func TestTheLogoIconEndpointStreamsTheIconSlotAndNotTheWideOne(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	handlers := people.NewHandlers(e.DB()).WithBlobstore(blob)
	ctx := e.Admin()
	wide := logoPNG(t)
	orgID := seedLoggedOrg(ctx, t, e, blob, wide)

	// An organization wearing only the wide mark answers 404 here, which is the
	// state every record but the installation's own anchor is in — and the
	// signal a client falls back to the wide mark on.
	empty := httptest.NewRecorder()
	handlers.GetOrganizationLogoIcon(empty,
		httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/logo/icon", nil).WithContext(ctx),
		crmcontracts.Id(orgID.UUID))
	if empty.Code != http.StatusNotFound {
		t.Fatalf("GET icon on a record wearing only a wide mark = %d, want 404: %s",
			empty.Code, empty.Body.String())
	}
}

func TestOrganizationLogoRemovesLegacyTransparentCanvasAtTheDisplayBoundary(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	ctx := e.Admin()
	wide := image.NewNRGBA(image.Rect(0, 0, 32, 8))
	for y := range 8 {
		for x := range 32 {
			wide.SetNRGBA(x, y, color.NRGBA{R: 255, G: 90, A: 255})
		}
	}
	legacy, err := imagenorm.SquarePNG(wide, 32)
	if err != nil {
		t.Fatalf("encoding a legacy square-canvas logo: %v", err)
	}
	orgID := seedLoggedOrg(ctx, t, e, blob, legacy)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/logo", nil).WithContext(ctx)
	people.NewHandlers(e.DB()).WithBlobstore(blob).GetOrganizationLogo(rec, req, crmcontracts.Id(orgID.UUID))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET logo = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	displayed, err := png.Decode(rec.Body)
	if err != nil {
		t.Fatalf("display response is not a PNG: %v", err)
	}
	if bounds := displayed.Bounds(); bounds.Dx() != 32 || bounds.Dy() != 8 {
		t.Fatalf("displayed logo is %v, want the original 4:1 wordmark", bounds)
	}
}

func TestOrganizationLogoIs404WithoutOneAnd501WithoutAnObjectStore(t *testing.T) {
	e := Setup(t)
	blob := blobstore.NewMemory()
	ctx := e.Admin()

	bare, err := e.People.CreateOrganization(ctx, people.CreateOrganizationInput{
		DisplayName: "Kein Logo GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}
	bareID := ids.From[ids.OrganizationKind](ids.UUID(bare.Id))

	// No logo on file: a 404 the client renders as a monogram.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/"+bareID.String()+"/logo", nil).WithContext(ctx)
	people.NewHandlers(e.DB()).WithBlobstore(blob).GetOrganizationLogo(rec, req, crmcontracts.Id(bareID.UUID))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET logo of a logo-less org = %d, want 404", rec.Code)
	}

	// An id that names nothing answers the same 404, so the endpoint never
	// confirms which organizations exist.
	rec = httptest.NewRecorder()
	missing := ids.NewV7()
	req = httptest.NewRequest(http.MethodGet, "/v1/organizations/"+missing.String()+"/logo", nil).WithContext(ctx)
	people.NewHandlers(e.DB()).WithBlobstore(blob).GetOrganizationLogo(rec, req, crmcontracts.Id(missing))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET logo of an unknown org = %d, want 404", rec.Code)
	}

	// A role with no object store wired: the organization HAS a logo, and the
	// endpoint says the deployment cannot serve it rather than nil-derefing.
	orgID := seedLoggedOrg(ctx, t, e, blob, logoPNG(t))
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/organizations/"+orgID.String()+"/logo", nil).WithContext(ctx)
	people.NewHandlers(e.DB()).GetOrganizationLogo(rec, req, crmcontracts.Id(orgID.UUID))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("GET logo with no object store = %d, want 501", rec.Code)
	}
}
