// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The lane reads Bcc from the reader's own import row only, and a To or Cc row
// naming one of the reader's addresses overrides it. The capture side is
// proven through the real sink in compose's strangermail integration test.
func TestBccIsTheReadersOwnImportUnlessToOrCcNamesThem(t *testing.T) {
	e := setupLoad(t)
	e.exec(t, `UPDATE app_user SET email=$2 WHERE id=$1`, e.rep, "reader-"+e.rep.String()+"@ourco.example")
	message := e.waitingFrom(t, "View the document", "sender@example.com", e.buyer(t))
	e.directlyTo(t, message, e.other)
	e.exec(t, `UPDATE activity SET sender_was_stranger = true, foreign_link = true WHERE id = $1`, message)

	row, ok := present(t, storeAddressing(e, "reader@ourco.example"), e, message)
	if !ok {
		t.Fatal("the stranger's mail left the queue; it must be demoted, never hidden")
	}
	if row.BccOnly || !row.SenderWasStranger || !row.ForeignLink {
		t.Errorf("with no import of the reader's: bcc=%v stranger=%v foreign=%v, want no Bcc and both others",
			row.BccOnly, row.SenderWasStranger, row.ForeignLink)
	}

	e.exec(t, `INSERT INTO capture_import (activity_id, user_id, bcc_only) VALUES ($1, $2, true)`, message, e.rep)
	if row, _ := present(t, storeAddressing(e, "reader@ourco.example"), e, message); !row.BccOnly {
		t.Error("the reader's own Bcc import did not reach the lane")
	}

	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'cc', $3)`, ids.NewV7(), message, "reader-"+e.rep.String()+"@ourco.example")
	if row, _ := present(t, storeAddressing(e, "reader@ourco.example"), e, message); row.BccOnly {
		t.Error("a reader named in Cc read the message as a Bcc copy")
	}
}

// NULL is mail captured before the facts existed, and reads as no evidence.
func TestUnjudgedMailCarriesNoStrangerFacts(t *testing.T) {
	e := setupLoad(t)
	message := e.waitingFromToRep(t, "Question", "sender@example.com", e.buyer(t))
	e.exec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, message, e.rep)

	row, ok := present(t, storeAddressing(e, "reader@ourco.example"), e, message)
	if !ok {
		t.Fatal("the message is not waiting")
	}
	if row.BccOnly || row.SenderWasStranger || row.ForeignLink {
		t.Errorf("unjudged mail read bcc=%v stranger=%v foreign=%v, want none", row.BccOnly, row.SenderWasStranger, row.ForeignLink)
	}
}
