// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Capture judges Bcc for the mailbox the mail came through. A second reader
// named in To or Cc of the same message was addressed openly, so the lane
// reports no Bcc for them.
func TestABccCopyIsBccOnlyForAReaderNamedNowhere(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	message := e.waitingFrom(t, "View the document", "sender@example.com", contact)
	e.exec(t, `UPDATE activity SET bcc_only = true, sender_was_stranger = true, foreign_link = true
		WHERE id = $1`, message)
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'cc', 'copied@ourco.example')`, ids.NewV7(), message)

	unnamed, ok := present(t, storeAddressing(e, "reader@ourco.example"), e, message)
	if !ok {
		t.Fatal("the stranger's mail left the queue; it must be demoted, never hidden")
	}
	if !unnamed.BccOnly || !unnamed.SenderWasStranger || !unnamed.ForeignLink {
		t.Errorf("a reader named nowhere read bcc=%v stranger=%v foreign=%v, want all three",
			unnamed.BccOnly, unnamed.SenderWasStranger, unnamed.ForeignLink)
	}
	copied, _ := present(t, storeAddressing(e, "copied@ourco.example"), e, message)
	if copied.BccOnly {
		t.Error("a reader named in Cc read the message as a Bcc copy")
	}
}

// NULL is mail captured before the facts existed, and reads as no evidence.
func TestUnjudgedMailCarriesNoStrangerFacts(t *testing.T) {
	e := setupLoad(t)
	message := e.waitingFrom(t, "Question", "sender@example.com", e.buyer(t))

	row, ok := present(t, storeAddressing(e, "reader@ourco.example"), e, message)
	if !ok {
		t.Fatal("the message is not waiting")
	}
	if row.BccOnly || row.SenderWasStranger || row.ForeignLink {
		t.Errorf("unjudged mail read bcc=%v stranger=%v foreign=%v, want none", row.BccOnly, row.SenderWasStranger, row.ForeignLink)
	}
}
