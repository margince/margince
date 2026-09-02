// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A person putting their own mark on the installation's company, and taking it
// off again.
//
// What these cases hold is the PRECEDENCE between the two writers of this one
// field: an upload outranks what a website read resolves, and a removal gives
// the field back rather than standing as a permanent hold — otherwise a company
// whose logo somebody once cleared could never be given a face by any read
// again.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
)

// theCompanyExists gives the installation the record a mark can be put on.
func theCompanyExists(t *testing.T, e *integration.Env) people.Company {
	t.Helper()
	offer, icp := "Revenue operations software", "RevOps at SaaS scale-ups"
	company, err := e.People.SaveCompany(e.As(e.Rep1, nil, integration.AdminPerms),
		people.SaveCompanyInput{
			DisplayName: "Acme GmbH",
			Fields:      map[string]*string{"offer_summary": &offer, "icp": &icp},
		})
	if err != nil {
		t.Fatalf("save the company: %v", err)
	}
	return company
}

// uploadMark drives the real transport, because the parse, the re-encode and
// the object write are all this side of the store — a case that called the
// store directly would prove nothing about what an upload actually stores.
func uploadMark(t *testing.T, e *integration.Env, handlers companyHandlers, image []byte, filename string) crmcontracts.CompanyProfile {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("building the upload: %v", err)
	}
	if _, err := part.Write(image); err != nil {
		t.Fatalf("writing the upload: %v", err)
	}
	if err := form.Close(); err != nil {
		t.Fatalf("closing the upload: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/company/logo", &body).
		WithContext(e.As(e.Rep1, nil, integration.AdminPerms))
	request.Header.Set("Content-Type", form.FormDataContentType())
	recorder := httptest.NewRecorder()
	handlers.UploadCompanyLogo(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload → %d %s, want 200", recorder.Code, recorder.Body.String())
	}
	return decodeCompany(t, recorder)
}

func decodeCompany(t *testing.T, recorder *httptest.ResponseRecorder) crmcontracts.CompanyProfile {
	t.Helper()
	var company crmcontracts.CompanyProfile
	if err := json.Unmarshal(recorder.Body.Bytes(), &company); err != nil {
		t.Fatalf("decoding the company: %v", err)
	}
	return company
}

func TestAnUploadedMarkIsStoredAsThisServersOwnPNG(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	handlers := companyHandlers{store: e.People, blob: blob}
	company := theCompanyExists(t, e)

	uploaded := uploadMark(t, e, handlers, logoFixture(t, 400, 400), "acme-logo.png")

	wantURL := "/v1/organizations/" + company.OrganizationID.String() + "/logo"
	if uploaded.LogoUrl == nil || *uploaded.LogoUrl != wantURL {
		t.Fatalf("logo_url = %v, want %q — the face the shell and the record both render",
			uploaded.LogoUrl, wantURL)
	}
	key, err := e.People.OrganizationLogoKey(e.As(e.Rep1, nil, integration.AdminPerms), company.OrganizationID)
	if err != nil {
		t.Fatalf("the company wears no mark after its own upload: %v", err)
	}
	stored, object, err := blob.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("the row names no stored object: %v", err)
	}
	defer func() {
		if cerr := stored.Close(); cerr != nil {
			t.Fatalf("closing the stored object: %v", cerr)
		}
	}()
	if object.ContentType != imagenorm.ContentType {
		t.Fatalf("stored content type %q, want the normalized %q", object.ContentType, imagenorm.ContentType)
	}
	// The bytes are this server's re-encode, not the upload echoed back. What a
	// person hands over is never what the next viewer is served.
	raw, err := io.ReadAll(stored)
	if err != nil {
		t.Fatalf("reading the stored object: %v", err)
	}
	decoded, err := imagenorm.Decode(raw)
	if err != nil {
		t.Fatalf("the stored mark does not decode: %v", err)
	}
	if bounds := decoded.Bounds(); bounds.Dx() != bounds.Dy() {
		t.Fatalf("the stored mark is %dx%d, want a square", bounds.Dx(), bounds.Dy())
	}
}

func TestAPersonsMarkOutranksWhatAWebsiteReadResolves(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	handlers := companyHandlers{store: e.People, blob: blob}
	company := theCompanyExists(t, e)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	uploadMark(t, e, handlers, logoFixture(t, 400, 400), "acme-logo.png")
	chosen, err := e.People.OrganizationLogoKey(ctx, company.OrganizationID)
	if err != nil {
		t.Fatalf("reading the uploaded mark: %v", err)
	}

	// A read resolving a different mark afterwards. It reports that it wrote
	// nothing and hands its own object back, which is what lets the resolve lane
	// collect bytes the record never took.
	written, superseded, err := e.People.SetOrganizationLogo(ctx, company.OrganizationID,
		chosen+"-from-the-site", touchIconURL)
	if err != nil {
		t.Fatalf("the resolve failed outright: %v", err)
	}
	if written {
		t.Fatal("a website read overwrote the mark a person chose")
	}
	if superseded != nil {
		t.Fatalf("a declined resolve reported %q as superseded, want nothing", *superseded)
	}
	after, err := e.People.OrganizationLogoKey(ctx, company.OrganizationID)
	if err != nil {
		t.Fatalf("reading the mark after the resolve: %v", err)
	}
	if after != chosen {
		t.Fatalf("the company wears %q, want the uploaded %q", after, chosen)
	}
}

func TestRemovingAMarkGivesTheFieldBackToTheNextRead(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	handlers := companyHandlers{store: e.People, blob: blob}
	company := theCompanyExists(t, e)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	uploadMark(t, e, handlers, logoFixture(t, 400, 400), "acme-logo.png")
	uploaded, err := e.People.OrganizationLogoKey(ctx, company.OrganizationID)
	if err != nil {
		t.Fatalf("reading the uploaded mark: %v", err)
	}

	recorder := httptest.NewRecorder()
	handlers.DeleteCompanyLogo(recorder,
		httptest.NewRequest(http.MethodDelete, "/v1/company/logo", nil).WithContext(ctx))
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete → %d %s, want 200", recorder.Code, recorder.Body.String())
	}
	if cleared := decodeCompany(t, recorder); cleared.LogoUrl != nil {
		t.Fatalf("logo_url = %q after a removal, want none so the monogram renders", *cleared.LogoUrl)
	}
	// The bytes go with the reference. An object nothing names is storage
	// nobody can ever reach.
	if _, _, err := blob.Get(context.Background(), uploaded); err == nil {
		t.Fatal("the removed mark's bytes are still in the store")
	}

	// And the field is genuinely back: the removal is not a standing hold, so a
	// later read may give this company a face again.
	written, _, err := e.People.SetOrganizationLogo(ctx, company.OrganizationID,
		uploaded+"-from-the-site", touchIconURL)
	if err != nil {
		t.Fatalf("the resolve after a removal failed: %v", err)
	}
	if !written {
		t.Fatal("a removal left the field held, so no read can ever give this company a mark again")
	}
}

// The other half of the precedence rule, and the reason it is not simply "the
// first mark wins": a company can be read AGAIN, from the settings card, to
// pick up what its website says now. A re-read whose whole point is a fresher
// logo has to be allowed to land one.
func TestReReadingTheCompanysSiteReplacesTheMarkAnEarlierReadLanded(t *testing.T) {
	e := integration.Setup(t)
	human := e.As(e.Rep1, nil, integration.AdminPerms)
	blob := newRecordingBlobstore()
	engine := &deepReadEngine{people: e.People, blob: blob, log: slog.New(slog.DiscardHandler)}

	// The first read, which is onboarding: it creates the company and gives it
	// the mark its site declared. Driven through the real flow rather than
	// planted, because what a planted row cannot carry is the PROVENANCE, and
	// provenance is the whole question here — a mark stamped by hand would be a
	// person's, which is the case that must NOT be replaced.
	firstArgs, first := readTheAnchorsSiteFor(t, e, blob)
	created := confirmTheAnchorAsTheAPIDoes(t, e, engine, firstArgs)
	if wearing, err := e.People.OrganizationLogoKey(human, created); err != nil || wearing != first {
		t.Fatalf("after onboarding the company wears %q (%v), want the first read's mark at %q — "+
			"this case has no replacement to observe otherwise", wearing, err, first)
	}

	// The second read, which is the settings card's refresh against a company
	// that already exists.
	args, parked := readTheAnchorsSiteFor(t, e, blob)
	orgID := confirmTheAnchorAsTheAPIDoes(t, e, engine, args)
	// The SAME company, which is what makes this a re-read rather than two
	// unrelated onboardings: a workspace has at most one live anchor
	// (uq_organization_anchor), so the second confirmation resolves the record
	// the first one created instead of minting a rival.
	if orgID != created {
		t.Fatalf("the second confirmation landed on %s, want the company the first created (%s)",
			orgID, created)
	}
	if parked == first {
		t.Fatal("both reads parked one object, so this case cannot tell a replacement from a no-op")
	}
	stale := first

	wearing, err := e.People.OrganizationLogoKey(human, orgID)
	if err != nil {
		t.Fatalf("the company lost its logo to the re-read: %v", err)
	}
	if wearing != parked {
		t.Fatalf("the company wears %q, want the re-read's own mark at %q — a refresh that "+
			"resolves a new logo and then keeps the old one reports work it did not do",
			wearing, parked)
	}
	// The mark it stopped wearing is the object nothing references now, and the
	// transport collects exactly that one.
	if _, _, err := blob.Get(context.Background(), stale); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("the superseded object at %q answers %v, want it collected", stale, err)
	}
	if n := rowsNaming(t, e, stale); n != 0 {
		t.Fatalf("%d row(s) still name the collected object at %q", n, stale)
	}
}
