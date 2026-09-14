// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Acknowledging a refusal of advertising, where the applicable rules owe one.
//
// gates/messagingruleapplied_test.go carried OptOutAcknowledgement in its
// register of declared-and-unapplied obligations: the Vietnamese pack has said
// since it shipped that Decree 91/2020 Art. 16 owes a recipient who refuses
// advertising a confirmation within twenty-four hours, and nothing sent one.
// That register is empty now, and this is the last entry to leave it.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// owingAcknowledgement binds the store to a pack that owes one, with a lane to
// stage it on.
func owingAcknowledgement(t *testing.T, e *channelConsentEnv, owes bool) *recordingStager {
	t.Helper()
	code := testJurisdiction(t)
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: code, Version: 1, OptOutAcknowledgement: owes,
	})
	stager := &recordingStager{}
	e.store = e.store.
		WithConfirmationLane(stager, &recordingVault{}, "https://crm.example.test/").
		WithInstallationCountry(
			InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) {
				return code, nil
			}))
	return stager
}

// TestAMarketingObjectionIsAcknowledged is the register line this closes.
func TestAMarketingObjectionIsAcknowledged(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager := owingAcknowledgement(t, e, true)

	if err := e.store.Suppress(e.ctx, SuppressInput{
		ContactID: e.contact, Kind: commsauthz.ReasonObjection, Reason: "No more ads.",
	}); err != nil {
		t.Fatalf("recording the objection: %v", err)
	}

	if stager.calls != 1 {
		t.Fatalf("the stop staged %d acknowledgements, want 1 — a Vietnamese recipient who "+
			"refuses advertising is owed one within twenty-four hours", stager.calls)
	}
	if stager.seen.Category != commsauthz.CategoryOptoutConfirmation {
		t.Errorf("the acknowledgement went out as %q, want optout_confirmation — that is the "+
			"one category a broad stop does not bind, which is what lets this reach somebody "+
			"who has just stopped everything", stager.seen.Category)
	}
}

// TestTheAcknowledgementCarriesNoLinkAndNoAdvertising.
//
// It goes to somebody who has just told the product to stop, so anything beyond
// "we heard you" is the thing they asked not to receive. A link asking them to
// do something more would read as a message that did not take the first answer.
func TestTheAcknowledgementCarriesNoLinkAndNoAdvertising(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager := owingAcknowledgement(t, e, true)

	if err := e.store.Suppress(e.ctx, SuppressInput{
		ContactID: e.contact, Kind: commsauthz.ReasonObjection,
	}); err != nil {
		t.Fatalf("recording the objection: %v", err)
	}

	body := stager.seen.Rendered.Body
	if strings.Contains(body, linkPlaceholder) {
		t.Errorf("the acknowledgement carries a link placeholder:\n%s", body)
	}
	if stager.seen.LinkRef != "" || !stager.seen.LinkID.IsZero() {
		t.Errorf("the acknowledgement was staged with link material (%q/%v) — comms refuses a "+
			"body whose placeholder count disagrees with what it carries",
			stager.seen.LinkRef, stager.seen.LinkID)
	}
	if body == "" || stager.seen.Rendered.Subject == "" {
		t.Error("the acknowledgement has no words")
	}
}

// TestAStopUnderNoSuchObligationAcknowledgesNothing. Every installation but a
// Vietnamese one today, and a message nobody is owed is a message somebody who
// asked us to stop receives anyway.
func TestAStopUnderNoSuchObligationAcknowledgesNothing(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager := owingAcknowledgement(t, e, false)

	if err := e.store.Suppress(e.ctx, SuppressInput{
		ContactID: e.contact, Kind: commsauthz.ReasonObjection,
	}); err != nil {
		t.Fatalf("recording the objection: %v", err)
	}
	if stager.calls != 0 {
		t.Errorf("a pack owing no acknowledgement sent %d — the subject asked us to stop and "+
			"received mail for it", stager.calls)
	}
}

// TestABroadStopIsAcknowledgedToo.
//
// Somebody asking us to stop contacting them entirely has refused advertising
// along with everything else. Reading the wider request as not including the
// narrower one would deny an acknowledgement to the subject who asked for most.
func TestABroadStopIsAcknowledgedToo(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager := owingAcknowledgement(t, e, true)

	if err := e.store.Suppress(e.ctx, SuppressInput{
		ContactID: e.contact, Kind: "subject_request", Reason: "Stop contacting me.",
	}); err != nil {
		t.Fatalf("recording the broad stop: %v", err)
	}
	if stager.calls != 1 {
		t.Errorf("a broad stop staged %d acknowledgements, want 1", stager.calls)
	}
}

// TestAContactWithNoAddressStillGetsTheirStop.
//
// The stop is what the subject asked for; the acknowledgement is what the
// decree adds. Refusing the first because the second cannot be addressed would
// cost them the thing they wanted to satisfy the thing they did not.
func TestAContactWithNoAddressStillGetsTheirStop(t *testing.T) {
	e := setupChannelConsent(t)
	stager := owingAcknowledgement(t, e, true)

	if err := e.store.Suppress(e.ctx, SuppressInput{
		ContactID: e.contact, Kind: commsauthz.ReasonObjection,
	}); err != nil {
		t.Fatalf("a contact with no address could not be stopped: %v — the stop is the thing "+
			"they asked for", err)
	}
	if stager.calls != 0 {
		t.Errorf("an acknowledgement was staged for a contact with no address: %d", stager.calls)
	}

	var standing bool
	if err := e.owner.QueryRow(context.Background(), `
		SELECT EXISTS (SELECT 1 FROM communication_suppression
		                WHERE contact_id = $1 AND revoked_at IS NULL)`,
		e.contact).Scan(&standing); err != nil {
		t.Fatalf("reading the stop: %v", err)
	}
	if !standing {
		t.Error("the stop was not recorded")
	}
}

// TestAnUnsubscribePressIsAcknowledged is Codex's finding, and it was the
// common case.
//
// A press writes per-purpose withdrawals and no suppression at all, so hooking
// only the suppression door answered the act a rep records and stayed silent on
// the one a subject performs — an obligation that looks discharged and is not.
func TestAnUnsubscribePressIsAcknowledged(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	seedMarketingPurpose(t, e)
	stager := owingAcknowledgement(t, e, true)

	if _, err := e.store.Record(e.ctx, RecordInput{
		ContactID: e.contact, PurposeID: e.newsletter, NewState: "granted",
		PolicyText: &grantWording,
	}); err != nil {
		t.Fatalf("granting before the press: %v", err)
	}

	withdrawn, err := e.store.PublicStopAllMarketing(e.ctx, e.contact)
	if err != nil {
		t.Fatalf("the press failed: %v", err)
	}
	if len(withdrawn) == 0 {
		t.Fatal("the press withdrew nothing, so this test would pass on an acknowledgement " +
			"that was correctly skipped")
	}
	if stager.calls != 1 {
		t.Errorf("the press staged %d acknowledgements, want 1 — this is the ordinary way a "+
			"Vietnamese recipient refuses advertising", stager.calls)
	}
}

// TestAReplayedPressAcknowledgesOnce. The subject performed one act, and a
// replay changes no state — telling them again that we stopped is a second
// message they did not ask for.
func TestAReplayedPressAcknowledgesOnce(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	seedMarketingPurpose(t, e)
	stager := owingAcknowledgement(t, e, true)

	if _, err := e.store.Record(e.ctx, RecordInput{
		ContactID: e.contact, PurposeID: e.newsletter, NewState: "granted",
		PolicyText: &grantWording,
	}); err != nil {
		t.Fatalf("granting before the press: %v", err)
	}
	if _, err := e.store.PublicStopAllMarketing(e.ctx, e.contact); err != nil {
		t.Fatalf("the first press failed: %v", err)
	}
	if _, err := e.store.PublicStopAllMarketing(e.ctx, e.contact); err != nil {
		t.Fatalf("the replay failed: %v", err)
	}
	if stager.calls != 1 {
		t.Errorf("two presses staged %d acknowledgements, want 1 — a replay withdraws nothing "+
			"and owes nothing", stager.calls)
	}
}

// TestTheStopSurvivesAnAcknowledgementThatCannotBeStaged is Codex's finding,
// and the consequence was that advertising stayed permitted.
//
// The stop is what the subject asked for; the acknowledgement is what the
// decree adds. Letting the second roll back the first inverts their importance
// — and the case is not hypothetical, since a contact whose address has
// hard-bounced cannot be acknowledged at all.
func TestTheStopSurvivesAnAcknowledgementThatCannotBeStaged(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager := owingAcknowledgement(t, e, true)
	stager.fail = errors.New("this message cannot be staged")

	if err := e.store.Suppress(e.ctx, SuppressInput{
		ContactID: e.contact, Kind: commsauthz.ReasonObjection, Reason: "No more ads.",
	}); err != nil {
		t.Fatalf("a refusal was lost because its acknowledgement could not be staged: %v — "+
			"advertising stays permitted, which is the opposite of what the subject said", err)
	}

	var standing bool
	if err := e.owner.QueryRow(context.Background(), `
		SELECT EXISTS (SELECT 1 FROM communication_suppression
		                WHERE contact_id = $1 AND revoked_at IS NULL)`,
		e.contact).Scan(&standing); err != nil {
		t.Fatalf("reading the stop: %v", err)
	}
	if !standing {
		t.Error("the stop was rolled back by its own acknowledgement")
	}
}
