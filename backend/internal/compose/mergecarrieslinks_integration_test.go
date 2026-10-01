// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A merge carries the links and records other modules keep about the retired
// contact, through the real contacts→consent and contacts→introductions edges.
//
// Here for the reason mergecarriesstops_integration_test.go gives: contacts owns
// the merge, the carried tables belong to modules it cannot import, and the
// seam between them is wired in this package.
//
// The defect: a merge moved the grants and left the links behind. An
// unsubscribe press then withdrew the retired record while the survivor kept
// receiving mail, a confirm link stopped resolving, and the survivor's page
// lost the introductions colleagues had asked for.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// carryEnv is one installation with the merge wired exactly as compose wires
// it: stops, consent links and introduction asks.
type carryEnv struct {
	e        *integration.Env
	admin    context.Context
	consent  *consent.Store
	intros   *introductions.Store
	contacts *contacts.Store
}

func setupCarry(t *testing.T) *carryEnv {
	t.Helper()
	e := integration.Setup(t)
	consentStore := consent.NewStore(e.DB())
	intros := introductions.NewStore(e.DB(), time.Now)
	return &carryEnv{
		e: e, admin: e.Admin(), consent: consentStore, intros: intros,
		contacts: contacts.NewStore(e.DB()).WithStopCarrier(consentStore).
			WithSatelliteCarriers(consentStore, intros),
	}
}

// contactAt creates a contact holding one live primary address.
func (c *carryEnv) contactAt(t *testing.T, name, address string) ids.ContactID {
	t.Helper()
	created, err := c.e.Contacts.CreateContact(c.admin, contacts.CreateContactInput{
		FullName: name, Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true, Position: 1}},
	})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.From[ids.ContactKind](ids.UUID(created.Id))
}

// emailRow is the id of the row a contact holds an address on, live or not.
func (c *carryEnv) emailRow(t *testing.T, contact ids.ContactID, address string) ids.UUID {
	t.Helper()
	var row ids.UUID
	if err := c.e.Pool.QueryRow(context.Background(), `
		SELECT id FROM contact_email WHERE contact_id = $1 AND lower(email) = lower($2)
		 ORDER BY archived_at NULLS FIRST LIMIT 1`, contact, address).Scan(&row); err != nil {
		t.Fatalf("reading %s's row for %s: %v", contact, address, err)
	}
	return row
}

func (c *carryEnv) merge(t *testing.T, from, into ids.ContactID) {
	t.Helper()
	if _, err := c.contacts.MergeContact(c.admin, from, into); err != nil {
		t.Fatalf("merging: %v", err)
	}
}

// withdrawalLink mints an all-marketing unsubscribe link through the real
// writer, the one a send takes.
func (c *carryEnv) withdrawalLink(t *testing.T, in consent.WithdrawalMintInput) string {
	t.Helper()
	in.Scope = consent.WithdrawalScopeAllMarketing
	var token string
	if err := c.e.DB().Tx(c.admin, func(tx pgx.Tx) error {
		var err error
		token, err = c.consent.EnsureWithdrawalCredentialTx(c.admin, tx, in)
		return err
	}); err != nil {
		t.Fatalf("minting the withdrawal link: %v", err)
	}
	return token
}

// preferenceLink mints the preference-centre link a send to address carries.
func (c *carryEnv) preferenceLink(t *testing.T, address string) string {
	t.Helper()
	token, found, err := c.consent.PreferenceTokenForEmail(c.admin, address)
	if err != nil || !found {
		t.Fatalf("minting the preference link for %s: found=%v err=%v", address, found, err)
	}
	return token
}

// confirmLaneStore is a consent store that can mail a confirm link, wired the
// way setupConfirmLane wires it.
func confirmLaneStore(t *testing.T, e *integration.Env) *consent.Store {
	t.Helper()
	inserter, err := jobs.NewInserter(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("jobs.NewInserter: %v", err)
	}
	return consent.NewStore(InstallationDB(e.Pool)).
		WithConfirmationLane(NewControllerMailQueue(e.Pool, inserter), &sealingVault{}, "https://crm.example.test")
}

// purpose creates a consent purpose through the real writer, or reads the one
// already seeded under that key.
func (c *carryEnv) purpose(t *testing.T, key string, doubleOptIn bool) ids.PurposeID {
	t.Helper()
	var id ids.PurposeID
	err := c.e.Pool.QueryRow(context.Background(), `SELECT id FROM consent_purpose WHERE key = $1`, key).Scan(&id)
	if err == nil {
		return id
	}
	created, err := c.consent.CreatePurpose(c.admin, key, key, doubleOptIn)
	if err != nil {
		t.Fatalf("creating purpose %s: %v", key, err)
	}
	return created.ID
}

// grantNewsletter gives a contact a live grant on a single-opt-in newsletter
// and proves the gate lets it send, so a later refusal means something.
func (c *carryEnv) grantNewsletter(t *testing.T, contact ids.ContactID) string {
	t.Helper()
	const key = "carry_newsletter"
	source, wording := "manual", "Our newsletter, once a month."
	if _, err := c.consent.Record(c.admin, consent.RecordInput{
		ContactID: contact, PurposeID: c.purpose(t, key, false), NewState: "granted",
		Source: &source, PolicyText: &wording,
	}); err != nil {
		t.Fatalf("granting the newsletter: %v", err)
	}
	if err := c.sendAllowed(key, c.primaryAddress(t, contact)); err != nil {
		t.Fatalf("the newsletter is refused before anything was pressed (%v), so a later refusal proves nothing", err)
	}
	return key
}

func (c *carryEnv) primaryAddress(t *testing.T, contact ids.ContactID) string {
	t.Helper()
	var address string
	if err := c.e.Pool.QueryRow(context.Background(), `
		SELECT email FROM contact_email WHERE contact_id = $1 AND archived_at IS NULL AND is_primary`,
		contact).Scan(&address); err != nil {
		t.Fatalf("reading %s's address: %v", contact, err)
	}
	return address
}

// sendAllowed asks the gate every mail surface asks before a send.
func (c *carryEnv) sendAllowed(purposeKey, address string) error {
	return consent.NewGate(c.consent).RequireGrantedForEmails(c.admin, []string{address}, purposeKey)
}

// pressUnsubscribe presses a link the way a mailbox provider does, through the
// public handler and the principal the public middleware binds.
func (c *carryEnv) pressUnsubscribe(t *testing.T, token string) {
	t.Helper()
	if err := c.pressStatus(token); err != nil {
		t.Fatalf("the press was refused: %v", err)
	}
}

func (c *carryEnv) publicCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), c.e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:public_preferences"})
}

func (c *carryEnv) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := c.e.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return n
}

// An old unsubscribe link, pressed after the merge, stops mail to the survivor.
func TestAMergedContactsUnsubscribeLinkWithdrawsTheSurvivor(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Unsub Retired", "unsub@carry.test")
	survivor := c.contactAt(t, "Unsub Survivor", "unsub-survivor@carry.test")
	newsletter := c.grantNewsletter(t, survivor)
	token := c.withdrawalLink(t, consent.WithdrawalMintInput{Address: "unsub@carry.test", ContactID: retired})

	c.merge(t, retired, survivor)
	c.pressUnsubscribe(t, token)

	if err := c.sendAllowed(newsletter, "unsub-survivor@carry.test"); !errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Fatalf("a newsletter to the survivor answers %v after their old link was pressed, want it refused", err)
	}
}

// A preference link whose address row moved with the merge opens the
// survivor's centre at that same row.
func TestAMergedContactsPreferenceLinkOpensTheSurvivorsCentre(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Pref Retired", "pref@carry.test")
	survivor := c.contactAt(t, "Pref Survivor", "pref-survivor@carry.test")
	row := c.emailRow(t, retired, "pref@carry.test")
	token := c.preferenceLink(t, "pref@carry.test")

	c.merge(t, retired, survivor)

	ref, err := c.consent.ResolvePreferenceToken(context.Background(), token)
	if err != nil {
		t.Fatalf("the preference link stopped resolving after the merge: %v", err)
	}
	if ref.ContactID != survivor || ref.EmailID == nil || *ref.EmailID != row {
		t.Fatalf("the link opens (%s, %v), want the survivor %s at the moved row %s",
			ref.ContactID, ref.EmailID, survivor, row)
	}
	if _, err := c.consent.PublicPreferenceView(c.admin, ref); err != nil {
		t.Errorf("the survivor's centre did not open through the carried link: %v", err)
	}
}

// A link naming an address the merge left behind archived cannot take the
// survivor's slot for that address, which the survivor's own link holds. It is
// marked merged_into_survivor and opens the same page the survivor's own link
// opens, and its withdrawal half still acts on the survivor.
func TestALinkToAnAddressLeftBehindOpensTheSurvivorsPageForThatAddress(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Archived Retired", "moved@carry.test")
	retiredToken := c.preferenceLink(t, "moved@carry.test")
	if _, err := c.e.Contacts.UpdateContact(c.admin, retired, contacts.UpdateContactInput{
		Emails: []contacts.ContactEmailInput{}, Source: "manual",
	}); err != nil {
		t.Fatalf("archiving the retired contact's address: %v", err)
	}
	survivor := c.contactAt(t, "Archived Survivor", "moved@carry.test")
	survivorToken := c.preferenceLink(t, "moved@carry.test")

	c.merge(t, retired, survivor)

	var reason *string
	if err := c.e.Pool.QueryRow(context.Background(),
		`SELECT revoked_reason FROM preference_token WHERE token = $1`, retiredToken).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason == nil || *reason != consent.PreferenceRevokedMergedIntoSurvivor {
		t.Fatalf("the left-behind link reads revoked_reason=%v, want %s", reason, consent.PreferenceRevokedMergedIntoSurvivor)
	}
	carried, err := c.consent.ResolvePreferenceToken(context.Background(), retiredToken)
	if err != nil {
		t.Fatalf("the left-behind link no longer opens a preference centre: %v", err)
	}
	own, err := c.consent.ResolvePreferenceToken(context.Background(), survivorToken)
	if err != nil {
		t.Fatalf("the survivor's own link stopped resolving: %v", err)
	}
	if carried.ContactID != own.ContactID || carried.EmailID == nil || own.EmailID == nil || *carried.EmailID != *own.EmailID {
		t.Fatalf("the left-behind link opens (%s, %v), want the survivor's own page (%s, %v)",
			carried.ContactID, carried.EmailID, own.ContactID, own.EmailID)
	}
	withdrawal, err := c.consent.ResolveWithdrawalToken(context.Background(), retiredToken)
	if err != nil {
		t.Fatalf("the left-behind link's unsubscribe half was refused: %v", err)
	}
	if withdrawal.ContactID != survivor || withdrawal.Address != "moved@carry.test" {
		t.Errorf("the unsubscribe half withdraws (%s, %q), want the survivor at moved@carry.test",
			withdrawal.ContactID, withdrawal.Address)
	}
}

// A confirm link answers only for a live subject, so one left on the retired
// record was dead. Carried, the answer lands on the survivor.
func TestAMergedContactsConfirmLinkAnswersOnTheSurvivor(t *testing.T) {
	c := setupCarry(t)
	integration.ApplyRiverSchema(t)
	lane := confirmLaneStore(t, c.e)
	marketing := c.purpose(t, consent.PurposeMarketingEmail, true)
	retired := c.contactAt(t, "Confirm Retired", "confirm@carry.test")
	survivor := c.contactAt(t, "Confirm Survivor", "confirm-survivor@carry.test")
	issued, err := lane.IssueConfirmToken(c.admin, retired)
	if err != nil {
		t.Fatalf("issuing the confirm link: %v", err)
	}

	c.merge(t, retired, survivor)

	if _, err := lane.SubmitConfirmation(c.admin, issued.Token, consent.ConfirmSubmission{
		MarketingChoice: "withdrawn", MarketingWording: "No news, thank you.",
	}); err != nil {
		t.Fatalf("answering through the carried link: %v", err)
	}
	if n := c.count(t, `SELECT count(*) FROM contact_consent WHERE contact_id = $1 AND purpose_id = $2 AND state = 'withdrawn'`,
		survivor, marketing); n != 1 {
		t.Fatalf("the survivor holds %d withdrawn marketing row(s) after answering through the carried link, want 1", n)
	}
}

// Both halves' qualifying events are the survivor's now. The one event both
// halves recorded for the same booking stays once. The lawful-basis carry is
// held through a real send in integration/mergecarriesbasis_integration_test.go.
func TestAMergeCarriesQualifyingEventsWithoutDoublingOne(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Basis Retired", "basis@carry.test")
	survivor := c.contactAt(t, "Basis Survivor", "basis-survivor@carry.test")
	if _, err := c.consent.RecordQualifyingEvent(c.admin, retired, consent.RecordQualifyingEventInput{
		Kind: "in_person", Note: "met at the trade fair", OccurredAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("recording the hand-written event: %v", err)
	}
	booking, ownBooking := ids.NewV7(), ids.NewV7()
	for _, who := range []ids.ContactID{retired, survivor} {
		if err := c.consent.RecordInquiry(c.admin, who, booking); err != nil {
			t.Fatalf("recording the inquiry: %v", err)
		}
	}
	if err := c.consent.RecordInquiry(c.admin, retired, ownBooking); err != nil {
		t.Fatalf("recording the retired contact's own inquiry: %v", err)
	}

	c.merge(t, retired, survivor)

	if n := c.count(t, `SELECT count(*) FROM consent_qualifying_event WHERE contact_id = $1 AND kind = 'in_person'`, survivor); n != 1 {
		t.Errorf("the survivor holds %d hand-recorded event(s), want the retired record's 1", n)
	}
	if n := c.count(t, `SELECT count(*) FROM consent_qualifying_event WHERE contact_id = $1 AND source_entity_id = $2`, survivor, booking); n != 1 {
		t.Errorf("the survivor holds %d event(s) for the one booking, want exactly 1", n)
	}
	if n := c.count(t, `SELECT count(*) FROM consent_qualifying_event WHERE contact_id = $1 AND source_entity_id = $2`, survivor, ownBooking); n != 1 {
		t.Errorf("the survivor holds %d event(s) for the retired contact's own booking, want 1", n)
	}
	if n := c.count(t, `SELECT count(*) FROM consent_qualifying_event WHERE contact_id = $1`, retired); n != 0 {
		t.Errorf("%d qualifying event(s) stayed on the retired record", n)
	}
}

// Double-opt-in history follows the contact, so an erasure of the survivor
// finds it. No writer mints these any more; this is the shape the retired one
// wrote.
func TestAMergeCarriesDoubleOptInHistory(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "DOI Retired", "doi@carry.test")
	survivor := c.contactAt(t, "DOI Survivor", "doi-survivor@carry.test")
	purpose, err := c.consent.CreatePurpose(c.admin, "doi_carry_newsletter", "Newsletter", true)
	if err != nil {
		t.Fatalf("creating the purpose: %v", err)
	}
	if _, err := c.e.Pool.Exec(context.Background(), `
		INSERT INTO consent_doi_token (contact_id, purpose_id, token_hash, expires_at)
		VALUES ($1, $2, 'doi-carry-hash', now())`, retired, purpose.ID); err != nil {
		t.Fatal(err)
	}

	c.merge(t, retired, survivor)

	if n := c.count(t, `SELECT count(*) FROM consent_doi_token WHERE contact_id = $1`, survivor); n != 1 {
		t.Errorf("the survivor holds %d double-opt-in row(s), want the retired record's 1", n)
	}
}

// The carry is audited on the survivor, naming what moved and where from.
func TestTheCarryIsAuditedOnTheSurvivor(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Audit Retired", "audit@carry.test")
	survivor := c.contactAt(t, "Audit Survivor", "audit-survivor@carry.test")
	c.withdrawalLink(t, consent.WithdrawalMintInput{Address: "audit@carry.test", ContactID: retired})

	c.merge(t, retired, survivor)

	var carried int
	var from string
	if err := c.e.Pool.QueryRow(context.Background(), `
		SELECT (after->>'consent_records_carried')::int, evidence->>'carried_from'
		  FROM audit_log
		 WHERE entity_type = 'contact' AND entity_id = $1
		   AND after->>'consent_records_carried' IS NOT NULL`,
		survivor).Scan(&carried, &from); err != nil {
		t.Fatalf("no audit row on the survivor names the carry: %v", err)
	}
	if carried != 1 || from != retired.String() {
		t.Errorf("the audit row says %d carried from %s, want 1 from %s", carried, from, retired)
	}
}

// AN UNWIRED SEAM REFUSES A MERGE THAT WOULD STRAND A LINK, and only that one.
func TestAnUnwiredMergeRefusesOnlyWhenALinkWouldBeStranded(t *testing.T) {
	c := setupCarry(t)
	unwired := contacts.NewStore(c.e.DB()).WithStopCarrier(c.consent)

	plain := c.contactAt(t, "Unwired Plain", "plain@carry.test")
	plainInto := c.contactAt(t, "Unwired Plain Survivor", "plain-survivor@carry.test")
	if _, err := unwired.MergeContact(c.admin, plain, plainInto); err != nil {
		t.Fatalf("an ordinary merge was refused for want of a seam it did not need: %v", err)
	}

	holder := c.contactAt(t, "Unwired Holder", "holder@carry.test")
	holderInto := c.contactAt(t, "Unwired Holder Survivor", "holder-survivor@carry.test")
	c.withdrawalLink(t, consent.WithdrawalMintInput{Address: "holder@carry.test", ContactID: holder})

	_, err := unwired.MergeContact(c.admin, holder, holderInto)
	var notWired *contacts.SatelliteCarrierNotWiredError
	if !errors.As(err, &notWired) {
		t.Fatalf("the merge answered %v, want SatelliteCarrierNotWiredError", err)
	}
	if n := c.count(t, `SELECT count(*) FROM withdrawal_credential WHERE contact_id = $1`, holder); n != 1 {
		t.Errorf("the refused merge left %d link(s) on the source, want its own intact", n)
	}
	if n := c.count(t, `SELECT count(*) FROM contact WHERE id = $1 AND archived_at IS NULL`, holder); n != 1 {
		t.Error("the refused merge retired the source anyway")
	}
}

// Upgraded installations hold preference links that never recorded their
// address. Both halves holding one is a collision on the NULL slot the unique
// index counts; the retired half's link is marked and still opens the
// survivor's own page.
func TestALegacyAddresslessLinkCollidesOnTheNullSlot(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Legacy Retired", "legacy@carry.test")
	survivor := c.contactAt(t, "Legacy Survivor", "legacy-survivor@carry.test")
	// The shape a link minted before links recorded their address has. No
	// writer mints one any more.
	retiredToken, survivorToken := "pref_legacy_retired_"+retired.String(), "pref_legacy_survivor_"+survivor.String()
	for who, token := range map[ids.ContactID]string{retired: retiredToken, survivor: survivorToken} {
		if _, err := c.e.Pool.Exec(context.Background(), `
			INSERT INTO preference_token (contact_id, token, expires_at) VALUES ($1, $2, now() + interval '30 days')`,
			who, token); err != nil {
			t.Fatal(err)
		}
	}

	c.merge(t, retired, survivor)

	carried, err := c.consent.ResolvePreferenceToken(context.Background(), retiredToken)
	if err != nil {
		t.Fatalf("the retired half's legacy link stopped resolving: %v", err)
	}
	if carried.ContactID != survivor || carried.EmailID != nil {
		t.Errorf("the legacy link opens (%s, %v), want the survivor with no recorded address", carried.ContactID, carried.EmailID)
	}
	if n := c.count(t, `SELECT count(*) FROM preference_token WHERE contact_id = $1 AND revoked_at IS NULL`, survivor); n != 1 {
		t.Errorf("the survivor holds %d live link(s), want only their own", n)
	}
	if _, err := c.consent.ResolvePreferenceToken(context.Background(), survivorToken); err != nil {
		t.Errorf("the survivor's own legacy link stopped resolving: %v", err)
	}
}

// What a merge carried onto the survivor is the survivor's, so erasing them
// erases it too.
func TestErasingTheSurvivorErasesWhatTheMergeCarried(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Erase Retired", "erase@carry.test")
	survivor := c.contactAt(t, "Erase Survivor", "erase-survivor@carry.test")
	c.withdrawalLink(t, consent.WithdrawalMintInput{Address: "erase@carry.test", ContactID: retired})
	c.preferenceLink(t, "erase@carry.test")
	c.merge(t, retired, survivor)

	if err := privacy.NewEraser(c.e.DB()).EraseContact(c.admin, survivor.UUID, "subject request"); err != nil {
		t.Fatalf("erasing the survivor: %v", err)
	}

	for _, table := range []string{"withdrawal_credential", "preference_token"} {
		// table is one of two literals above.
		if n := c.count(t, `SELECT count(*) FROM `+table+` WHERE contact_id IN ($1, $2)`, retired, survivor); n != 0 {
			t.Errorf("%d %s row(s) outlived the survivor's erasure", n, table)
		}
	}
}
