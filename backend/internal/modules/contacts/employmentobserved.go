// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// employmentFirstObservedSQL is the earliest message from any of a contact's
// addresses on a domain. The message being captured now counts on its own,
// since its participant rows may not be visible yet. It is a lower bound, never the start, so it goes
// in first_observed_at. A header dated ahead of the server's clock is forgeable
// and does not count. A held or archived message does: too early a date only
// keeps the reach unbounded. Every argument is an SQL expression.
func employmentFirstObservedSQL(contact, domain, activity, skew string) string {
	return storekit.SQLf(`least(
		(SELECT min(ev.occurred_at)
		   FROM contact_email pe
		   JOIN activity_participant ap ON lower(ap.address) = lower(pe.email)
		   JOIN activity ev ON ev.id = ap.activity_id
		  WHERE pe.contact_id = %[1]s
		    -- A literal suffix compare, never LIKE — see OnDomain.
		    AND (split_part(pe.email, '@', 2) = %[2]s
		         OR right(split_part(pe.email, '@', 2), length(%[2]s) + 1) = '.' || %[2]s)
		    AND ev.occurred_at <= now() + %[4]s::interval),
		(SELECT ev.occurred_at FROM activity ev
		  WHERE ev.id = %[3]s AND ev.occurred_at <= now() + %[4]s::interval))`,
		contact, domain, activity, skew)
}

// observeEmployment moves an existing edge's first observation earlier when
// this message predates it. A backfill walks old mail after new. Without this,
// an edge planted by recent mail hides the older mail from the account.
//
// Only an edge with no date pays for the evidence scan. A dated edge already
// holds the earliest evidence seen so far, so it only ratchets to this message.
// The caller holds the contact's employment write lock.
func observeEmployment(ctx context.Context, tx pgx.Tx, in EnsureCounterpartyInput, domain string, contactID ids.ContactID, companyID ids.CompanyID) error {
	var (
		edgeID        ids.UUID
		before, after *time.Time
	)
	err := tx.QueryRow(ctx, `
		WITH msg AS (
			SELECT occurred_at FROM activity
			 WHERE id = $4 AND occurred_at <= now() + $5::interval),
		edge AS (
			SELECT r.id, r.first_observed_at
			  FROM relationship r
			 WHERE r.contact_id = $1 AND r.company_id = $2 AND r.kind = 'employment'
			   AND r.archived_at IS NULL AND `+employment.IsCurrentSQL("r.ended_at")+`
			   AND (r.first_observed_at IS NULL OR r.first_observed_at > (SELECT occurred_at FROM msg))),
		seen AS (
			SELECT edge.id, edge.first_observed_at AS was,
			       CASE WHEN edge.first_observed_at IS NULL
			            THEN `+employmentFirstObservedSQL("$1", "$3", "$4", "$5")+`
			            ELSE (SELECT occurred_at FROM msg) END AS at
			  FROM edge)
		UPDATE relationship r
		   SET first_observed_at = seen.at
		  FROM seen
		 WHERE r.id = seen.id AND seen.at IS NOT NULL
		   AND (seen.was IS NULL OR seen.at < seen.was)
		RETURNING r.id, seen.was, r.first_observed_at`,
		contactID, companyID, domain, in.ActivityID, mailClockSkewAllowance).Scan(&edgeID, &before, &after)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("contacts: dating the employment edge from captured mail: %w", err)
	}
	return auditFirstObservation(ctx, tx, firstObservationMove{
		edge: edgeID, contact: contactID, company: companyID,
		before: before, after: after, origin: relationshipOriginCapture,
	})
}

// firstObservationMove is one edge whose first observation moved, and what
// moved it.
type firstObservationMove struct {
	edge          ids.UUID
	contact       ids.ContactID
	company       ids.CompanyID
	before, after *time.Time
	origin        string
}

// auditFirstObservation writes the audit row and the event for one move. The
// origin goes in the evidence, because it is not a column that changed.
func auditFirstObservation(ctx context.Context, tx pgx.Tx, move firstObservationMove) error {
	auditID, err := storekit.AuditWithEvidence(ctx, tx, actionUpdate, tableRelationship, move.edge,
		map[string]any{fieldFirstObservedAt: move.before},
		map[string]any{fieldFirstObservedAt: move.after},
		map[string]any{"origin": move.origin})
	if err != nil {
		return fmt.Errorf("contacts: audit the employment edge's first observation: %w", err)
	}
	delta := map[string]any{
		eventKeyDelta: map[string]any{tableRelationship: map[string]any{
			"id": move.edge, relationshipKindField: employmentKind, employmentActionField: actionUpdate,
			companyFK: move.company, fieldFirstObservedAt: move.after, "origin": move.origin,
		}},
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, move.contact.UUID,
		relationshipUpdatedPayload(contactEntity, delta)); err != nil {
		return fmt.Errorf("contacts: publish the employment edge's first observation: %w", err)
	}
	return nil
}

// fieldFirstObservedAt is the column name the audit row and the event delta
// both carry, spelled once so the two cannot disagree.
const fieldFirstObservedAt = "first_observed_at"

// foldFirstObservations carries the earliest first observation of the
// employment edges a merge is about to archive as duplicates onto the edge that
// survives them. Without it the survivor keeps the later date and the merged
// record loses the older mail from the account. Call it before the archive.
//
// mergedColumn is the endpoint the merge rewrites ("contact_id" or
// "company_id"); the edges must agree on the other one.
func foldFirstObservations(ctx context.Context, tx pgx.Tx, mergedColumn string, source, target ids.UUID) error {
	sharedColumn := "contact_id"
	if mergedColumn == "contact_id" {
		sharedColumn = "company_id"
	}
	rows, err := tx.Query(ctx, storekit.SQLf(`
		UPDATE relationship s SET first_observed_at = f.earliest
		  FROM (SELECT b.id, b.first_observed_at AS was, min(a.first_observed_at) AS earliest
		          FROM relationship a
		          JOIN relationship b ON b.kind = a.kind AND b.id <> a.id AND b.archived_at IS NULL
		           AND b.%[2]s = a.%[2]s AND b.%[1]s = $2
		         WHERE a.kind = 'employment' AND a.archived_at IS NULL AND a.%[1]s = $1
		           AND `+roleKeyedDuplicateSQL+`
		         GROUP BY b.id, b.first_observed_at) f
		 WHERE s.id = f.id AND f.earliest IS NOT NULL
		   AND (f.was IS NULL OR f.earliest < f.was)
		RETURNING s.id, s.contact_id, s.company_id, f.was, s.first_observed_at`,
		mergedColumn, sharedColumn), source, target)
	if err != nil {
		return fmt.Errorf("contacts: folding the merged employment edges' first observations: %w", err)
	}
	var moves []firstObservationMove
	for rows.Next() {
		move := firstObservationMove{origin: "merge"}
		if err := rows.Scan(&move.edge, &move.contact, &move.company, &move.before, &move.after); err != nil {
			rows.Close()
			return fmt.Errorf("contacts: folding the merged employment edges' first observations: %w", err)
		}
		moves = append(moves, move)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("contacts: folding the merged employment edges' first observations: %w", err)
	}
	for _, move := range moves {
		if err := auditFirstObservation(ctx, tx, move); err != nil {
			return err
		}
	}
	return nil
}
