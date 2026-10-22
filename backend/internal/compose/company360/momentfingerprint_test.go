// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// What a set-aside account card is held against. A reader's "Not now" hides
// the card whose fingerprint it stored, so two cards a reader can tell apart
// must not share one.

import "testing"

func TestTwoPromisesFromOneMessageAreSetAsideApart(t *testing.T) {
	first := commitment("Send the NDA", 3, "Carol Wagner")
	second := commitment("Send the quote", 3, "Carol Wagner")
	// One message, one moment: everything the evidence carries is shared.
	second.ActivityID, second.OccurredAt = first.ActivityID, first.OccurredAt

	a := momentCopyFor("en").accountClaimCard(momentNow, first, false)
	b := momentCopyFor("en").accountClaimCard(momentNow, second, false)
	if a.EvidenceFingerprint == b.EvidenceFingerprint {
		t.Error("two promises read out of one message share a fingerprint; setting one aside hides both")
	}
}

func TestAPromiseMovedToTodayComesBackFromASetAside(t *testing.T) {
	later := task("Send the signed contract", 30)
	today := later
	today.DueAt = due(0)
	if momentCopyFor("en").accountTaskCard(momentNow, later, false).EvidenceFingerprint ==
		momentCopyFor("en").accountTaskCard(momentNow, today, false).EvidenceFingerprint {
		t.Error("a task moved from next month to today keeps its fingerprint, so it stays set aside")
	}

	claim := commitment("Send the NDA", 30, "Carol Wagner")
	moved := claim
	moved.DueAt = due(0)
	if momentCopyFor("en").accountClaimCard(momentNow, claim, false).EvidenceFingerprint ==
		momentCopyFor("en").accountClaimCard(momentNow, moved, false).EvidenceFingerprint {
		t.Error("a claim moved to today keeps its fingerprint, so it stays set aside")
	}
}
