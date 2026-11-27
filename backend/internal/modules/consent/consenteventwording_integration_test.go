// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What a proof row may claim about what the subject was shown.

import (
	"context"
	"strings"
	"testing"
)

func wordingOf(t *testing.T, e *channelConsentEnv) (*string, *string) {
	t.Helper()
	var text, version *string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT policy_text, policy_version FROM consent_event
		 WHERE contact_id = $1 ORDER BY captured_at DESC, id DESC LIMIT 1`,
		e.contact).Scan(&text, &version); err != nil {
		t.Fatalf("reading the proof row: %v", err)
	}
	var current *string
	if err := e.owner.QueryRow(e.ctx, `SELECT policy_version FROM contact_consent WHERE contact_id = $1 AND purpose_id = $2`, e.contact, e.newsletter).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if (current == nil) != (version == nil) || (current != nil && *current != *version) {
		t.Fatalf("current version %v differs from proof version %v", current, version)
	}
	return text, version
}

// TestAWithdrawalNeedsNoWording is the other half, and the one that matters
// more: nothing is demonstrated when somebody takes consent back, so requiring
// a sentence there would leave a contact unable to opt out.
func TestAWithdrawalNeedsNoWording(t *testing.T) {
	e := setupChannelConsent(t)

	if _, err := e.store.Record(e.ctx, RecordInput{
		ContactID: e.contact, PurposeID: e.newsletter, NewState: "withdrawn",
	}); err != nil {
		t.Fatalf("withdrawing without wording: %v — a contact must always be able to opt out", err)
	}

	text, version := wordingOf(t, e)
	if text != nil || version != nil {
		t.Errorf("the withdrawal recorded wording %v / version %v, want both NULL: "+
			"a placeholder invents a claim about a screen that never existed", text, version)
	}
}

// TestAGrantRecordsTheWordingVerbatim is what the whole rule is for.
func TestAGrantRecordsTheWordingVerbatim(t *testing.T) {
	e := setupChannelConsent(t)
	shown := "Yes, email me about events. I can unsubscribe at any time."

	if _, err := e.store.Record(e.ctx, RecordInput{
		ContactID: e.contact, PurposeID: e.newsletter, NewState: "granted",
		PolicyText: &shown,
	}); err != nil {
		t.Fatalf("recording a grant with wording: %v", err)
	}

	text, version := wordingOf(t, e)
	if text == nil || *text != shown {
		t.Errorf("stored wording = %v, want the exact sentence shown", text)
	}
	// A door showing wording without naming a version still produces a row that
	// says which wording this was, via defaultWordingVersion. This asserts that
	// default is applied — not that the CHECK refused anything, which no path
	// through this writer can reach.
	if version == nil {
		t.Error("wording was stored with no version, so nothing names which sentence this was")
	}
}

// TestThePairCheckRefusesAHalfRecordedWording holds the constraint itself.
//
// The writer above cannot produce a half-filled pair, so the CHECK is never
// exercised by any test that goes through it — and an unexercised constraint is
// one nobody would notice losing. This writes the row the writer will not,
// straight past Go, and expects the database to refuse it.
func TestThePairCheckRefusesAHalfRecordedWording(t *testing.T) {
	e := setupChannelConsent(t)

	_, err := e.owner.Exec(e.ctx, `
		INSERT INTO consent_event (contact_id, purpose_id, new_state, source,
		                           policy_text, policy_version, captured_at, captured_by)
		VALUES ($1, $2, 'granted', 'test', 'a sentence with no version', NULL, now(), 'test')`,
		e.contact, e.newsletter)

	if err == nil {
		t.Fatal("a row carrying wording with no version was accepted: consent_event_wording_pairs " +
			"is not holding, so a proof row can name a sentence nothing identifies")
	}
	if !strings.Contains(err.Error(), "consent_event_wording_pairs") {
		t.Errorf("refused by %v, want the pair CHECK", err)
	}
}

func TestConsentStateTracksEachProofVersionAndClearsOnWithdrawal(t *testing.T) {
	e := setupChannelConsent(t)
	shown := "Send the newsletter"
	for _, version := range []string{"policy-1", "policy-2"} {
		in := RecordInput{ContactID: e.contact, PurposeID: e.newsletter, NewState: "granted", PolicyText: &shown, PolicyVersion: &version}
		grant, err := e.store.Record(e.ctx, in)
		if err != nil || !grant.Changed {
			t.Fatalf("grant %s: changed=%v err=%v", version, grant.Changed, err)
		}
		assertStateProofVersion(t, e, grant, &version)
		unchanged, err := e.store.Record(e.ctx, in)
		if err != nil || unchanged.Changed {
			t.Fatalf("idempotent grant: changed=%v err=%v", unchanged.Changed, err)
		}
		withdrawal, err := e.store.Record(e.ctx, RecordInput{ContactID: e.contact, PurposeID: e.newsletter, NewState: "withdrawn"})
		if err != nil || !withdrawal.Changed {
			t.Fatalf("withdrawal: changed=%v err=%v", withdrawal.Changed, err)
		}
		assertStateProofVersion(t, e, withdrawal, nil)
	}
}

func assertStateProofVersion(t *testing.T, e *channelConsentEnv, proof State, want *string) {
	t.Helper()
	var state, event *string
	if err := e.owner.QueryRow(e.ctx, `
		SELECT c.policy_version, e.policy_version FROM contact_consent c
		JOIN consent_event e ON e.id = $3
		WHERE c.contact_id = $1 AND c.purpose_id = $2`, e.contact, e.newsletter, proof.ProofEventID).Scan(&state, &event); err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]*string{"state": state, "proof": event} {
		if (got == nil) != (want == nil) || (got != nil && *got != *want) {
			t.Errorf("%s policy version = %v, want %v", name, got, want)
		}
	}
}
