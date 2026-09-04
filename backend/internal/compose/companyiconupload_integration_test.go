// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The square badge's own transport, end to end.
//
// The two upload routes share one decode, one ceiling and one store verb, and
// that sharing is exactly the arrangement in which a mis-wired slot is
// invisible: an icon upload that wrote the wide column would return 200, store
// real bytes, and answer a plausible URL — while quietly taking away the
// wordmark a person had chosen. So every case here watches BOTH slots, and the
// assertion that matters most is about the one the request did not name.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

// uploadIcon drives the icon route the way uploadMark drives the wide one — the
// real transport, because the parse, the re-encode and the object write are all
// this side of the store.
func uploadIcon(t *testing.T, e *integration.Env, handlers companyHandlers, image []byte, filename string) {
	t.Helper()
	body, contentType := markUpload(t, image, filename)
	request := httptest.NewRequest(http.MethodPost, "/v1/company/logo/icon", body).
		WithContext(e.As(e.Rep1, nil, integration.AdminPerms))
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()
	handlers.UploadCompanyLogoIcon(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("icon upload → %d %s, want 200", recorder.Code, recorder.Body.String())
	}
}

func TestTheSquareBadgeIsStoredWithoutDisturbingTheWideMark(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	handlers := companyHandlers{store: e.People, blob: blob}
	company := theCompanyExists(t, e)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	uploadMark(t, e, handlers, logoFixture(t, 800, 200), "acme-wordmark.png")
	wide, err := e.People.OrganizationLogoKey(ctx, company.OrganizationID, people.LogoWide)
	if err != nil {
		t.Fatalf("the company wears no wide mark after its own upload: %v", err)
	}

	uploadIcon(t, e, handlers, logoFixture(t, 256, 256), "acme-badge.png")

	icon, err := e.People.OrganizationLogoKey(ctx, company.OrganizationID, people.LogoIcon)
	if err != nil {
		t.Fatalf("the company wears no badge after its own upload: %v", err)
	}
	if icon == wide {
		t.Fatal("both slots name one object, so the badge upload overwrote the wordmark")
	}
	if stillWide, keyErr := e.People.OrganizationLogoKey(ctx, company.OrganizationID, people.LogoWide); keyErr != nil || stillWide != wide {
		t.Fatalf("the wide mark is now %q (%v), want the untouched %q", stillWide, keyErr, wide)
	}
	// Both objects are in the store: the badge's bytes were written, and the
	// wordmark's were not collected as though the badge had superseded them.
	for name, key := range map[string]string{"the wordmark": wide, "the badge": icon} {
		reader, _, getErr := blob.Get(context.Background(), key)
		if getErr != nil {
			t.Fatalf("%s names no stored object: %v", name, getErr)
		}
		if closeErr := reader.Close(); closeErr != nil {
			t.Fatalf("closing %s: %v", name, closeErr)
		}
	}

	// The profile answers each slot's own endpoint, which is what the sidebar
	// picks between at its two widths.
	read, err := e.People.GetCompany(ctx)
	if err != nil {
		t.Fatalf("reading the company back: %v", err)
	}
	profile := toContractCompany(read)
	wantIcon := *people.LogoURL(company.OrganizationID.UUID, &icon, people.LogoIcon)
	if profile.LogoIconUrl == nil || *profile.LogoIconUrl != wantIcon {
		t.Fatalf("logo_icon_url = %v, want %q", profile.LogoIconUrl, wantIcon)
	}
	wantWide := *people.LogoURL(company.OrganizationID.UUID, &wide, people.LogoWide)
	if profile.LogoUrl == nil || *profile.LogoUrl != wantWide {
		t.Fatalf("logo_url = %v, want the untouched %q", profile.LogoUrl, wantWide)
	}

	// And each endpoint serves its own slot's bytes. A route that read the other
	// column would stream a real, correct-looking PNG of the wrong picture,
	// which no status or header check can see — so the two responses are
	// compared against each other rather than merely inspected.
	serve := people.NewHandlers(e.DB()).WithBlobstore(blob)
	badge := streamedMark(t, ctx, func(recorder *httptest.ResponseRecorder, request *http.Request) {
		serve.GetOrganizationLogoIcon(recorder, request, crmcontracts.Id(company.OrganizationID.UUID))
	}, "/v1/organizations/"+company.OrganizationID.String()+"/logo/icon")
	wordmark := streamedMark(t, ctx, func(recorder *httptest.ResponseRecorder, request *http.Request) {
		serve.GetOrganizationLogo(recorder, request, crmcontracts.Id(company.OrganizationID.UUID))
	}, "/v1/organizations/"+company.OrganizationID.String()+"/logo")
	if bytes.Equal(badge, wordmark) {
		t.Fatal("the two endpoints stream identical bytes — one of them is reading the other's column")
	}
}

// streamedMark runs one logo endpoint and answers the bytes it wrote.
func streamedMark(t *testing.T, ctx context.Context, serve func(*httptest.ResponseRecorder, *http.Request), path string) []byte {
	t.Helper()
	recorder := httptest.NewRecorder()
	serve(recorder, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s → %d %s, want 200", path, recorder.Code, recorder.Body.String())
	}
	if len(recorder.Body.Bytes()) == 0 {
		t.Fatalf("GET %s streamed no bytes", path)
	}
	return recorder.Body.Bytes()
}

func TestRemovingTheBadgeLeavesTheWideMarkStanding(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	handlers := companyHandlers{store: e.People, blob: blob}
	company := theCompanyExists(t, e)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	uploadMark(t, e, handlers, logoFixture(t, 800, 200), "acme-wordmark.png")
	uploadIcon(t, e, handlers, logoFixture(t, 256, 256), "acme-badge.png")
	icon, err := e.People.OrganizationLogoKey(ctx, company.OrganizationID, people.LogoIcon)
	if err != nil {
		t.Fatalf("reading the uploaded badge: %v", err)
	}

	recorder := httptest.NewRecorder()
	handlers.DeleteCompanyLogoIcon(recorder,
		httptest.NewRequest(http.MethodDelete, "/v1/company/logo/icon", nil).WithContext(ctx))
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete → %d %s, want 200", recorder.Code, recorder.Body.String())
	}
	cleared := decodeCompany(t, recorder)
	if cleared.LogoIconUrl != nil {
		t.Fatalf("logo_icon_url = %q after a removal, want none so the rail falls back", *cleared.LogoIconUrl)
	}
	// The fallback the collapsed rail depends on: taking the badge off must not
	// take the company's face with it.
	if cleared.LogoUrl == nil {
		t.Fatal("removing the badge cleared the wide mark too — the collapsed rail has nothing to fall back to")
	}
	// The bytes go with the reference. An object nothing names is storage
	// nobody can ever reach.
	if _, _, getErr := blob.Get(context.Background(), icon); getErr == nil {
		t.Fatal("the removed badge's bytes are still in the store")
	}
}
