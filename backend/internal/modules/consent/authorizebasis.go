// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Writing down the lawful ground a send relied on.
//
// A basis nobody wrote down is an assertion, and Art. 5(2) puts the burden of
// showing it on the controller. Until now communication_basis had an erasure
// writer, a retention writer and a subject-access reader — and no writer at
// all, so the export answered "we relied on nothing" for every message ever
// sent.
//
// WHAT IS WRITTEN IS BOUNDED, which is the difference from the qualifying-event
// row this sits beside. A qualifying event never expires: one inbound message
// makes a person permanently correspondable-with, which is the shape that turns
// a single reply into an open licence. A basis row carries valid_until, so the
// ground it records runs out the way the evidence behind it does.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// recordBasis writes the ground a supported resolution rests on, once.
//
// ONCE is enforced by the read below rather than by a unique index, because
// "the same basis" is not a column: two replies on one thread rest on the same
// ground, and a row per message would make the export a second copy of the
// mailbox. A live row of the same kind and scope already says what a new one
// would.
//
// A failure to write is a failure of the send. The row is the evidence the
// message was permitted, so committing the delivery without it would put out
// mail the installation cannot account for — which is the exact gap this table
// exists to close.
func recordBasis(ctx context.Context, tx pgx.Tx, subject subjectRef, res resolution, req commsauthz.Request, w windows) error {
	if !res.Supported || res.Basis == "" {
		// Nothing was concluded, so there is nothing to stand behind. An
		// unsupported resolution falls through to the legacy verdict, whose own
		// grounds are already recorded as consent rows.
		return nil
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	threadKey := threadKeyOf(ctx, tx, req.AnchorActivityID)
	live, err := basisAlreadyLive(ctx, tx, subject, res.Basis, threadKey)
	if err != nil {
		return err
	}
	if live {
		return nil
	}
	validUntil := time.Now().Add(basisLifetime(res.Category, w))
	var personID, leadID *string
	if subject.Kind == entityPerson {
		personID = &subject.ID
	} else {
		leadID = &subject.ID
	}
	var sourceActivity *ids.UUID
	if req.AnchorActivityID != (ids.UUID{}) {
		anchor := req.AnchorActivityID
		sourceActivity = &anchor
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO communication_basis
		  (person_id, lead_id, kind, thread_key, source_activity_id, valid_until, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		personID, leadID, string(res.Basis), nullableText(threadKey),
		sourceActivity, validUntil, by); err != nil {
		return fmt.Errorf("consent: record the ground this message relied on: %w", err)
	}
	// AUDITED, like every other consent-basis write beside it
	// (RecordQualifyingEvent). The row is served to a data subject as "the
	// ground we relied on", so a trail saying when it was written and who
	// caused it is part of the same obligation. Against the PERSON, because
	// that is the record a later reader asks about.
	//
	// Audit without an event: there is no public event type for a lawful basis,
	// and inventing one would publish a legal conclusion about somebody to
	// every subscriber. Ratified in the write-shape gate's auditOnlyWrites for
	// that reason, the same way its sibling is.
	subjectID, err := ids.Parse(subject.ID)
	if err != nil {
		return fmt.Errorf("consent: the subject a ground was recorded for is not an id: %w", err)
	}
	if _, err := storekit.AuditEvent(ctx, tx, "update", subjectAuditEntity(subject), subjectID, map[string]any{
		"communication_basis": string(res.Basis),
		"resolved_category":   string(res.Category),
	}); err != nil {
		return err
	}
	return nil
}

// subjectAuditEntity names the table an audit row hangs off. A lead and a
// person are different records, and an entry pointing at the wrong one is a
// trail nobody finds.
func subjectAuditEntity(subject subjectRef) string {
	if subject.Kind == entityLead {
		return entityLead
	}
	return entityPerson
}

// basisLifetime is how long the ground a category rests on stays good.
//
// It mirrors the window the evidence was checked against, so the row expires
// when the thing behind it stops supporting a send. A basis outliving its
// evidence would be a record saying a message was lawful on a ground that had
// already run out.
func basisLifetime(category commsauthz.Category, w windows) time.Duration {
	if category == commsauthz.CategoryActiveDealFollowup {
		return w.dealFollow
	}
	return w.reply
}

// basisAlreadyLive reports whether a live row of this kind already covers the
// subject on this thread.
//
// Scoped by thread as well as kind, because two threads are two conversations:
// a basis earned on one is not the ground for writing about another, and
// collapsing them would make the first reply a licence for every later subject.
func basisAlreadyLive(ctx context.Context, tx pgx.Tx, subject subjectRef, basis commsauthz.Basis, threadKey string) (bool, error) {
	var found bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM communication_basis
			 WHERE ($1 = 'person' AND person_id = $2::uuid
			        OR $1 = 'lead' AND lead_id = $2::uuid)
			   AND kind = $3
			   AND revoked_at IS NULL
			   AND (valid_until IS NULL OR valid_until > now())
			   -- IS NOT DISTINCT FROM, so a threadless basis matches the
			   -- threadless case rather than never matching itself: NULL = NULL
			   -- is unknown, and a plain equality would write a new row on
			   -- every unthreaded send.
			   AND thread_key IS NOT DISTINCT FROM $4
		)`, subject.Kind, subject.ID, string(basis), nullableText(threadKey)).Scan(&found); err != nil {
		return false, fmt.Errorf("consent: read the grounds already on file: %w", err)
	}
	return found, nil
}

// threadKeyOf names the conversation an anchor belongs to, or "" when there is
// none.
//
// A read failure yields "" rather than an error, and the direction is worth
// stating because it is not obvious. The key SCOPES the row: a threadless basis
// matches MORE future sends in basisAlreadyLive, so losing it suppresses a
// later write rather than authorizing anything. Nothing reads communication_basis
// to permit a send — the engine decides from the evidence each time, and this
// table is the record it leaves behind — so the worst case is a thinner audit
// trail, and failing the send over it would refuse real mail to protect a
// narrowing nobody depends on.
func threadKeyOf(ctx context.Context, tx pgx.Tx, anchor ids.UUID) string {
	if anchor == (ids.UUID{}) {
		return ""
	}
	var key *string
	if err := tx.QueryRow(ctx,
		// archived_at excludes a restricted row too (the activity_restricted_is_archived
		// CHECK), so a held message never names the conversation a basis is
		// scoped to. Losing the key only widens the row, per the note above.
		`SELECT thread_key FROM activity WHERE id = $1 AND archived_at IS NULL`,
		anchor).Scan(&key); err != nil {
		return ""
	}
	if key == nil {
		return ""
	}
	return *key
}
