// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The closed vocabulary of audience_reason: the words the derivation writes
// into the column, and the list of them every reader needs.
//
// Its own file because both halves are one subject and the recompute beside
// it is about deriving an audience rather than naming why. Splitting them
// also keeps that file under the size ceiling, which is the ceiling asking
// how much a reader must hold at once — and the answer here is "nothing else".

// AudienceReason names why a derived audience is what it is. Closed
// vocabulary, and the column it lands in is withheld from a reader who may not
// read the content: "held because personnel" describes the content.
const (
	// ReasonPosture: a mailbox asked for it. A verdict on the thread can clear
	// it, which is the whole point of a mailbox posture — it holds the message
	// until something judges it.
	ReasonPosture = "posture"
	// ReasonWorkspaceFloor: the WORKSPACE turned mail sharing off. It is not a
	// mailbox posture and no verdict clears it: an admin decided colleagues do
	// not read captured mail, and a classifier concluding a thread is ordinary
	// says nothing about that decision. Only the admin turning sharing back on
	// opens these rows, and only for mail captured afterwards.
	ReasonWorkspaceFloor = "workspace_floor"
	// ReasonPendingVerdict: nothing has judged the message yet, and unjudged
	// is held.
	ReasonPendingVerdict = "pending_verdict"
	// ReasonManual: a human said so. It is also a LOCK — the derivation refuses
	// to move a row carrying it — so nothing but a human's own decision may
	// write it.
	ReasonManual = "manual"
	// ReasonVerdict: a classifier judged the thread and held it, without saying
	// which kind. The classifier's own kinds replace this word when it lands.
	ReasonVerdict = "verdict"
	// ReasonCounterparty: the importing seat holds mail with one of the parties,
	// whatever this particular message is about. Their decision, and no verdict
	// clears it — a classifier concluding a thread is ordinary says nothing
	// about whether this seat wants their lawyer's mail in a shared CRM.
	ReasonCounterparty = "counterparty"
	// ReasonConfidentialMarker: the sender said so in the subject line. The one
	// confidentiality signal that needs no model, and it outranks a later
	// verdict for the same reason a counterparty hold does — a person marked
	// this message, and a classifier disagreeing does not unmark it.
	ReasonConfidentialMarker = "explicitly_confidential"
	// ReasonNoRecord: the message is filed under no record because something
	// JUDGED its sender — a suppression rule, a settled verdict, a thread the
	// owner's own. Written by the capture ladder rather than derived here, and
	// carried through every recompute: a link arriving later says nothing about
	// a judgement that was never about the filing.
	ReasonNoRecord = "no_record"
	// ReasonNoCounterparty: the message named nobody a record could be created
	// FOR. The calendar case — attendance is a list, so the mapper leaves the
	// counterparty unset and the ladder concludes it named nobody. No judgement
	// was made about anyone, so this hold means only "nothing has filed it
	// yet", and it stops being true the moment something does. That is the one
	// row-carried hold a link lifts, and noRecordHoldStands is where it does.
	ReasonNoCounterparty = "no_counterparty"
)

// EveryReason is every word this column can carry, for the readers that must
// handle all of them rather than the ones somebody remembered.
//
// It exists because the browser held five of these nine and drew NOTHING for
// the rest. `explicitly_confidential` is what a confidentiality verdict stamps,
// so an ordinary business mail narrowed to its participants told the reader
// only that it was not shared with them — never why, and a verdict nobody can
// see is a verdict nobody can correct.
//
// Listed rather than derived from the constants because Go cannot enumerate a
// const block, so a tenth reason could be declared above and missed here.
//
// Held by: TestEveryReasonIsListed (backend/gates/frontendaudiencereasons_test.go)
//
// It reads the constant declarations in THIS file and fails when one is absent
// below. Its sibling, TestTheBrowserNamesEveryAudienceReason, then holds the
// browser's label map against this function in both directions.
func EveryReason() []string {
	return []string{
		ReasonPosture,
		ReasonWorkspaceFloor,
		ReasonPendingVerdict,
		ReasonManual,
		ReasonVerdict,
		ReasonCounterparty,
		ReasonConfidentialMarker,
		ReasonNoRecord,
		ReasonNoCounterparty,
	}
}
