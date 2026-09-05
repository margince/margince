// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The door every mail-shaped operation goes through, and why it is one door.
//
// A capture connection is a mailbox or a calendar, and the ops that only a
// mailbox can answer — the four backfill ops, the mail posture — refused a
// calendar by accident before this: the registry found the calendar connector
// is not a Backfiller and answered `connector_unsupported`, "this provider
// cannot enumerate a mailbox backward from a date". True, and the wrong
// sentence. A calendar is not a mailbox that cannot be enumerated; it is not a
// mailbox. The connections screen turned that answer into "this mailbox type
// can't be backfilled" under a Google Calendar row drawn with an envelope
// beside the member's own email address, and the member reported the product
// refusing to import their mail.
//
// The mail posture had no door at all: the registry keys on the provider name
// and never asks what kind of connection it is, so it wrote a calendar row an
// answer to a question nobody could have asked it.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// mailboxOnly refuses an operation that only a mailbox can answer, naming the
// kind of connection this is and what to do instead. detail is the caller's,
// because "no mail history to import" and "no mail to take a posture towards"
// are different facts about the same connection.
func mailboxOnly(w http.ResponseWriter, r *http.Request, provider crmcontracts.CaptureProvider, detail string) bool {
	if IsMailProvider(string(provider)) {
		return true
	}
	httperr.Write(w, r, &httperr.DetailedError{
		Status: http.StatusUnprocessableEntity, Code: "not_a_mailbox", Detail: detail,
	})
	return false
}

// noMailHistory is what the backfill ops say. It names the way forward: on
// either vendor the mail and the calendar are separate connections on one
// account, so the reader almost always has the other one already.
const noMailHistory = "This is a calendar connection, which carries no mail history. " +
	"Import history from the mailbox connection on the same account."

// noMailToPosture is what the posture door says. There is no way forward to
// offer: a calendar brings in meetings, and the question does not apply to it.
const noMailToPosture = "This is a calendar connection, which brings in no mail to take a posture towards."
