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

// ONE LIVE CREDENTIAL per address and scope. Two working links for one
// subscription is two bearer credentials to leak, and revoking one would leave
// the other working.
func TestASecondSendReusesTheLinkTheFirstMailCarried(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	address := "subject-" + e.person.String() + "@example.test"
	in := WithdrawalMintInput{Address: address, PersonID: e.person, Scope: WithdrawalScopeAllMarketing}

	first := mintWithdrawal(t, e, in)
	second := mintWithdrawal(t, e, in)

	if first == "" {
		t.Fatal("the first mint produced no token")
	}
	// The second mint discloses nothing, because the row it would have written
	// already exists and the table holds only a hash.
	if second != "" {
		t.Errorf("the second mint returned a token, so a second live credential exists for one subscription")
	}
	var live int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM withdrawal_credential
		  WHERE lower(address) = lower($1) AND revoked_at IS NULL`, address).Scan(&live); err != nil {
		t.Fatalf("counting live credentials: %v", err)
	}
	if live != 1 {
		t.Errorf("%d live credentials for one address and scope, want 1", live)
	}
	// And the first link still works, which is what makes returning nothing safe.
	if _, err := e.store.ResolveWithdrawalToken(e.ctx, first); err != nil {
		t.Errorf("the original link stopped working after a re-send: %v", err)
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
