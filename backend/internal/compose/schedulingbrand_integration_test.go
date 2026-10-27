// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestBookingBrandUsesTheCurrentAnchorAndServesItsLogoWithoutLogin(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	theCompanyExists(t, e)
	uploadMark(t, e, companyHandlers{store: e.Contacts, blob: blob}, logoFixture(t, 400, 100), "logo.png")
	store := e.Activities.WithSchedulingBrand(e.Contacts)
	obsoleteName, obsoleteLogo := "Obsolete company", "https://old.example/logo.png"
	profile, err := store.SaveSchedulingProfile(e.Admin(), crmcontracts.SchedulingProfile{
		CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30,
		Title: "Let's meet", CompanyName: &obsoleteName, LogoUrl: &obsoleteLogo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.CompanyName == nil || *profile.CompanyName != "Acme GmbH" || profile.LogoUrl == nil {
		t.Fatalf("profile did not use anchor identity: %+v", profile)
	}
	server := New(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)), WithBlobstore(blob))
	logo := httptest.NewRecorder()
	server.ServeHTTP(logo, httptest.NewRequest(http.MethodGet, *profile.LogoUrl, nil))
	if logo.Code != http.StatusOK || logo.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("anonymous logo: %d %s", logo.Code, logo.Body.String())
	}
	if _, err := png.Decode(bytes.NewReader(logo.Body.Bytes())); err != nil {
		t.Fatal(err)
	}
	cached := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, *profile.LogoUrl, nil)
	request.Header.Set("If-None-Match", logo.Header().Get("ETag"))
	server.ServeHTTP(cached, request)
	if cached.Code != http.StatusNotModified || cached.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("public logo did not revalidate: %d", cached.Code)
	}
	for _, method := range []string{http.MethodHead, http.MethodPost} {
		refused := httptest.NewRecorder()
		server.ServeHTTP(refused, httptest.NewRequest(method, *profile.LogoUrl, nil))
		if refused.Code != http.StatusNotFound {
			t.Fatalf("unexpected logo method admitted: %s %d", method, refused.Code)
		}
	}
	if _, err := e.Contacts.SaveCompany(e.Admin(), contacts.SaveCompanyInput{DisplayName: "New anchor name"}); err != nil {
		t.Fatal(err)
	}
	guest := httptest.NewRecorder()
	server.ServeHTTP(guest, httptest.NewRequest(http.MethodGet, "/v1/public/booking/"+*profile.Slug+"/profile", nil))
	var public crmcontracts.PublicSchedulingProfile
	if err := json.Unmarshal(guest.Body.Bytes(), &public); err != nil {
		t.Fatal(err)
	}
	if guest.Code != http.StatusOK || public.CompanyName != "New anchor name" || public.Enabled {
		t.Fatalf("public profile did not follow the anchor or changed paused status: %d %+v", guest.Code, public)
	}
	if _, err := e.Contacts.ClearCompanyLogo(e.Admin(), contacts.LogoWide); err != nil {
		t.Fatal(err)
	}
	missing := httptest.NewRecorder()
	server.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, *profile.LogoUrl, nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("removed logo remains public: %d", missing.Code)
	}
}

func TestBookingBrandWithoutAnAnchorDoesNotReuseObsoleteOverrides(t *testing.T) {
	e := integration.Setup(t)
	name, url := "Former company", "https://old.example/logo.png"
	profile, err := e.Activities.SaveSchedulingProfile(e.Admin(), crmcontracts.SchedulingProfile{
		CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30,
		Title: "Let's meet", CompanyName: &name, LogoUrl: &url,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = e.Activities.WithSchedulingBrand(e.Contacts).SchedulingProfile(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	if profile.CompanyName == nil || *profile.CompanyName != "" || profile.LogoUrl != nil {
		t.Fatalf("obsolete branding still exposed without anchor: %+v", profile)
	}
}

func TestBookingBrandTrimsLegacyLogosWithoutAuthorizingPublicWrites(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	theCompanyExists(t, e)
	// Historical objects predate PutLogo's write-time crop.
	original := letterboxedFixture(t, 400)
	key := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](e.WS), "company_logo", "legacy")
	if err := blob.Put(context.Background(), key, bytes.NewReader(original), int64(len(original)), "image/png"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.SetAnchorCompanyLogo(e.Admin(), contacts.LogoWide, key, "legacy.png"); err != nil {
		t.Fatal(err)
	}
	server := New(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)), WithBlobstore(blob))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/public/booking/company-logo", nil))
	picture, err := png.Decode(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || picture.Bounds().Dx() != 4*picture.Bounds().Dy() {
		t.Fatalf("legacy logo not trimmed: %d %v", response.Code, picture.Bounds())
	}
	reader, _, err := blob.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	stored, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, original) {
		t.Fatal("anonymous read rewrote the stored logo")
	}
}

func TestBookingBrandFollowsTheAnchorOnPersonalProposals(t *testing.T) {
	e := integration.Setup(t)
	theCompanyExists(t, e)
	blob := blobstore.NewMemory()
	uploadMark(t, e, companyHandlers{store: e.Contacts, blob: blob}, logoFixture(t, 400, 100), "logo.png")
	vault := keyvault.NewMemory()
	// The public route reads expiry on the server's clock, so the proposal must be minted on it too.
	store := e.Activities.WithPublicBaseURL("https://crm.example.test").WithMeetingVault(vault).WithSchedulingCalendar(privateCalendar{}).WithSchedulingBrand(e.Contacts)
	if _, err := store.SaveSchedulingProfile(e.Admin(), crmcontracts.SchedulingProfile{Provider: "gcal", CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30, Title: "Discovery"}); err != nil {
		t.Fatal(err)
	}
	contact := e.SeedContact(t, "Proposal guest", nil)
	proposal, err := store.CreateProposal(e.Admin(), crmcontracts.MeetingProposalRequest{ContactId: crmcontracts.Id(contact), AttendeeEmail: "guest@example.test", Subject: "Personal discovery", DurationMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	server := New(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)), WithBlobstore(blob), WithKeyvault(vault), WithPublicBaseURL("https://crm.example.test"))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/public/proposal/"+strings.Split(proposal.URL, "proposal-")[1], nil))
	var public crmcontracts.PublicMeetingProposal
	if err := json.Unmarshal(response.Body.Bytes(), &public); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || public.Profile.CompanyName != "Acme GmbH" || public.Profile.LogoUrl == nil || *public.Profile.LogoUrl != "https://crm.example.test/v1/public/booking/company-logo" {
		t.Fatalf("proposal brand: %d %+v", response.Code, public.Profile)
	}
}

type unavailableBookingBrand struct{ reads int }

func (b *unavailableBookingBrand) PublicBookingBrand(context.Context, ids.UserID) (string, *string, error) {
	b.reads++
	return "", nil, errors.New("branding unavailable")
}

func TestBookingBrandFailureDoesNotFailCommittedSettingsOrAvailability(t *testing.T) {
	e := integration.Setup(t)
	brand := &unavailableBookingBrand{}
	now := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	store := e.Activities.WithClock(func() time.Time { return now }).WithSchedulingBrand(brand).WithSchedulingCalendar(privateCalendar{}).WithWorkingHours(workingHoursResolver(e.Pool))
	saved, err := store.SaveSchedulingProfile(e.Admin(), crmcontracts.SchedulingProfile{Provider: "gcal", CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30, Title: "Saved despite branding"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Title != "Saved despite branding" || saved.CompanyName != nil || saved.LogoUrl != nil || brand.reads != 1 {
		t.Fatalf("save response: %+v; brand reads=%d", saved, brand.reads)
	}
	brand.reads = 0
	slots, _, err := store.ReliableAvailability(e.Admin(), ids.From[ids.UserKind](e.AdminUser), now.Add(3*time.Hour), now.Add(6*time.Hour), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) == 0 || brand.reads != 0 {
		t.Fatalf("availability depends on brand: slots=%d brand reads=%d", len(slots), brand.reads)
	}
}

// Two hosts working for two legal entities each show their own entity on the
// booking page, and an entity's logo is served without login. An entity with no
// logo shows its name and borrows no other entity's.
func TestBookingBrandFollowsTheLegalEntityTheHostWorksFor(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	handlers := companyHandlers{store: e.Contacts, blob: blob}
	theCompanyExists(t, e)
	uploadMark(t, e, handlers, logoFixture(t, 400, 100), "logo.png")
	admin := e.As(e.Rep1, nil, integration.AdminPerms)
	vienna := describeVienna(t, e)
	if recorder := uploadEntityMark(admin, t, handlers, vienna.ID); recorder.Code != http.StatusOK {
		t.Fatalf("upload the entity logo → %d %s", recorder.Code, recorder.Body)
	}
	if err := e.Contacts.AssignLegalEntityMember(admin, vienna.ID, ids.From[ids.UserKind](e.Rep1)); err != nil {
		t.Fatalf("assign: %v", err)
	}
	store := e.Activities.WithSchedulingBrand(e.Contacts)
	profileOf := func(ctx context.Context) crmcontracts.SchedulingProfile {
		t.Helper()
		profile, err := store.SaveSchedulingProfile(ctx, crmcontracts.SchedulingProfile{
			CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30, Title: "Let's meet",
		})
		if err != nil {
			t.Fatal(err)
		}
		return profile
	}

	anchorHost := profileOf(e.Admin())
	if anchorHost.CompanyName == nil || *anchorHost.CompanyName != "Acme GmbH" ||
		anchorHost.LogoUrl == nil || *anchorHost.LogoUrl != "/v1/public/booking/company-logo" {
		t.Fatalf("a default-entity host: %q %q, want the anchor's name and logo", deref(anchorHost.CompanyName), deref(anchorHost.LogoUrl))
	}
	viennaHost := profileOf(e.As(e.Rep1, nil, integration.AdminPerms))
	if viennaHost.CompanyName == nil || *viennaHost.CompanyName != "Wien" ||
		viennaHost.LogoUrl == nil || *viennaHost.LogoUrl != "/v1/public/booking/company-logo?entity="+vienna.ID.String() {
		t.Fatalf("a Vienna host: %q %q, want Wien and its own logo", deref(viennaHost.CompanyName), deref(viennaHost.LogoUrl))
	}

	server := New(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)), WithBlobstore(blob))
	logo := httptest.NewRecorder()
	server.ServeHTTP(logo, httptest.NewRequest(http.MethodGet, *viennaHost.LogoUrl, nil))
	if logo.Code != http.StatusOK || logo.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("anonymous entity logo: %d %s", logo.Code, logo.Body.String())
	}

	if _, err := e.Contacts.ClearLegalEntityLogo(admin, vienna.ID, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	bare := profileOf(e.As(e.Rep1, nil, integration.AdminPerms))
	if bare.CompanyName == nil || *bare.CompanyName != "Wien" || bare.LogoUrl != nil {
		t.Fatalf("an entity with no logo: %q %q, want its name and no logo", deref(bare.CompanyName), deref(bare.LogoUrl))
	}
	gone := httptest.NewRecorder()
	server.ServeHTTP(gone, httptest.NewRequest(http.MethodGet, *viennaHost.LogoUrl, nil))
	if gone.Code != http.StatusNotFound {
		t.Fatalf("a removed entity logo is still public: %d", gone.Code)
	}
}
