// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestLifecycleScopeMustCiteTheCurrentMessage(t *testing.T) {
	source := ids.NewV7()
	thread := settledThread{Messages: []threadMessage{{
		ID: source, Body: "We are ending the Atlas pilot. Our support agreement continues.",
	}}}
	for _, test := range []struct {
		name, scope, quote, workstream string
		valid                          bool
	}{
		{"named pilot", "workstream", "We are ending the Atlas pilot.", "Atlas pilot", true},
		{"legacy unknown", "", "", "", true},
		{"unknown named engagement", "unknown", "", "Luna pilot", false},
		{"legacy named engagement", "", "", "Luna pilot", false},
		{"normalized source", "workstream", "We are ending the Atlas\npilot.", "Atlas pilot", true},
		{"invented whole account", "company", "All business ends.", "", false},
		{"unquoted scope", "company", "", "", false},
		{"invented engagement", "workstream", "We are ending the Atlas pilot.", "Luna pilot", false},
		{"invalid scope", "everything", "", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			event := extractedEvent{
				Kind: signalKindContractEnded, MessageID: source.String(),
				LifecycleScope: test.scope, Quote: test.quote, Workstream: test.workstream,
			}
			reason := validateLifecycleEvidence(event, thread)
			if (reason == "") != test.valid {
				t.Fatalf("validation=%q, want valid=%v", reason, test.valid)
			}
		})
	}
}

func TestRecordedLifecycleEvidenceKeepsSourceProofAndScope(t *testing.T) {
	source := ids.NewV7()
	event := extractedEvent{
		Kind: signalKindContractEnded, Summary: "An engagement ended.",
		Quote: "We are ending the Atlas pilot.", LifecycleScope: "workstream", Workstream: "Atlas pilot",
	}
	evidence := eventSignalEvidence(event, source, settledThread{Messages: []threadMessage{{ID: source, Body: event.Quote}}})
	if evidence.ActivityID != source || evidence.Snippet != event.Summary || evidence.SourceQuote == nil ||
		evidence.LifecycleScope != "workstream" || evidence.Workstream != "Atlas pilot" {
		t.Fatalf("recorded evidence lost its source scope: %+v", evidence)
	}
}

func TestAStrayScopeOnAnotherKindIsDroppedNotRefused(t *testing.T) {
	for _, scope := range []string{"", lifecycleScopeUnknown, lifecycleScopeCompany, lifecycleScopeWorkstream} {
		event := extractedEvent{Kind: extractKindCommitment, LifecycleScope: scope, Workstream: "invented engagement"}
		if reason := validateLifecycleEvidence(event, settledThread{}); reason != "" {
			t.Fatalf("scope %q refused a commitment: %s", scope, reason)
		}
		evidence := eventSignalEvidence(event, ids.NewV7(), settledThread{})
		if evidence.LifecycleScope != "" || evidence.Workstream != "" || evidence.SourceQuote != nil {
			t.Fatalf("scope %q persisted on a commitment: %+v", scope, evidence)
		}
	}
}
