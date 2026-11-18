// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The installation's name and marks, as every seat's brand block reads them.
// A rep may read the brand and load both marks, and may still not read the
// profile the brand is taken from.

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestARepReadsTheInstallationsNameAndMarksAndNothingElse(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	handlers := companyHandlers{store: e.Contacts, blob: blob}
	read := installationBrand(e.Contacts)
	rep := e.As(e.Rep2, nil, integration.AccountRepPerms)

	if brand, err := read(rep); err != nil || brand != nil {
		t.Fatalf("brand before the installation described itself = %+v (%v), want none", brand, err)
	}

	company := theCompanyExists(t, e)
	uploadMark(t, e, handlers, logoFixture(t, 800, 200), "acme-wordmark.png")
	uploadIcon(t, e, handlers, logoFixture(t, 256, 256), "acme-badge.png")

	brand, err := read(rep)
	if err != nil || brand == nil {
		t.Fatalf("a rep's brand read = %+v (%v), want the company", brand, err)
	}
	if brand.DisplayName != "Acme GmbH" {
		t.Errorf("display_name = %q, want Acme GmbH", brand.DisplayName)
	}
	if brand.LogoUrl == nil || brand.LogoIconUrl == nil {
		t.Fatalf("logo_url = %v, logo_icon_url = %v, want both marks", brand.LogoUrl, brand.LogoIconUrl)
	}

	// Each URL is loaded as the rep, through the route it names: a URL the
	// stream then refused would draw a broken image in every non-admin rail.
	serve := contacts.NewHandlers(e.DB()).WithBlobstore(blob)
	id := crmcontracts.Id(company.CompanyID.UUID)
	streamedMark(rep, t, func(recorder *httptest.ResponseRecorder, request *http.Request) {
		serve.GetCompanyLogo(recorder, request, id)
	}, *brand.LogoUrl)
	streamedMark(rep, t, func(recorder *httptest.ResponseRecorder, request *http.Request) {
		serve.GetCompanyLogoIcon(recorder, request, id)
	}, *brand.LogoIconUrl)

	raw, err := json.Marshal(brand)
	if err != nil {
		t.Fatalf("encoding the brand: %v", err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("decoding the brand: %v", err)
	}
	if keys := slices.Sorted(maps.Keys(wire)); !slices.Equal(keys, []string{"display_name", "logo_icon_url", "logo_url"}) {
		t.Errorf("the brand carries %v, want the name and the two marks only", keys)
	}

	if _, err := e.Contacts.GetAnchorCompany(rep); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep's read of the full profile = %v, want permission denied", err)
	}
}

// A seat that may not read companies cannot load a mark, so it is handed the
// name alone and draws the monogram rather than an image the server refuses.
func TestASeatThatCannotLoadTheMarkGetsTheNameAlone(t *testing.T) {
	e := integration.Setup(t)
	handlers := companyHandlers{store: e.Contacts, blob: blobstore.NewMemory()}
	company := theCompanyExists(t, e)
	uploadMark(t, e, handlers, logoFixture(t, 800, 200), "acme-wordmark.png")
	seat := e.As(e.Rep2, nil, integration.RepPerms)

	if _, err := e.Contacts.CompanyLogoKey(seat, company.CompanyID, contacts.LogoWide); err == nil {
		t.Fatal("the logo stream admits a seat with no company read, so this case proves nothing")
	}
	brand, err := installationBrand(e.Contacts)(seat)
	if err != nil || brand == nil {
		t.Fatalf("brand = %+v (%v), want the company's name", brand, err)
	}
	if brand.DisplayName != "Acme GmbH" || brand.LogoUrl != nil || brand.LogoIconUrl != nil {
		t.Errorf("brand = %q, %v, %v, want the name and no mark", brand.DisplayName, brand.LogoUrl, brand.LogoIconUrl)
	}
}
