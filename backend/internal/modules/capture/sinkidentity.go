// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// IdentityResolver answers which activity already holds an external identity
// AND may be joined by this caller, and IdentityClaimer binds one to an
// activity.
//
// Two seams rather than one method set, because capture asks the two questions
// at different moments: it resolves BEFORE it inserts, to find the row the
// other door filed, and claims AFTER, so the identity names a row that exists.
//
// activities owns `activity_identity`, and capture never imports a sibling, so
// both travel the way AudienceRecomputer and AssertedTakeOver do — compose
// injects activities' own functions.
//
// The resolver decides ELIGIBILITY, not just existence, and capture relies on
// that: a row held by a different seat, or one that is no longer live, answers
// not-found. Both rules read `activity.captured_by` and `activity.archived_at`,
// which only the owning module may read, and both must stay there. Were capture
// to make the judgement it could only do so from the record in hand — and every
// field of that record came off the wire from whoever sent the message.
type IdentityResolver func(ctx context.Context, tx pgx.Tx, kind, key string) (ids.ActivityID, bool, error)

// IdentityClaimer binds an external identity to an activity, reporting whether
// this activity ended up holding it. A lost claim is not an error — see
// activities.ClaimIdentity.
type IdentityClaimer func(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, kind, key, attestedBy string) (bool, error)

// MeetingIdentityKeyer composes one calendar occurrence's identity from the
// parts a provider stated: the series, and the meeting within it.
//
// It travels as a seam rather than being spelled here because the format IS the
// agreement between the two doors — an importer states `ical_instance` as text
// while a connector reads a start as an instant, and only one function may
// decide that those are the same meeting. activities.MeetingIdentityKey is that
// function, and a second copy would fail silently: the resolve would simply
// never match, and both doors would go on landing a row each.
type MeetingIdentityKeyer func(series, occurrence string) string

// WithMessageIdentity returns a copy that files each captured record under the
// identity every ingestion door agrees on.
//
// A sink without it captures exactly as it did before: the natural key is the
// only identity, and a record an importer already filed lands a second time.
//
// The kinds are activities.IdentityKindMail and IdentityKindMeeting, passed in
// rather than spelled here: the vocabulary belongs to the module that owns the
// table and its CHECK, and a second copy of either word would be silent in the
// direction that matters.
func (s *Sink) WithMessageIdentity(
	mailKind, meetingKind string, meetingKey MeetingIdentityKeyer,
	resolve IdentityResolver, claim IdentityClaimer,
) *Sink {
	out := *s
	out.mailIdentityKind = mailKind
	out.meetingIdentityKind = meetingKind
	out.meetingIdentityKey = meetingKey
	out.resolveIdentity = resolve
	out.claimIdentity = claim
	return &out
}

// identityOfRecord is the (kind, key) this record is known by across doors, or
// empty when it carries none.
//
// The two shapes answer differently because their natural keys differ in what
// they already promise:
//
//   - MAIL needs no composition. mailmap files every message under
//     (EmailSourceSystem, Message-ID), and every transport agrees on that
//     Message-ID, so the natural key already IS the shared identity.
//   - A MEETING's natural key is the provider's own event id, which two
//     calendars number differently for one meeting. Its identity is the series
//     plus the occurrence, stated by the connector and composed by the seam.
//
// A record stating neither carries no cross-door identity, which is not an
// error: it captures under its natural key exactly as it always did.
func (s *Sink) identityOfRecord(rec connector.NormalizedRecord) (kind, key string) {
	if s.mailIdentityKind != "" &&
		rec.NaturalKey.SourceSystem == connector.EmailSourceSystem && rec.NaturalKey.SourceID != "" {
		return s.mailIdentityKind, rec.NaturalKey.SourceID
	}
	if s.meetingIdentityKind != "" && s.meetingIdentityKey != nil && rec.CrossDoorIdentity.Stated() {
		return s.meetingIdentityKind, s.meetingIdentityKey(
			rec.CrossDoorIdentity.Series,
			// The occurrence as an instant, handed to the keyer as the text the
			// import door states, so both doors reach one spelling through one
			// function.
			rec.CrossDoorIdentity.Occurrence.UTC().Format(time.RFC3339))
	}
	return "", ""
}

// activityHoldingIdentity answers the activity another door already filed this
// message under, and whether one did.
//
// False covers four cases deliberately, because none is an error and each
// leads to the same place: this sink files no cross-door identity, this record
// carries none, the identity is free, or the resolver refused to bind — a
// different seat holds it, or its holder was erased. The caller captures the
// message as its own row either way, which is what keeps the refusal from
// becoming an existence oracle (activities.ResolveBindableIdentity).
func (s *Sink) activityHoldingIdentity(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord,
) (ids.ActivityID, bool, error) {
	kind, key := s.identityOfRecord(rec)
	if key == "" || s.resolveIdentity == nil {
		return ids.ActivityID{}, false, nil
	}
	return s.resolveIdentity(ctx, tx, kind, key)
}

// claimRecordIdentity files a newly captured row under the identity the other
// door will look for.
//
// A LOSING claim is not an error, and the claim's own contract says so: a
// racing arrival got there first and holds the identity, while this row keeps
// its own natural key, its links and its content. Nothing is retried here —
// the next sync of either door resolves onto whichever row won.
func (s *Sink) claimRecordIdentity(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord,
) error {
	kind, key := s.identityOfRecord(rec)
	if key == "" || s.claimIdentity == nil {
		return nil
	}
	// The connector's own stamp: this identity was OBSERVED by a provider that
	// held the message, not asserted by a caller who typed its header.
	_, err := s.claimIdentity(ctx, tx, id, kind, key, capturedByFor(ctx, rec))
	return err
}
