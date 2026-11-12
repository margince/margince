// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import "context"

// NoticeRepo is the door a unit sends a notice through: `tx.Core().Notices().Send(…)`.
//
// A unit names who and what, never where. It passes no address and no channel:
// the core finds the member and decides how they are reached, from their own
// notification settings. A unit that held an address could mail anybody it
// pleased. That is a spam door with the product's name on it.
type NoticeRepo interface {
	// Send raises one notice about a record and answers how many members it newly told.
	//
	// The recipient is resolved when the call is made, against live rights. A
	// member who has left, was erased, or cannot open the record is left out,
	// so zero is a normal answer and not an error. A record the caller cannot
	// see answers ErrNotFound. A bus delivery has no caller, so it acts as the
	// unit's service member or as the member ActAs names; a send that has no
	// actor at all answers ErrForbidden. A unit that sends too often answers
	// ErrRateLimited, and the same call works later.
	Send(ctx context.Context, in NoticeRequest) (NoticeSent, error)
}

// NoticeRequest is one notice. Name one of Member and Role, not both.
type NoticeRequest struct {
	// Member is a user id. Role is a role key, and reaches every live member
	// who holds it.
	Member string
	Role   string
	// ActAs is the member a job tick or a bus delivery sends for. Leave it
	// empty to send as the unit's service member, which stays when a colleague
	// leaves; a unit with none declared answers ErrForbidden. A named member
	// must hold one of the unit's user-scoped secrets now, and the notice is
	// judged under their own rights, as an ingest is. A send with a caller
	// leaves it empty and runs as that caller.
	ActAs UserID
	// Record is what the notice is about. The body the reader sees is a link to
	// it. What a reader may see of the record is decided when they open it.
	// No personal data is copied into the notice.
	Record RecordRef
	// Subject is one short line of the unit's own words.
	Subject string
	// Key names the event the notice is about. A second send with the same key
	// to the same member writes nothing, so a retry never notifies twice.
	Key string
}

// contactRecord is a contact in the words core stores.
const contactRecord = "contact"

// NoticeRecordTypes are the record types a notice may be about: the identity
// records a reader opens by id.
func NoticeRecordTypes() []string {
	return []string{contactRecord, string(KindCompany), string(KindDeal)}
}

// RecordRef names one core record. Type is one of NoticeRecordTypes.
type RecordRef struct {
	Type string
	ID   string
}

// NoticeSent says how many members were newly told. A member a retry under the
// same Key had already told is not counted.
type NoticeSent struct {
	Recipients int
}
