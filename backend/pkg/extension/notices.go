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
	// see answers ErrNotFound. A unit that sends too often answers
	// ErrRateLimited, and the same call works later.
	Send(ctx context.Context, in NoticeRequest) (NoticeSent, error)
}

// NoticeRequest is one notice. Name one of Member and Role, not both.
type NoticeRequest struct {
	// Member is a user id. Role is a role key, and reaches every live member
	// who holds it.
	Member string
	Role   string
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

// RecordRef names one core record. Type is "contact", "company" or "deal".
type RecordRef struct {
	Type string
	ID   string
}

// NoticeSent says how many members were newly told. A member a retry under the
// same Key had already told is not counted.
type NoticeSent struct {
	Recipients int
}
