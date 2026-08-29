// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"strings"
	"testing"
	"time"
)

// The key is the claim's PATH, never its value. Keyed on the value, a verdict
// would evaporate the moment the evidence shifted — which is precisely when
// the human's answer matters most.
func TestClaimKeyDependsOnThePathAndNotTheValue(t *testing.T) {
	a := ClaimKey("profile_field:title")
	b := ClaimKey("profile_field:title")
	if a != b {
		t.Fatal("the same claim path hashed twice gave two keys; no verdict could survive a re-derivation")
	}
	if a == ClaimKey("profile_field:phone") {
		t.Error("two different claims share a key; suppressing one would suppress the other")
	}
	if len(a) != 64 {
		t.Errorf("key length = %d, want a 64-char sha256 hex digest", len(a))
	}
}

// Normalization happens in ONE place so two surfaces cannot spell the same
// claim differently and lose each other's verdicts.
func TestClaimKeyNormalizesSpacingAndCase(t *testing.T) {
	canonical := ClaimKey("moment:went_quiet")
	for _, variant := range []string{
		"  moment:went_quiet  ",
		"MOMENT:WENT_QUIET",
		"Moment:Went_Quiet\n",
	} {
		if ClaimKey(variant) != canonical {
			t.Errorf("ClaimKey(%q) differs from the canonical spelling", variant)
		}
	}
}

// The lookup key is composite: the same claim path under two different kinds
// is two claims, and a consulting surface must not read one as the other.
func TestVerdictLookupKeySeparatesKinds(t *testing.T) {
	path := ClaimKey("profile_field:title")
	if VerdictLookupKey(ClaimProfileField, path) == VerdictLookupKey(ClaimSignal, path) {
		t.Error("the same path under two claim kinds produced one lookup key")
	}
	if !strings.HasPrefix(VerdictLookupKey(ClaimSignal, path), ClaimSignal+":") {
		t.Error("the lookup key does not lead with its claim kind, so the two halves cannot be told apart")
	}
}

// Every subject the ledger accepts is also an RBAC object the write is gated
// on. A value here with no object would gate on nothing.
func TestEverySubjectTypeIsAnRBACObject(t *testing.T) {
	for subject := range feedbackSubjects {
		if strings.TrimSpace(subject) == "" {
			t.Error("an empty subject type would gate auth.Require on nothing")
		}
	}
	for _, want := range []string{"organization", "person", "deal", "lead"} {
		if !feedbackSubjects[want] {
			t.Errorf("%q is in the column's CHECK but not accepted here", want)
		}
	}
	if feedbackSubjects["activity"] {
		t.Error("a subject outside the column's CHECK would fail at the constraint rather than at the door")
	}
}

// A human's decision is about the value that was in front of them. These are
// the two directions the ruling needs, and it needs BOTH: an assertion that
// only a stale verdict is dropped passes just as happily against an overlay
// that never fires at all, and one that only a current verdict stands passes
// against an overlay with no recency check — which is the defect this exists
// for.
func TestAVerdictAppliesToTheValueItWasRecordedAgainst(t *testing.T) {
	corrected, note := "VP Sales", "they run the team, not the function"
	recorded := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	v := NewVerdict(ClaimProfileField, ClaimKey("profile_field:role"),
		VerdictCorrected, &corrected, &note, recorded)

	for _, tc := range []struct {
		name    string
		valueAt time.Time
		applies bool
	}{
		{
			// The ordinary case: the machine wrote a value, a human read it
			// and disagreed. Their correction is what the page shows.
			name:    "a verdict recorded after the value it corrects stands",
			valueAt: recorded.Add(-time.Hour),
			applies: true,
		}, {
			// The defect. Something replaced the value the human was looking
			// at — an accepted research claim, a fresh enrichment, an edit
			// through another door — and their correction now describes a
			// value the record no longer holds.
			name:    "a verdict older than the value it corrects does not",
			valueAt: recorded.Add(time.Hour),
			applies: false,
		}, {
			// A verdict and the value it is about can be written in ONE
			// transaction, where both carry the same now(). Refusing that
			// would suppress a decision at the instant it was made.
			name:    "a verdict recorded in the same instant stands",
			valueAt: recorded,
			applies: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision, applies := v.AsOf(tc.valueAt)
			if applies != tc.applies {
				t.Fatalf("applies = %v, want %v", applies, tc.applies)
			}
			if !applies {
				// The MARKER goes with the value. "corrected" beside a value
				// the human never wrote says they wrote it, which is the same
				// lie one field along.
				if decision.Verdict != "" || decision.CorrectedValue != nil || decision.Note != nil {
					t.Errorf("a stale verdict still handed back %+v", decision)
				}
				return
			}
			if decision.Verdict != VerdictCorrected {
				t.Errorf("verdict = %q, want %q", decision.Verdict, VerdictCorrected)
			}
			if decision.CorrectedValue == nil || *decision.CorrectedValue != corrected {
				t.Errorf("corrected value = %v, want %q", decision.CorrectedValue, corrected)
			}
			if decision.Note == nil || *decision.Note != note {
				t.Errorf("note = %v, want %q", decision.Note, note)
			}
		})
	}
}

// A verdict with no corrected value — "confirmed", "suppressed" — is dated the
// same way. The human confirmed an answer; a different answer is not the one
// they confirmed, and carrying their marker onto it says they saw it.
func TestAMarkerWithoutACorrectionIsDatedToo(t *testing.T) {
	recorded := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	v := NewVerdict(ClaimProfileField, ClaimKey("profile_field:role"),
		VerdictConfirmed, nil, nil, recorded)

	if _, applies := v.AsOf(recorded.Add(-time.Hour)); !applies {
		t.Error("a confirmation of the value on file was dropped")
	}
	if _, applies := v.AsOf(recorded.Add(time.Hour)); applies {
		t.Error("a confirmation outlived the value it confirmed — the page would say a human had seen an answer they never saw")
	}
}
