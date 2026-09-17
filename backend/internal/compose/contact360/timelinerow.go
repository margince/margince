// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// Turning one timeline row into an Activity.
//
// Split out of readActivities because it IS a separate concept: that function
// decides which rows this contact may see and in what order, and this one
// decides what a row means once it arrives. Keeping them together pushed the
// file past the length cap, and the cap was right — the two change for
// different reasons and are read by readers asking different questions.
//
// This is a hand-written twin of activities.activityProjection, which is how
// the contact timeline came to be missing a column for a whole slice, twice.
// Every column the list learns to carry has to be taught here too, and
// TestTheContact360TimelineNamesTheTransportThatCarriedAMessage in
// compose/integration is what says so out loud — with two siblings beside it
// for the version and the transcript source, each a later instance of the same
// defect.

import (
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sourceAuthorColumns is the author's three columns, for the SELECT that reads
// them.
//
// Subselected rather than joined: that query is hand-written, and a join would
// be a fourth place to keep in step with the activities list's own projection.
// A column carrying its own source needs nothing of its caller.
//
// NO liveness filter on the name lookup. Who wrote something in August is a
// fact about August: a colleague who has since left was still its author, and
// readEmailParties already refuses the same filter for the same reason.
const sourceAuthorColumns = `a.source_author_id, a.source_author_name,
		       (SELECT u.display_name FROM app_user u WHERE u.id = a.source_author_id)`

// scanTimelineRow reads one row of the timeline query into an Activity.
//
// The column order here and the SELECT list in readActivities are one thing in
// two places; a column added to either without the other is a scan error at
// best and a silently shifted field at worst.
func scanTimelineRow(rows pgx.Rows, contactID ids.ContactID) (crmcontracts.Activity, error) {
	var a crmcontracts.Activity
	var id ids.UUID
	var audience string
	var version int64
	var contentAvailable, bulkMailAttested, filedHere bool
	var threadKey, audienceReason, authorName, authorSeatName *string
	var authorID *ids.UUID
	if err := rows.Scan(&id, &a.Kind, &a.ChannelProvider, &a.Subject, &a.Body,
		&a.Direction, &a.OccurredAt, &a.DueAt, &a.IsDone, &a.AssigneeId, &a.Source, &a.CapturedBy,
		&a.CreatedAt, &threadKey, &bulkMailAttested, &audience, &audienceReason,
		&a.SourceSystem, &version, &contentAvailable,
		&authorID, &authorName, &authorSeatName, &filedHere); err != nil {
		return crmcontracts.Activity{}, err
	}
	a.Id = openapi_types.UUID(id)
	aud := crmcontracts.ActivityAudience(audience)
	a.Audience = &aud
	a.Version = &version
	// The thread key and the bulk attestation are what lets the record page fold
	// this page into conversations the way the list's page folds; the key
	// identifies the message at the provider, so it is withheld with the
	// content, exactly as the list's scan withholds it.
	a.BulkMailAttested = &bulkMailAttested
	a.ThreadKey = threadKey
	// Why the row is held travels with the row. The record page seeds its
	// timeline from this read, so a reason dropped here is a reason the timeline
	// never has — and the timeline is where an owner decides whether to share
	// the thread.
	a.AudienceReason = audienceReason
	state := crmcontracts.ActivityContentStateAvailable
	if !contentAvailable {
		state = crmcontracts.ActivityContentStateWithheld
		// The reason describes what the message is about, so it is withheld with
		// the content: a colleague who may not read a held message does not
		// learn why it is held either.
		a.Subject, a.Body, a.ThreadKey, a.AudienceReason = nil, nil, nil, nil
	}
	a.ContentState = &state
	// Who wrote it where it came from, folded through the SAME helper the
	// activities list uses so the two timelines cannot disagree about which of
	// the two names wins — including about when it is shown at all.
	//
	// WITHHELD with the content, and set after the withholding above for the
	// same reason the email summary is. It is a free-text name that arrived
	// with imported text, about a human usually party to neither side, and the
	// Art. 17 redaction clears it alongside the subject and the body. A field
	// the erasure treats as content cannot be a marker here.
	if contentAvailable {
		a.Author = activities.SourceAuthorOf(authorID, authorSeatName, authorName, a.SourceSystem)
	}
	// Composed from the shared helper rather than spelled again here. The
	// contract says an email row carries a summary exactly when kind=email, and
	// this hand-written twin of the projection is the one read that could make
	// that false. Set AFTER the withholding above, so a withheld row's summary
	// is the withheld one.
	a.EmailSummary = activities.RowEmailSummary(a)
	// Links say how the message is FILED, and a row can reach this page without
	// being filed here: a contact who was CC'd or who attended is on the message
	// through their participant row, while the filing belongs to whoever capture
	// named as its counterparty. Asserting a link for those would describe a row
	// activity_link does not hold — and a client acting on it, to unfile the
	// message, would act on nothing.
	if filedHere {
		a.Links = &[]crmcontracts.ActivityLink{{
			EntityType: crmcontracts.ActivityLinkEntityTypeContact,
			EntityId:   openapi_types.UUID(contactID.UUID),
		}}
	}
	return a, nil
}
