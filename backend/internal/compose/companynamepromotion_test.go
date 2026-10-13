// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// What makes two company-name proposals the same QUESTION — the rule a human's
// refusal is remembered by.
func TestRefusedNameKeyComparesTheClaimNotTheSpelling(t *testing.T) {
	company := ids.New[ids.CompanyKind]()

	// A refusal recorded by today's code: it carries the normalized key.
	current, err := json.Marshal(companyNameProposal{
		CompanyID: company, CurrentName: "Gitex",
		ProposedName: "Gitex Global GmbH", ProposedNameKey: "gitex global",
	})
	if err != nil {
		t.Fatal(err)
	}
	// A refusal recorded BEFORE proposed_name_key existed: raw spelling only,
	// and a different spelling of the very same claim.
	legacy := json.RawMessage(`{"company_id":"` + company.String() +
		`","current_name":"Gitex","proposed_name":"GITEX  GLOBAL  GmbH"}`)

	tests := []struct {
		name    string
		refused []json.RawMessage
		key     string
		want    bool
	}{
		{"today's refusal binds its own claim", []json.RawMessage{current}, "gitex global", true},
		{
			// The finding this test exists for: the legacy payload holds
			// dominantSpelling's pick, which moves as signatures accumulate.
			// Comparing spellings would forget the refusal the moment it did.
			name:    "a pre-upgrade refusal binds despite a spelling-only change",
			refused: []json.RawMessage{legacy}, key: "gitex global", want: true,
		},
		{"an unrelated claim is not refused", []json.RawMessage{current, legacy}, "acme", false},
		{"no refusals at all", nil, "gitex global", false},
		{
			// Defensive: a payload of some other shape is not this kind's
			// refusal, and must not panic or match by accident.
			name:    "a payload this kind cannot read is skipped",
			refused: []json.RawMessage{json.RawMessage(`["not","an","object"]`)}, key: "gitex global", want: false,
		},
		{
			// An empty key must never match an unreadable/absent name, or one
			// malformed refusal would suppress every later proposal.
			name:    "an empty key matches nothing",
			refused: []json.RawMessage{json.RawMessage(`{"proposed_name":""}`)}, key: "", want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := refusedNameKey(tc.refused, tc.key); got != tc.want {
				t.Errorf("refusedNameKey(_, %q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

// What each verdict does to a signature before it is counted as evidence.
//
// Driven through ruledSignatures rather than the sweep, because the question is
// the ruling itself: a human's decision about one contact's company_name reaches
// this reader like every other, and the three verdicts say three different
// things. Their SQL path is exercised in the integration lane.
func TestEachVerdictDoesItsOwnThingToASignature(t *testing.T) {
	contact := ids.New[ids.ContactKind]()
	captured := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	decided := captured.Add(time.Hour)
	claim := ai.VerdictLookupKey(ai.ClaimProfileField,
		ai.ClaimKey(ai.ProfileFieldClaimPath(companyNameField)))

	signature := func() []contacts.SignatureCompanyName {
		return []contacts.SignatureCompanyName{
			{ContactID: contact, Value: "Gitex Global GmbH", CapturedAt: captured},
		}
	}
	ledger := func(verdict string, corrected *string) map[ids.UUID]map[string]ai.Verdict {
		shown := "Gitex Global GmbH"
		return map[ids.UUID]map[string]ai.Verdict{
			contact.UUID: {claim: ai.NewVerdict(ai.ClaimProfileField, string(ai.ProfileFieldClaimPath(companyNameField)),
				verdict, corrected, nil, decided, &captured, &shown)},
		}
	}
	corrected := "Gitex Global SE"

	t.Run("suppressed stops counting", func(t *testing.T) {
		// A human said this observation is wrong. A wrong observation
		// corroborates nothing, whatever it is asked to corroborate — and
		// corroboration is the whole safety property this sweep rests on.
		kept := ruledSignatures(signature(), ledger(ai.VerdictSuppressed, nil), claim)
		if len(kept) != 0 {
			t.Errorf("a suppressed signature still counts: %+v", kept)
		}
	})

	t.Run("corrected counts, using the human's value", func(t *testing.T) {
		kept := ruledSignatures(signature(), ledger(ai.VerdictCorrected, &corrected), claim)
		if len(kept) != 1 || kept[0].Value != corrected {
			t.Fatalf("a corrected signature reads %+v, want one line valued %q", kept, corrected)
		}
		if kept[0].Confirmed {
			t.Error("a corrected signature is marked confirmed; only a confirmation is")
		}
	})

	t.Run("confirmed counts, and is marked", func(t *testing.T) {
		kept := ruledSignatures(signature(), ledger(ai.VerdictConfirmed, nil), claim)
		if len(kept) != 1 || !kept[0].Confirmed {
			t.Fatalf("a confirmed signature reads %+v, want one line marked confirmed", kept)
		}
	})

	t.Run("an unreviewed signature is untouched", func(t *testing.T) {
		kept := ruledSignatures(signature(), map[ids.UUID]map[string]ai.Verdict{}, claim)
		if len(kept) != 1 || kept[0].Confirmed || kept[0].Value != "Gitex Global GmbH" {
			t.Fatalf("an unreviewed signature reads %+v, want it as captured", kept)
		}
	})

	// The verdict is about the value that was in front of the human. An
	// accepted research claim replaces the whole row, and a decision about what
	// the row USED to say must not strike what it says now.
	t.Run("a verdict about a value since replaced does not apply", func(t *testing.T) {
		moved := signature()
		moved[0].Value = "Gitex Holding AG"
		kept := ruledSignatures(moved, ledger(ai.VerdictSuppressed, nil), claim)
		if len(kept) != 1 {
			t.Errorf("a stale suppression struck the value that replaced it: %+v", kept)
		}
	})
}
