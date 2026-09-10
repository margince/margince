// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// A withdrawal link outlives the mail that carried it.
//
// The defect these cover: preference_token is one credential doing two jobs —
// it opens the preference centre, which reads and grants, and it is also what
// the RFC 8058 one-click POST carries. It slides 30 days and is revoked on
// every rotation, so pressing unsubscribe on a two-year-old newsletter answers
// "this link is no longer valid" and the only remaining way to stop the mail is
// to ask a human. That is the friction one-click unsubscribe exists to remove.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// mintWithdrawal mints a credential the way a send path will, returning the
// token that goes in the mail.
func mintWithdrawal(t *testing.T, e *channelConsentEnv, in WithdrawalMintInput) string {
	t.Helper()
	var token string
	err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		token, err = e.store.EnsureWithdrawalCredentialTx(e.ctx, tx, in)
		return err
	})
	if err != nil {
		t.Fatalf("minting the withdrawal credential: %v", err)
	}
	return token
}

// TestAWithdrawalLinkSurvivesTheReadTokensRotation is the whole slice in one
// test: the preference token rotates and dies, and the withdrawal link written
// into the same mail still withdraws.
func TestAWithdrawalLinkSurvivesTheReadTokensRotation(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	seedSubjectAddress(t, e)
	address := "subject-" + e.person.String() + "@example.test"

	token := mintWithdrawal(t, e, WithdrawalMintInput{
		Address:  address,
		PersonID: e.person,
		Scope:    WithdrawalScopeAllMarketing,
	})
	if token == "" {
		t.Fatal("the first mint returned no token, so no link could be written into the mail")
	}

	// Every preference token this person holds rotates and is revoked, which is
	// what happens on the next send today.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE preference_token SET revoked_at = now(), revoked_reason = 'rotated'
		  WHERE person_id = $1`, e.person); err != nil {
		t.Fatalf("rotating the preference tokens: %v", err)
	}

	ref, err := e.store.ResolveWithdrawalToken(e.ctx, token)
	if err != nil {
		t.Fatalf("the withdrawal link stopped working when the READ token rotated: %v", err)
	}
	if ref.Address != address {
		t.Errorf("the link speaks for %q, want the address the mail went to (%q)", ref.Address, address)
	}
	if ref.Scope != WithdrawalScopeAllMarketing {
		t.Errorf("scope is %q, want %q", ref.Scope, WithdrawalScopeAllMarketing)
	}
}

// EVERY MESSAGE CARRIES A WORKING LINK, and one subscription still has one
// live credential.
//
// The first spelling returned nothing on the second send, reasoning that the
// existing link still worked. It does — but the send path reads an empty
// token as "no unsubscribe surface" and ships no header at all. For a person
// that quietly degraded to the preference token; for a LEAD there is no
// fallback, so every send after the first went out with no opt-out. So the
// old credential is superseded and a fresh one minted.
func TestEverySendCarriesAWorkingLinkAndOnlyOneStaysLive(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	address := "subject-" + e.person.String() + "@example.test"
	in := WithdrawalMintInput{Address: address, PersonID: e.person, Scope: WithdrawalScopeAllMarketing}

	first := mintWithdrawal(t, e, in)
	second := mintWithdrawal(t, e, in)

	if first == "" || second == "" {
		t.Fatal("a send got no token, so its message carries no List-Unsubscribe header")
	}
	if first == second {
		t.Fatal("the second send reissued the first token, which the hash makes impossible " +
			"to do honestly — it can only mean the mint read it back from somewhere")
	}
	var live int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM withdrawal_credential
		  WHERE lower(address) = lower($1) AND revoked_at IS NULL`, address).Scan(&live); err != nil {
		t.Fatalf("counting live credentials: %v", err)
	}
	if live != 1 {
		t.Errorf("%d live credentials for one subscription, want 1 — two working links is "+
			"two bearer credentials to leak, and revoking one leaves the other alive", live)
	}
	// The newest link works, which is the one in the message the recipient
	// most recently received.
	if _, err := e.store.ResolveWithdrawalToken(e.ctx, second); err != nil {
		t.Errorf("the newest link does not work: %v", err)
	}
	// The superseded one does not, and says so in the row rather than merely
	// vanishing.
	if _, err := e.store.ResolveWithdrawalToken(e.ctx, first); err == nil {
		t.Error("the superseded link still resolves, so one subscription has two live credentials")
	}
	var reason string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT revoked_reason FROM withdrawal_credential
		  WHERE lower(address) = lower($1) AND revoked_at IS NOT NULL`, address).Scan(&reason); err != nil {
		t.Fatalf("reading the superseded row: %v", err)
	}
	if reason != WithdrawalRevokedSuperseded {
		t.Errorf("the retired credential says %q, want %q", reason, WithdrawalRevokedSuperseded)
	}
}

// A LEAD GETS A WORKING OPT-OUT. preference_token.person_id is NOT NULL, so a
// lead-only recipient gets no unsubscribe surface at all today: the send simply
// carries no header.
func TestALeadOnlyRecipientGetsAWorkingOptOut(t *testing.T) {
	e := setupChannelConsent(t)
	var leadID ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`INSERT INTO lead (full_name, email, source, captured_by)
		 VALUES ('Lead Recipient', $1, 'test', 'human:x') RETURNING id`,
		"lead-optout@example.test").Scan(&leadID); err != nil {
		t.Fatalf("seeding the lead: %v", err)
	}

	token := mintWithdrawal(t, e, WithdrawalMintInput{
		Address: "lead-optout@example.test",
		LeadID:  ids.From[ids.LeadKind](leadID),
		Scope:   WithdrawalScopeAllMarketing,
	})
	if token == "" {
		t.Fatal("a lead recipient got no withdrawal link, so their mail carries no working opt-out")
	}
	ref, err := e.store.ResolveWithdrawalToken(e.ctx, token)
	if err != nil {
		t.Fatalf("the lead's opt-out link does not resolve: %v", err)
	}
	if ref.LeadID.UUID != leadID {
		t.Errorf("the link speaks for lead %s, want %s", ref.LeadID, leadID)
	}
	if !ref.PersonID.IsZero() {
		t.Error("the link names a person, but no person holds this address")
	}
}

// AN OLD LINK STILL WITHDRAWS. Every link already in a mailbox carries a
// preference token, so refusing them would ship the fix with every existing
// withdrawal link still broken.
func TestAnExpiredPreferenceTokenStillWithdraws(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	var legacy string
	if err := e.owner.QueryRow(context.Background(),
		`INSERT INTO preference_token (person_id, token, expires_at, revoked_at, revoked_reason)
		 VALUES ($1, 'pref_legacy_expired', now() - interval '400 days', now() - interval '300 days', 'rotated')
		 RETURNING token`, e.person).Scan(&legacy); err != nil {
		t.Fatalf("seeding the legacy token: %v", err)
	}

	ref, err := e.store.ResolveWithdrawalToken(e.ctx, legacy)
	if err != nil {
		t.Fatalf("an expired, rotated preference token no longer withdraws: %v — "+
			"every link already in a mailbox carries one of these", err)
	}
	if !ref.Legacy {
		t.Error("the ref does not say it came from a legacy token, so a caller cannot tell it apart")
	}
	// It carries WITHDRAWAL authority and nothing more.
	if ref.Scope != WithdrawalScopeAllMarketing {
		t.Errorf("a legacy link resolved to scope %q, want all_marketing — a preference token names "+
			"no subscription, so no narrower reading is honest", ref.Scope)
	}
}

// THE THREE REASONS THAT ARE NOT ROTATIONS. Honouring any of them would let a
// link act for a subject who is gone, or for a holder who is not the recipient.
func TestATokenRevokedForErasureOrCompromiseNeverWithdrawsAgain(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	for _, reason := range []string{"erasure", "compromise", "merged_predecessor"} {
		t.Run(reason, func(t *testing.T) {
			token := "pref_dead_" + reason
			if _, err := e.owner.Exec(context.Background(),
				`INSERT INTO preference_token (person_id, token, expires_at, revoked_at, revoked_reason)
				 VALUES ($1, $2, now() + interval '30 days', now(), $3)`,
				e.person, token, reason); err != nil {
				t.Fatalf("seeding the revoked token: %v", err)
			}
			// Unexpired on purpose: the refusal must come from the REASON, not
			// from the clock, or the test proves only that expiry works.
			if _, err := e.store.ResolveWithdrawalToken(e.ctx, token); err == nil {
				t.Fatalf("a token revoked for %s still withdraws", reason)
			}
		})
	}
}

// Unknown, revoked and expired answer alike, so the surface is not an oracle
// for which of the three a probe found.
func TestAnUnknownWithdrawalTokenIsIndistinguishableFromARevokedOne(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	address := "subject-" + e.person.String() + "@example.test"
	live := mintWithdrawal(t, e, WithdrawalMintInput{
		Address: address, PersonID: e.person, Scope: WithdrawalScopeAllMarketing,
	})
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return e.store.RevokeSubjectCredentialsTx(e.ctx, tx, e.person, WithdrawalRevokedErasure)
	}); err != nil {
		t.Fatalf("revoking: %v", err)
	}

	_, revokedErr := e.store.ResolveWithdrawalToken(e.ctx, live)
	_, unknownErr := e.store.ResolveWithdrawalToken(e.ctx, "wd_nothing_here_at_all")
	if revokedErr == nil {
		t.Fatal("a revoked credential still resolves")
	}
	if unknownErr == nil {
		t.Fatal("an unknown credential resolves")
	}
	if revokedErr.Error() != unknownErr.Error() {
		t.Errorf("revoked answers %q and unknown answers %q — the difference tells a prober "+
			"that the address had a subscription", revokedErr, unknownErr)
	}
	if !errors.Is(revokedErr, apperrors.ErrNotFound) {
		t.Errorf("a revoked credential answers %v, want ErrNotFound", revokedErr)
	}
}

// THE SEND PATH mints the credential the mail's List-Unsubscribe header
// carries, and it reaches a lead — which the preference token cannot.
func TestTheSendPathMintsAWorkingLinkForALeadOnlyAddress(t *testing.T) {
	e := setupChannelConsent(t)
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO lead (full_name, email, source, captured_by)
		 VALUES ('Header Lead', $1, 'test', 'human:x')`,
		"header-lead@example.test"); err != nil {
		t.Fatalf("seeding the lead: %v", err)
	}

	token, ok, err := e.store.WithdrawalTokenForEmail(e.ctx, "header-lead@example.test", "")
	if err != nil {
		t.Fatalf("minting for the send path: %v", err)
	}
	if !ok || token == "" {
		t.Fatal("a lead-only recipient got no unsubscribe token, so their marketing mail " +
			"goes out with no List-Unsubscribe header at all")
	}
	ref, err := e.store.ResolveWithdrawalToken(e.ctx, token)
	if err != nil {
		t.Fatalf("the header's link does not resolve: %v", err)
	}
	if ref.LeadID.IsZero() {
		t.Error("the credential names no lead, so nothing connects the press to the record")
	}
}

// A PERSON WINS OVER A LEAD holding the same address, because a promoted lead's
// mail is the person's. Both records can legitimately carry one address —
// uq_person_email_dedupe bounds person_email alone — so this is reachable in a
// way two live PERSONS are not.
func TestAPersonWinsOverALeadHoldingTheSameAddress(t *testing.T) {
	e := setupChannelConsent(t)
	shared := "both-records@example.test"
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO person_email (person_id, email, is_primary, source, captured_by)
		 VALUES ($1, $2, true, 'test', 'human:x')`, e.person, shared); err != nil {
		t.Fatalf("seeding the person's address: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO lead (full_name, email, source, captured_by)
		 VALUES ('Same Address Lead', $1, 'test', 'human:x')`, shared); err != nil {
		t.Fatalf("seeding the lead: %v", err)
	}

	token, ok, err := e.store.WithdrawalTokenForEmail(e.ctx, shared, "")
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if !ok {
		t.Fatal("no link was minted for an address two records hold")
	}
	ref, err := e.store.ResolveWithdrawalToken(e.ctx, token)
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if ref.PersonID != e.person {
		t.Errorf("the credential names %v, want the person %v — a promoted lead's mail is "+
			"the person's, so the person is who the opt-out acts for", ref.PersonID, e.person)
	}
	if !ref.LeadID.IsZero() {
		t.Error("the credential names a lead as well as a person, so two records claim one link")
	}
}

// A LEAD'S PRESS ACTUALLY STOPS THE MAIL. Resolving the link is not the same
// as acting on it: the first spelling resolved a lead's credential and then
// answered 404, because withdrawing a per-purpose consent state needs a person
// and a lead holds none. That handed a lead a link that worked right up to the
// moment it mattered.
func TestALeadsPressRecordsAStopRatherThanRefusing(t *testing.T) {
	e := setupChannelConsent(t)
	var leadID ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`INSERT INTO lead (full_name, email, source, captured_by)
		 VALUES ('Pressing Lead', $1, 'test', 'human:x') RETURNING id`,
		"pressing-lead@example.test").Scan(&leadID); err != nil {
		t.Fatalf("seeding the lead: %v", err)
	}
	token := mintWithdrawal(t, e, WithdrawalMintInput{
		Address: "pressing-lead@example.test",
		LeadID:  ids.From[ids.LeadKind](leadID),
		Scope:   WithdrawalScopeAllMarketing,
	})
	ref, err := e.store.ResolveWithdrawalToken(e.ctx, token)
	if err != nil {
		t.Fatalf("resolving the lead's link: %v", err)
	}

	if err := e.store.StopForCredential(e.ctx, ref); err != nil {
		t.Fatalf("the lead's press did not record a stop: %v", err)
	}

	var stops int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_suppression
		  WHERE lead_id = $1 AND revoked_at IS NULL`, leadID).Scan(&stops); err != nil {
		t.Fatalf("counting the lead's stops: %v", err)
	}
	if stops != 1 {
		t.Fatalf("the lead holds %d live stop(s) after pressing unsubscribe, want 1 — "+
			"their link resolved and then did nothing", stops)
	}

	// A REPLAY CHANGES NOTHING. Mailbox providers retry, and two live rows of
	// one kind would mean the second lift re-enables mail the first refused.
	if err := e.store.StopForCredential(e.ctx, ref); err != nil {
		t.Fatalf("the replayed press errored: %v", err)
	}
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_suppression
		  WHERE lead_id = $1 AND revoked_at IS NULL`, leadID).Scan(&stops); err != nil {
		t.Fatalf("recounting: %v", err)
	}
	if stops != 1 {
		t.Errorf("a replayed press left %d live stops, want 1", stops)
	}
}

// THE ROTATION THE ADAPTER DEPENDS ON must survive the new constraint. The
// legacy adapter honours a token revoked as 'rotated' and refuses one revoked
// for erasure, so the constraint requires every revocation to name a reason —
// and the production rotation writer set only revoked_at. Every marketing send
// that rotated a token would have aborted.
//
// This drives the REAL writer rather than writing the reason by hand, which is
// what let the first version of these tests pass over the defect.
func TestTheProductionRotationNamesItsReason(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	address := "subject-" + e.person.String() + "@example.test"

	first, found, err := e.store.PreferenceTokenForEmail(e.ctx, address)
	if err != nil || !found {
		t.Fatalf("minting the first preference token: %v (found=%v)", err, found)
	}
	// Age it past the ceiling so the next mint must rotate rather than reuse.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE preference_token SET created_at = now() - interval '400 days',
		        expires_at = now() - interval '1 day'
		  WHERE person_id = $1`, e.person); err != nil {
		t.Fatalf("ageing the token: %v", err)
	}

	second, found, err := e.store.PreferenceTokenForEmail(e.ctx, address)
	if err != nil {
		t.Fatalf("the rotation aborted: %v — every marketing send rotating a token "+
			"would fail this way", err)
	}
	if !found || second == first {
		t.Fatalf("no rotation happened (found=%v, same token=%v)", found, second == first)
	}

	var reason *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT revoked_reason FROM preference_token
		  WHERE person_id = $1 AND revoked_at IS NOT NULL`, e.person).Scan(&reason); err != nil {
		t.Fatalf("reading the rotated row: %v", err)
	}
	if reason == nil || *reason != "rotated" {
		t.Errorf("the rotated token says reason %v, want \"rotated\" — the withdrawal adapter "+
			"honours that reason and refuses erasure, so an unnamed one is unclassifiable", reason)
	}
	// AND THE OLD LINK STILL WITHDRAWS, which is the property the reason exists
	// to make decidable.
	if _, err := e.store.ResolveWithdrawalToken(e.ctx, first); err != nil {
		t.Errorf("the rotated-away link no longer withdraws: %v", err)
	}
}

// AN ALL-MARKETING LINK STOPS MARKETING AND LEAVES CORRESPONDENCE ALONE.
//
// The legacy one-click sweep stops every purpose in the catalog except the
// locked transactional one, so a press also ends business correspondence: the
// person who unsubscribed from a newsletter stops receiving replies to their
// own enquiries. Narrowing THAT changes what links already in mailboxes do, so
// it belongs to the slice owning the purpose vocabulary. A new credential is
// owed no such breadth, and its scope is called all_marketing.
func TestAnAllMarketingLinkLeavesBusinessCorrespondenceRunning(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	for _, p := range []struct{ key, class string }{
		{"newsletter_blast", "marketing"},
		{"cold_calling", "phone_outreach"},
		{"business_correspondence", "business_correspondence"},
	} {
		if _, err := e.owner.Exec(context.Background(),
			`INSERT INTO consent_purpose (key, label, class) VALUES ($1, $1, $2)
			 ON CONFLICT (key) DO UPDATE SET class = EXCLUDED.class`, p.key, p.class); err != nil {
			t.Fatalf("seeding purpose %s: %v", p.key, err)
		}
	}

	stopped, err := e.store.WithdrawMarketingForCredential(e.ctx, e.person)
	if err != nil {
		t.Fatalf("the credential's press failed: %v", err)
	}

	for _, want := range []string{"newsletter_blast", "cold_calling"} {
		if !slices.Contains(stopped, want) {
			t.Errorf("the press left %q running, and a link that says all_marketing "+
				"has to stop it", want)
		}
	}
	if slices.Contains(stopped, "business_correspondence") {
		t.Error("the press stopped business correspondence — the person who unsubscribed " +
			"from a newsletter would stop receiving replies to their own enquiries")
	}
	if slices.Contains(stopped, PurposeTransactional) {
		t.Error("the press stopped transactional mail, which is locked and not a subscription")
	}
}

// A CREDENTIAL PRESS TELLS THE PRESSER NOTHING ABOUT THE CONSENT STATE.
//
// The response used to name the purposes it changed, and an empty list meant
// "already unsubscribed". That is consent state, disclosed through a mutation
// to a long-lived bearer token that is specifically not allowed to read one:
// anyone holding a link out of a forwarded mail could ask whether the
// recipient had already opted out, and an all-marketing press would enumerate
// the workspace's purpose keys besides.
func TestACredentialPressDisclosesNoConsentState(t *testing.T) {
	first := answeredKeys([]string{"newsletter_blast", "cold_calling"}, true)
	second := answeredKeys([]string{}, true)
	if len(first) != 0 {
		t.Errorf("a credential press answered %v — that names the purposes this workspace "+
			"runs and says the recipient was still subscribed", first)
	}
	if len(first) != len(second) {
		t.Error("a first press and a replayed one answer differently, so the response " +
			"still says whether the recipient had already unsubscribed")
	}
	// The preference token keeps the real list: its holder can read the whole
	// state on the next GET anyway, and the unsubscribe screen uses the empty
	// list to say "already off" rather than "stopped".
	if got := answeredKeys([]string{"newsletter_blast"}, false); len(got) != 1 {
		t.Errorf("a preference-token press answered %v, want the purposes it changed", got)
	}
}
