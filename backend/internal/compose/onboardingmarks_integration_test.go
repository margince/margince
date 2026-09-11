// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The anchor company's SECOND face. It is the one company the chrome draws at
// two widths, so its cold-start read parks a lockup and a badge and the
// confirmation binds each to its own slot — under the same human-precedence
// rule, slot by slot. The fixtures are onboardinglogo_integration_test.go's.

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheColdStartParksBothMarksAndTheConfirmationBindsEachToItsSlot(t *testing.T) {
	// The installation's own company is drawn at two widths. A seed page that
	// labels its wordmark and declares its icon gives the dossier one mark per
	// slot, and the confirmation moves each onto the record it creates — the
	// lockup where every record wears its mark, the badge where only the anchor
	// wears one — so the shell draws the company's own lockup expanded and its
	// own badge collapsed from the first moment the company exists.
	e := integration.Setup(t)
	site := &assetSite{assets: map[string][]byte{
		touchIconURL: logoFixture(t, 512, 512),
		wordmarkURL:  logoFixture(t, 600, 150),
	}}
	blob := blobstore.NewMemory()
	args := readTheOnboardingSiteDeclaring(t, e, onboardingLogoWorker(e, site, blob), declaringBothMarks())

	wideKey, wideOrigin := parkedMark(t, e, args.SiteReadID, people.LogoWide)
	if wideKey == nil || wideOrigin == nil || *wideOrigin != wordmarkURL {
		t.Fatalf("the wide slot parked key %v origin %v, want the wordmark at %q", wideKey, wideOrigin, wordmarkURL)
	}
	iconKey, iconOrigin := parkedMark(t, e, args.SiteReadID, people.LogoIcon)
	if iconKey == nil || iconOrigin == nil || *iconOrigin != touchIconURL {
		t.Fatalf("the icon slot parked key %v origin %v, want the icon at %q", iconKey, iconOrigin, touchIconURL)
	}
	if *wideKey == *iconKey {
		t.Fatalf("both slots name one object %q; each mark must have its own", *wideKey)
	}

	company := confirmTheAnchor(t, e, args)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	for slot, want := range map[people.LogoSlot]string{people.LogoWide: *wideKey, people.LogoIcon: *iconKey} {
		bound, err := e.People.CompanyLogoKey(ctx, company.CompanyID, slot)
		if err != nil {
			t.Fatalf("the confirmed anchor has no %s: %v", slot, err)
		}
		if bound != want {
			t.Fatalf("the anchor's %s names %q, want the parked object at %q", slot, bound, want)
		}
	}
	// Handed over, not shared: the dossier's references are gone with the marks.
	for _, slot := range []people.LogoSlot{people.LogoWide, people.LogoIcon} {
		if left, _ := parkedMark(t, e, args.SiteReadID, slot); left != nil {
			t.Fatalf("the confirmed dossier still names %q in its %s slot", *left, slot)
		}
	}
	// The profile the shell reads carries both URLs, each on its own path.
	profile, err := e.People.GetAnchorCompany(ctx)
	if err != nil {
		t.Fatalf("read the company profile: %v", err)
	}
	wire := toContractCompany(profile)
	wantIcon := *people.LogoURL(company.CompanyID.UUID, iconKey, people.LogoIcon)
	if wire.LogoIconUrl == nil || *wire.LogoIconUrl != wantIcon {
		t.Fatalf("the profile's logo_icon_url = %v, want %q — the badge the collapsed rail draws", wire.LogoIconUrl, wantIcon)
	}
	// The badge's provenance is filed under its own field name, by the read.
	var capturedBy string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT captured_by FROM field_provenance
			WHERE object_type = 'company' AND object_id = $1 AND field_name = 'logo_icon'
			ORDER BY captured_at DESC, id DESC LIMIT 1`, company.CompanyID).Scan(&capturedBy)
	}); err != nil {
		t.Fatalf("reading the badge's provenance: %v", err)
	}
	if capturedBy != "agent:site-read" {
		t.Fatalf("logo_icon captured_by = %q, want the site read", capturedBy)
	}
}

func TestAColdStartWithoutALockupLeavesTheBadgeSlotEmpty(t *testing.T) {
	// An icon-only site gets what every installation got before the lockup
	// existed: the icon in the wide slot, nothing in the badge slot, and the
	// collapsed rail falling back to the wide mark on its own.
	e := integration.Setup(t)
	site := &assetSite{assets: map[string][]byte{touchIconURL: logoFixture(t, 512, 512)}}
	args := readTheOnboardingSite(t, e, onboardingLogoWorker(e, site, blobstore.NewMemory()))
	if key, _ := parkedMark(t, e, args.SiteReadID, people.LogoIcon); key != nil {
		t.Fatalf("an icon-only site parked a badge at %q beside the wide mark it already serves", *key)
	}
	company := confirmTheAnchor(t, e, args)
	_, err := e.People.CompanyLogoKey(e.As(e.Rep1, nil, integration.AdminPerms), company.CompanyID, people.LogoIcon)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the anchor's badge slot answers %v, want not-found so the rail falls back to the wide mark", err)
	}
}

func TestTheConfirmationKeepsTheBadgeAPersonChoseAndStillLandsTheLockup(t *testing.T) {
	// The slots are decided one at a time. A person who uploaded a badge before
	// the read ran keeps it, and its bytes; the lockup the read resolved still
	// lands, because nobody chose a wide mark; and the badge the read parked,
	// which nothing adopts, is collected by the transport.
	e := integration.Setup(t)
	human := e.As(e.Rep1, nil, integration.AdminPerms)
	blob := newRecordingBlobstore()
	saved, err := e.People.SaveCompany(human, people.SaveCompanyInput{DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("describe the company by hand: %v", err)
	}
	uploaded := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](e.WS), companyLogoKind,
		saved.CompanyID.String()+"/uploaded-badge")
	chosen := logoFixture(t, 64, 64)
	if err := blob.Put(context.Background(), uploaded, bytes.NewReader(chosen),
		int64(len(chosen)), imagenorm.ContentType); err != nil {
		t.Fatalf("store the person's own badge: %v", err)
	}
	if _, err := e.People.SetAnchorCompanyLogo(human, people.LogoIcon, uploaded, "badge.png"); err != nil {
		t.Fatalf("record the person's own badge: %v", err)
	}

	site := &assetSite{assets: map[string][]byte{
		touchIconURL: logoFixture(t, 512, 512),
		wordmarkURL:  logoFixture(t, 600, 150),
	}}
	args := readTheOnboardingSiteDeclaring(t, e, onboardingLogoWorker(e, site, blob), declaringBothMarks())
	parkedWide, _ := parkedMark(t, e, args.SiteReadID, people.LogoWide)
	parkedIcon, _ := parkedMark(t, e, args.SiteReadID, people.LogoIcon)
	if parkedWide == nil || parkedIcon == nil {
		t.Fatal("the read parked less than both marks; this case has nothing to decide")
	}

	engine := &deepReadEngine{people: e.People, blob: blob, log: slog.New(slog.DiscardHandler)}
	companyID := confirmTheAnchorAsTheAPIDoes(t, e, engine, args)

	boundIcon, err := e.People.CompanyLogoKey(human, companyID, people.LogoIcon)
	if err != nil {
		t.Fatalf("the anchor lost its badge: %v", err)
	}
	if boundIcon != uploaded {
		t.Fatalf("the anchor's badge is %q, want the one the person chose at %q", boundIcon, uploaded)
	}
	boundWide, err := e.People.CompanyLogoKey(human, companyID, people.LogoWide)
	if err != nil {
		t.Fatalf("the anchor has no wide mark: %v", err)
	}
	if boundWide != *parkedWide {
		t.Fatalf("the anchor's wide mark is %q, want the lockup the read parked at %q", boundWide, *parkedWide)
	}
	if _, _, err := blob.Get(context.Background(), *parkedIcon); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("the unadopted badge at %q answers %v, want it collected", *parkedIcon, err)
	}
	if n := rowsNaming(t, e, *parkedIcon); n != 0 {
		t.Fatalf("%d row(s) still name the collected badge at %q", n, *parkedIcon)
	}
	stored, _, err := blob.Get(context.Background(), uploaded)
	if err != nil {
		t.Fatalf("the badge the person chose answers %v, want it kept", err)
	}
	if err := stored.Close(); err != nil {
		t.Fatalf("closing the stored object: %v", err)
	}
}
