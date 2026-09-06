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
		 WHERE person_id = $1 ORDER BY captured_at DESC, id DESC LIMIT 1`,
		e.person).Scan(&text, &version); err != nil {
		t.Fatalf("reading the proof row: %v", err)
	}
	return text, version
}

// TestAWithdrawalNeedsNoWording is the other half, and the one that matters
// more: nothing is demonstrated when somebody takes consent back, so requiring
// a sentence there would leave a person unable to opt out.
func TestAWithdrawalNeedsNoWording(t *testing.T) {
	e := setupChannelConsent(t)

	if _, err := e.store.Record(e.ctx, RecordInput{
		PersonID: e.person, PurposeID: e.newsletter, NewState: "withdrawn",
	}); err != nil {
		t.Fatalf("withdrawing without wording: %v — a person must always be able to opt out", err)
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
		PersonID: e.person, PurposeID: e.newsletter, NewState: "granted",
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
		INSERT INTO consent_event (person_id, purpose_id, new_state, source,
		                           policy_text, policy_version, captured_at, captured_by)
		VALUES ($1, $2, 'granted', 'test', 'a sentence with no version', NULL, now(), 'test')`,
		e.person, e.newsletter)

	if err == nil {
		t.Fatal("a row carrying wording with no version was accepted: consent_event_wording_pairs " +
			"is not holding, so a proof row can name a sentence nothing identifies")
	}
	if !strings.Contains(err.Error(), "consent_event_wording_pairs") {
		t.Errorf("refused by %v, want the pair CHECK", err)
	}
}
