// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// EVERY PROPOSAL A DATA SUBJECT SENDS OPENS A CASE SOMEBODY OWES AN ANSWER TO.
//
// stageSubmission files what the subject asked for; openRightsCaseTx puts it in
// the DPO's queue with a deadline and a receipt. The two are one act, and the
// defect this slice closed was having only the first: a correction or erasure
// request landed in person_confirm_submission where nobody owned it, the
// statutory month ran anyway, and the subject held no reference to chase.
//
// stageOneProposal is where the pair is spelled, and this test is what keeps
// them together. They stay together while ONE statement files a submission. A second writer — a new submission kind added later, a retry
// path written in a hurry — files the proposal and opens nothing, and nothing
// else in the tree fails: the subject sees the same thank-you, the row exists,
// and the queue simply never learns.
//
// WHAT THIS GATE CAN AND CANNOT SEE. It counts the INSERT in consent's string
// literals, which is what a copy would carry and what renaming a helper cannot
// escape. A writer reaching the table from another module is outside this root;
// table ownership is what refuses that, and it is gated separately.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// TestOneWriterFilesAConfirmSubmission is what stageOneProposal's doc comment
// names, and it counts the only thing a copy cannot rename its way out of: the
// INSERT itself. One writer means one path from "the subject asked" to "the
// queue knows", and stageOneProposal is that path.
func TestOneWriterFilesAConfirmSubmission(t *testing.T) {
	t.Parallel()
	const key = "INSERT INTO person_confirm_submission"
	scope := gatekit.Scope{
		Roots:   []string{consentRoot},
		Subject: fileContains(key),
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	total, where := countAcross(t, scope, key)
	if total != 1 {
		t.Errorf("person_confirm_submission is written from %d place(s), want exactly 1: %s\n\n"+
			"Every proposal a subject sends must reach the rights case that answers it, and the "+
			"one writer is what makes that provable.", total, strings.Join(where, ", "))
	}
}
