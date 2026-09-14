// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Rendering the disclosures a jurisdiction pack declares.
//
// Every pack has declared them since it shipped and nothing read one, which
// gates/messagingruleapplied_test.go's register recorded in its own words. The
// obligation could not be met because nothing knew who the controller was —
// there was no name, address or privacy contact anywhere in the product.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// inJurisdiction binds the store to a pack declaring the given disclosures.
func inJurisdiction(
	t *testing.T, e *channelConsentEnv, code string, ds ...messagingrules.Disclosure,
) *Store {
	t.Helper()
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: jurisdiction.Code(code), Version: 1, Disclosures: ds,
	})
	return e.store.WithInstallationCountry(
		InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) {
			return jurisdiction.Code(code), nil
		}))
}

// operatorCtx is the env's context with the installation-settings grant.
//
// A DIFFERENT seat from officerCtx: the privacy queue and the installation's
// own identity are different objects, and somebody trusted to work subject
// requests is not thereby trusted to change what every outgoing message says
// about the company.
func operatorCtx(e *channelConsentEnv) context.Context {
	actor, _ := principal.Actor(e.ctx)
	actor.Permissions.Objects = map[string]principal.ObjectGrant{
		"installation_settings": {Read: true, Update: true},
	}
	return principal.WithActor(e.ctx, actor)
}

// settingsStoreFor builds the store that OWNS the setting table, over the same
// pool this suite runs on. Consent may not write that table, so the operator
// path goes through here in production too.
func settingsStoreFor(t *testing.T) *settings.Store {
	t.Helper()
	pool, err := testdb.Pool(context.Background(), os.Getenv("MARGINCE_TEST_APP_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	// Registered where the pool is handed out, before the test adds any
	// cleanup of its own, so it runs last and sees a package that has
	// genuinely stopped — a goroutine still holding a connection would go on
	// writing into the database the NEXT test just reset.
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	return settings.New(pool, settings.NewRegistry(Definitions()...))
}

// stateParticulars saves what the installation says about itself.
//
// Written as the row rather than through the settings store, because the store
// takes a gate this fixture has no seat for — and the READ is what these tests
// are about. The key is the entry's own, so a rename breaks this loudly.
func stateParticulars(t *testing.T, e *channelConsentEnv, p ControllerParticulars) {
	t.Helper()
	value, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO setting (key, value, updated_at) VALUES ($1, $2::jsonb, now())
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = now()`,
		ControllerIdentity.Key(), string(value)); err != nil {
		t.Fatalf("stating the controller particulars: %v", err)
	}
}

// disclosures asks what a message of this category must carry.
func disclosures(
	t *testing.T, e *channelConsentEnv, store *Store, category commsauthz.Category,
) []DisclosureLine {
	t.Helper()
	var out []DisclosureLine
	if err := store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		out, err = store.DisclosuresFor(e.ctx, tx, category)
		return err
	}); err != nil {
		t.Fatalf("resolving the disclosures: %v", err)
	}
	return out
}

// TestADeclaredDisclosureIsRenderedFromTheStatedParticulars is the register
// line this closes: the kinds were a closed set nothing turned into text.
func TestADeclaredDisclosureIsRenderedFromTheStatedParticulars(t *testing.T) {
	e := setupChannelConsent(t)
	store := inJurisdiction(t, e, "zd",
		messagingrules.Disclosure{Kind: messagingrules.ControllerIdentity},
		messagingrules.Disclosure{Kind: messagingrules.PrivacyContact})
	stateParticulars(t, e, ControllerParticulars{
		LegalName:      "Beispiel GmbH",
		PostalAddress:  "Hauptstraße 1\n10115 Berlin",
		PrivacyContact: "datenschutz@beispiel.test",
	})

	lines := disclosures(t, e, store, commsauthz.CategoryReplyToInbound)
	if len(lines) != 2 {
		t.Fatalf("the pack declares 2 disclosures and %d were rendered", len(lines))
	}
	for _, line := range lines {
		if line.Missing() {
			t.Errorf("%q rendered nothing although the installation stated its particulars",
				line.Kind)
		}
	}
	if lines[0].Text != "Beispiel GmbH\nHauptstraße 1\n10115 Berlin" {
		t.Errorf("the identity reads %q, want the name and address together", lines[0].Text)
	}
}

// TestAnUnconfiguredInstallationReportsTheDisclosureMissing.
//
// A DIFFERENT ANSWER from "nothing is owed", and the difference is the point.
// Rendering nothing for both would let an installation that never configured
// itself look identical to one in a jurisdiction that demands nothing — the
// first is a compliance gap somebody has to close.
func TestAnUnconfiguredInstallationReportsTheDisclosureMissing(t *testing.T) {
	e := setupChannelConsent(t)
	store := inJurisdiction(t, e, "ze",
		messagingrules.Disclosure{Kind: messagingrules.ControllerIdentity})

	lines := disclosures(t, e, store, commsauthz.CategoryReplyToInbound)
	if len(lines) != 1 {
		t.Fatalf("the pack declares 1 disclosure and %d were reported", len(lines))
	}
	if !lines[0].Missing() {
		t.Errorf("an installation that never said who it is rendered %q: the obligation "+
			"applies and cannot be met, which is what Missing reports", lines[0].Text)
	}
}

// TestNoPackDeclaresNoDisclosure. An installation whose country is unknown owes
// nothing this function can name, and says so by answering nothing at all —
// which a caller tells from the case above by the empty slice.
func TestNoPackDeclaresNoDisclosure(t *testing.T) {
	e := setupChannelConsent(t)
	stateParticulars(t, e, ControllerParticulars{LegalName: "Beispiel GmbH"})

	if lines := disclosures(t, e, e.store, commsauthz.CategoryMarketing); len(lines) != 0 {
		t.Errorf("an installation with no pack reported %d disclosures, want none", len(lines))
	}
}

// TestAMarketingOnlyDisclosureStaysOffAnInvoice.
//
// §7(3) UWG binds the objection route to ADVERTISING, at every use rather than
// only the first contact. Rendering it on every category would put an
// unsubscribe line under a payment reminder, which is both wrong and the kind
// of thing a recipient reads as an invitation to stop receiving invoices.
func TestAMarketingOnlyDisclosureStaysOffAnInvoice(t *testing.T) {
	e := setupChannelConsent(t)
	store := inJurisdiction(t, e, "zf",
		messagingrules.Disclosure{Kind: messagingrules.ControllerIdentity},
		messagingrules.Disclosure{Kind: messagingrules.ObjectionRoute, MarketingOnly: true})
	stateParticulars(t, e, ControllerParticulars{
		LegalName:      "Beispiel GmbH",
		ObjectionRoute: "Reply with STOP, or use the link in any of our mails.",
	})

	onInvoice := disclosures(t, e, store, commsauthz.CategoryInvoiceOrPayment)
	if len(onInvoice) != 1 || onInvoice[0].Kind != string(messagingrules.ControllerIdentity) {
		t.Errorf("an invoice carries %+v, want the identity alone: the objection route is "+
			"declared marketing-only", onInvoice)
	}
	onMarketing := disclosures(t, e, store, commsauthz.CategoryMarketing)
	if len(onMarketing) != 2 {
		t.Errorf("an advertising message carries %d disclosures, want both", len(onMarketing))
	}
}

// TestTheAdvertiserContactIsItsOwnParticular.
//
// Decree 91/2020 asks how to REACH the advertiser, which is a different fact
// from where they are registered. Folding it onto the postal address would
// answer a phone-number obligation with a street.
func TestTheAdvertiserContactIsItsOwnParticular(t *testing.T) {
	e := setupChannelConsent(t)
	store := inJurisdiction(t, e, "zg",
		messagingrules.Disclosure{Kind: messagingrules.AdvertiserContact, MarketingOnly: true})
	stateParticulars(t, e, ControllerParticulars{
		LegalName:     "Beispiel GmbH",
		PostalAddress: "Hauptstraße 1\n10115 Berlin",
	})

	lines := disclosures(t, e, store, commsauthz.CategoryMarketing)
	if len(lines) != 1 {
		t.Fatalf("the pack declares 1 disclosure and %d were reported", len(lines))
	}
	if !lines[0].Missing() {
		t.Errorf("the advertiser contact rendered %q from a name and a postal address: the "+
			"obligation is about reachability, and answering it with a street address "+
			"reports an unmet duty as met", lines[0].Text)
	}
}

// TestAnIdentityNeedsBothItsHalves.
//
// messaging.ControllerIdentity is "the legal name and postal address of the
// controller", so a name with no address is not an identity disclosure a
// supervisory authority would accept. Rendering it anyway would report the
// obligation met and leave the operator with no signal that their setup is
// incomplete — and the missing answer is the one that gets fixed.
//
// The first version of this test asserted the opposite, on the reasoning that
// somebody who stated a name had identified themselves. The kind's own
// definition says otherwise.
func TestAnIdentityNeedsBothItsHalves(t *testing.T) {
	e := setupChannelConsent(t)
	store := inJurisdiction(t, e, "zh",
		messagingrules.Disclosure{Kind: messagingrules.ControllerIdentity})
	stateParticulars(t, e, ControllerParticulars{LegalName: "Beispiel GmbH"})

	lines := disclosures(t, e, store, commsauthz.CategoryReplyToInbound)
	if len(lines) != 1 {
		t.Fatalf("the pack declares 1 disclosure and %d were reported", len(lines))
	}
	if !lines[0].Missing() {
		t.Errorf("a name with no postal address rendered %q as a complete identity: the "+
			"obligation names two things and is not met by one", lines[0].Text)
	}
}

// TestTheParticularsRoundTrip. What a read returns is what a write of the same
// body would store — otherwise a form shows somebody a value that is not saved.
func TestTheParticularsRoundTrip(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := operatorCtx(e)
	in := ControllerParticulars{
		LegalName:         "Beispiel GmbH",
		PostalAddress:     "Hauptstraße 1\n10115 Berlin",
		PrivacyContact:    "datenschutz@beispiel.test",
		ObjectionRoute:    "Reply with STOP.",
		AdvertiserContact: "+49 30 123456",
	}

	store := settingsStoreFor(t)
	if err := settings.Set(ctx, store, ControllerIdentity, in); err != nil {
		t.Fatalf("stating the particulars: %v", err)
	}
	read, err := settings.Get(ctx, store, ControllerIdentity)
	if err != nil {
		t.Fatalf("reading them back: %v", err)
	}
	if read != in {
		t.Errorf("the read answered %+v, want what was stored", read)
	}
}

// TestStatingTheParticularsNeedsTheSettingsGrant. These words go out at the
// bottom of every message this installation sends, so changing them changes
// what every recipient is told.
func TestStatingTheParticularsNeedsTheSettingsGrant(t *testing.T) {
	e := setupChannelConsent(t)

	store := settingsStoreFor(t)
	if err := settings.Set(e.ctx, store, ControllerIdentity, ControllerParticulars{
		LegalName: "Somebody Else Ltd",
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep restated who this installation is: %v", err)
	}
	if _, err := settings.Get(e.ctx, store, ControllerIdentity); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep was shown the particulars form: %v", err)
	}
}

// TestAnOverlongParticularIsRefused. Every field goes into every message, so an
// unbounded one is a way to make all of them arbitrarily large.
func TestAnOverlongParticularIsRefused(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := operatorCtx(e)

	err := settings.Set(ctx, settingsStoreFor(t), ControllerIdentity,
		ControllerParticulars{LegalName: strings.Repeat("x", particularsFieldCap+1)})
	if err == nil {
		t.Fatal("an over-long legal name was accepted: every field goes into every message " +
			"this installation sends")
	}
}

// TestAWhitespaceParticularIsRefused. A field of only spaces reads as filled in
// and discloses nothing, which is worse than leaving it empty: the empty one is
// reported missing and gets fixed.
func TestAWhitespaceParticularIsRefused(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := operatorCtx(e)

	err := settings.Set(ctx, settingsStoreFor(t), ControllerIdentity,
		ControllerParticulars{LegalName: "   "})
	if err == nil {
		t.Fatal("a whitespace legal name was accepted: it reads as filled in and discloses " +
			"nothing, which is worse than leaving it empty")
	}
}

// TestAWriterWithoutTheReadGrantChangesNothing is the ordering Codex found.
//
// The PUT echoes the stored value, which asks the read verb. Checking that
// AFTER the write meant a principal holding update-without-read changed what
// every outgoing message says about the company, then received a 403 for the
// same request — with the mutation already committed and nothing telling them
// it had happened.
func TestAWriterWithoutTheReadGrantChangesNothing(t *testing.T) {
	e := setupChannelConsent(t)
	store := settingsStoreFor(t)
	stated := ControllerParticulars{
		LegalName:     "Beispiel GmbH",
		PostalAddress: "Hauptstraße 1\n10115 Berlin",
	}
	if err := settings.Set(operatorCtx(e), store, ControllerIdentity, stated); err != nil {
		t.Fatalf("stating the particulars: %v", err)
	}

	actor, _ := principal.Actor(e.ctx)
	actor.Permissions.Objects = map[string]principal.ObjectGrant{
		"installation_settings": {Update: true},
	}
	writeOnly := principal.WithActor(e.ctx, actor)

	h := Handlers{store: e.store}.WithSettings(store)
	rec := httptest.NewRecorder()
	body := `{"legal_name":"Somebody Else Ltd"}`
	req := httptest.NewRequest(http.MethodPut, "/v1/privacy/controller-particulars",
		strings.NewReader(body)).WithContext(writeOnly)
	req.Header.Set("Content-Type", "application/json")
	h.SetControllerParticulars(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("a writer with no read grant got 200; the response echoes the stored value, "+
			"so this request cannot succeed (body: %s)", rec.Body.String())
	}
	after, err := settings.Get(operatorCtx(e), store, ControllerIdentity)
	if err != nil {
		t.Fatalf("reading the particulars back: %v", err)
	}
	if after != stated {
		t.Errorf("the refused request still changed the particulars to %+v: the read gate "+
			"has to run BEFORE the write, or a 403 arrives after the mutation committed",
			after)
	}
}
