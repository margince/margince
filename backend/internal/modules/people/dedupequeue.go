// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The dedupe review queue over dedupe_candidate (DH-DDL-1, DH-EXT-1/2):
// confidence-sorted reads with the detection-time evidence snapshot
// (DH-N-8 — rendered as captured, never re-derived), and the two
// dispositions. `merge` executes the owner's merge verb — mergePerson /
// mergeOrganization, ONE merge in the system — and `not_a_duplicate`
// flips the row that suppresses the pair from every future sweep
// (AC-dedupe-7: the unique pair index meets the row and re-proposes
// nothing).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ErrNotUndoable marks an undo on a merged pair: the merge verb's
// reversibility (PO-AC-M6) is not built, so the merge stands (409).
var ErrNotUndoable = errors.New("people: a merged pair cannot be re-opened — the merge stands")

// DedupeInputError marks caller input the queue refuses (422 on the wire).
type DedupeInputError struct {
	Field string
	Msg   string
}

func (e *DedupeInputError) Error() string { return "people: " + e.Field + ": " + e.Msg }

// FieldFault names the dedupe argument that was malformed.
func (e *DedupeInputError) FieldFault() (field, code, message string) {
	return e.Field, "invalid", e.Error()
}

// DedupeCandidateRow is one queue row as stored.
type DedupeCandidateRow struct {
	ID          ids.UUID
	EntityType  string // person | organization | lead
	LeftID      ids.UUID
	RightID     ids.UUID
	Confidence  float64
	Evidence    json.RawMessage // the detection-time snapshot, verbatim
	Disposition string          // open | merged | not_a_duplicate
	DisposedBy  *ids.UUID
	DisposedAt  *time.Time
	CreatedAt   time.Time
}

// DedupeQueueInput filters one list page.
type DedupeQueueInput struct {
	Status     string // open (default) | merged | not_a_duplicate
	EntityType string // "" = both
	Cursor     string
	Limit      int
}

const (
	dedupeQueueDefaultLimit = 25
	dispositionOpen         = "open"
	dispositionMerged       = "merged"
	dispositionNotDuplicate = "not_a_duplicate"
	// auditEntityDedupe names the queue row in its audit entries.
	auditEntityDedupe = "dedupe_candidate"
	// auditKeyDisposition is the audited field name of the queue verdict.
	auditKeyDisposition = "disposition"
	// sqlAlwaysVisible is the no-op arm a scoped predicate falls back to when
	// the caller's scope leaves that record type unbounded. Declared here and
	// used across this module's scoped reads.
	sqlAlwaysVisible = "true"
)

// dedupeCursor is the queue's keyset: confidence-descending with the id
// as the tiebreak — opaque on the wire.
type dedupeCursor struct {
	Confidence float64  `json:"c"`
	ID         ids.UUID `json:"id"`
}

// encodeDedupeCursor renders the queue's position. It has no error channel to
// a caller mid-page, and the position is a float and a uuid — nothing here can
// fail to marshal. An empty token would be refused on the way back in.
func encodeDedupeCursor(c dedupeCursor) string {
	token, err := storekit.EncodeOpaque(c)
	if err != nil {
		return ""
	}
	return token
}

func decodeDedupeCursor(token string) (dedupeCursor, error) {
	c, err := storekit.DecodeOpaque[dedupeCursor](token)
	if err != nil {
		return dedupeCursor{}, err
	}
	// Valid JSON is not yet a cursor. `null` and `{}` both unmarshal without
	// error and leave the keyset at its zero value, which reads as a real
	// position and pages from the top of the queue instead of refusing. Every
	// token this encodes names a row, so an absent id is the tell — and which
	// field tells is what stays here rather than moving to the envelope.
	if c.ID == (ids.UUID{}) {
		return dedupeCursor{}, &storekit.MalformedCursorError{}
	}
	return c, nil
}

// requireDedupeRead gates the queue read on the entities it exposes: the
// unfiltered queue shows both record types, so it needs both reads.
func requireDedupeRead(ctx context.Context, entityType string) error {
	if entityType == "" || entityType == entityPerson {
		if err := auth.Require(ctx, entityPerson, principal.ActionRead); err != nil {
			return err
		}
	}
	if entityType == "" || entityType == entityOrganization {
		if err := auth.Require(ctx, entityOrganization, principal.ActionRead); err != nil {
			return err
		}
	}
	if entityType == "" || entityType == entityLead {
		if err := auth.Require(ctx, entityLead, principal.ActionRead); err != nil {
			return err
		}
	}
	return nil
}

// requireDedupeWrite gates the disposition verbs: deciding a pair mutates
// the records' dedupe fate, so it needs update on the pair's entity type —
// a read-only seat can look at the queue but never decide it.
func requireDedupeWrite(ctx context.Context, entityType string) error {
	return auth.Require(ctx, entityType, principal.ActionUpdate)
}

// dedupeVisibilityClause renders the queue's row-scope filter: a candidate
// surfaces only when BOTH sides of its pair are visible to the caller —
// the evidence snapshot reads both records, so listing a pair IS a read of
// them (H1). Empty for unbounded callers.
func dedupeVisibilityClause(ctx context.Context, arg func(any) int) (string, error) {
	personClause, err := auth.ScopeClauseFor(ctx, entityPerson, "vp", arg)
	if err != nil {
		return "", err
	}
	orgClause, err := auth.ScopeClauseFor(ctx, entityOrganization, "vo", arg)
	if err != nil {
		return "", err
	}
	leadClause, err := auth.ScopeClauseFor(ctx, entityLead, "vl", arg)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`((entity_type = 'person' AND %s) OR (entity_type = 'organization' AND %s) OR (entity_type = 'lead' AND %s))`,
		bothSidesServable(entityPerson, "vp", "left_person_id", "right_person_id", personClause),
		bothSidesServable(entityOrganization, "vo", "left_org_id", "right_org_id", orgClause),
		bothSidesServable(entityLead, "vl", "left_lead_id", "right_lead_id", leadClause)), nil
}

// bothSidesServable is the per-side predicate for one entity type: the subject
// row must EXIST, be LIVE, and pass whatever row scope the caller has.
//
// Emitted unconditionally, where the predicate this replaced was skipped whole
// when no record type narrowed the caller. That was not the bug — person and
// organization are capture-private, so even an all-scope human is bounded on
// them and the clause was built anyway — but it made the liveness term depend on
// a scope the reader might not have. A system principal is unbounded on all
// three (auth.Unbounded), so on the day one reads this queue the old shape would
// have served it every archived pair in the installation.
//
// The liveness term is not part of the scope clause and cannot be folded into
// it. auth.EnsureVisibleLive says why in its own words: erasure anonymizes a
// person in place and stamps archived_at while LEAVING owner_id alone, so a
// scope predicate answers "yes, still yours" for a record every live read path
// refuses. The same is true of a plain archive.
//
// Without it a decision outlives both records it is about. The candidate carries
// its own archived_at and nothing sweeps it when a SUBJECT is archived, so the
// pair keeps the confidence it was filed with and holds its rank in a lane that
// serves ten by score. Archiving one of two duplicates is a reasonable way to
// resolve a pair — the most natural one for a company entered twice, since it
// needs no merge decision — so the more diligently a workspace resolves them
// that way, the faster its queue fills with its own finished work.
func bothSidesServable(entityType, alias, leftColumn, rightColumn, scopeClause string) string {
	side := func(column string) string {
		terms := fmt.Sprintf("%[1]s.id = dedupe_candidate.%[2]s AND %[1]s.archived_at IS NULL", alias, column)
		if scopeClause != "" {
			terms += " AND " + scopeClause
		}
		return fmt.Sprintf("EXISTS (SELECT 1 FROM %s %s WHERE %s)", entityType, alias, terms)
	}
	return "(" + side(leftColumn) + " AND " + side(rightColumn) + ")"
}

// ListDedupeCandidates pages the queue, confidence-sorted (AC-dedupe-1).
func (s *Store) ListDedupeCandidates(ctx context.Context, in DedupeQueueInput) ([]DedupeCandidateRow, string, error) {
	if err := requireDedupeRead(ctx, in.EntityType); err != nil {
		return nil, "", err
	}
	if in.Status == "" {
		in.Status = dispositionOpen
	}
	if in.Limit <= 0 || in.Limit > 100 {
		in.Limit = dedupeQueueDefaultLimit
	}

	query := `
		SELECT id, entity_type, coalesce(left_person_id, left_org_id, left_lead_id), coalesce(right_person_id, right_org_id, right_lead_id),
		       confidence, evidence, disposition, disposed_by, disposed_at, created_at
		FROM dedupe_candidate
		WHERE disposition = $1 AND archived_at IS NULL`
	args := []any{in.Status}
	visClause, err := dedupeVisibilityClause(ctx, func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		return nil, "", err
	}
	if visClause != "" {
		query += " AND " + visClause
	}
	if in.EntityType != "" {
		args = append(args, in.EntityType)
		query += fmt.Sprintf(" AND entity_type = $%d", len(args))
	}
	if in.Cursor != "" {
		cur, err := decodeDedupeCursor(in.Cursor)
		if err != nil {
			return nil, "", err
		}
		args = append(args, cur.Confidence, cur.ID)
		query += fmt.Sprintf(" AND (confidence, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, in.Limit+1)
	query += fmt.Sprintf(" ORDER BY confidence DESC, id DESC LIMIT $%d", len(args))

	var rows []DedupeCandidateRow
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		res, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer res.Close()
		for res.Next() {
			var r DedupeCandidateRow
			if err := res.Scan(&r.ID, &r.EntityType, &r.LeftID, &r.RightID, &r.Confidence,
				&r.Evidence, &r.Disposition, &r.DisposedBy, &r.DisposedAt, &r.CreatedAt); err != nil {
				return err
			}
			rows = append(rows, r)
		}
		return res.Err()
	})
	if err != nil {
		return nil, "", fmt.Errorf("people: listing dedupe candidates: %w", err)
	}
	next := ""
	if len(rows) > in.Limit {
		rows = rows[:in.Limit]
		last := rows[len(rows)-1]
		next = encodeDedupeCursor(dedupeCursor{Confidence: last.Confidence, ID: last.ID})
	}
	return rows, next, nil
}

// CountOpenDedupeCandidates is how many open pairs THIS caller can see.
//
// It exists because a count is a read. The digest reported one workspace-wide
// number to every reader, which tells an own-scope rep how many duplicate
// pairs exist among records they may not open — the existence-hiding rule the
// queue itself keeps by requiring both sides visible. Sharing
// dedupeVisibilityClause rather than restating the predicate is what keeps the
// badge and the list answering the same question: a count that disagreed with
// the page under it would send a reader looking for rows that were never
// theirs.
func (s *Store) CountOpenDedupeCandidates(ctx context.Context) (int, error) {
	if err := requireDedupeRead(ctx, ""); err != nil {
		return 0, err
	}
	query := `SELECT count(*) FROM dedupe_candidate WHERE disposition = $1 AND archived_at IS NULL`
	args := []any{dispositionOpen}
	visClause, err := dedupeVisibilityClause(ctx, func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		return 0, err
	}
	if visClause != "" {
		query += " AND " + visClause
	}
	var open int
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(&open)
	}); err != nil {
		return 0, fmt.Errorf("people: counting open dedupe candidates: %w", err)
	}
	return open, nil
}

// GetDedupeCandidate reads one row with its full evidence. Both sides of
// the pair must pass the caller's row scope — the evidence names them
// both, so a pair with an out-of-scope side reads as absent (404,
// existence-hiding).
func (s *Store) GetDedupeCandidate(ctx context.Context, id ids.UUID) (DedupeCandidateRow, error) {
	var row DedupeCandidateRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		row, err = readDedupeCandidate(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := auth.EnsureVisible(ctx, tx, row.EntityType, row.LeftID); err != nil {
			return err
		}
		return auth.EnsureVisible(ctx, tx, row.EntityType, row.RightID)
	})
	if err != nil {
		return DedupeCandidateRow{}, err
	}
	if err := requireDedupeRead(ctx, row.EntityType); err != nil {
		return DedupeCandidateRow{}, err
	}
	return row, nil
}

func readDedupeCandidate(ctx context.Context, tx pgx.Tx, id ids.UUID) (DedupeCandidateRow, error) {
	var r DedupeCandidateRow
	err := tx.QueryRow(ctx, `
		SELECT id, entity_type, coalesce(left_person_id, left_org_id, left_lead_id), coalesce(right_person_id, right_org_id, right_lead_id),
		       confidence, evidence, disposition, disposed_by, disposed_at, created_at
		FROM dedupe_candidate WHERE id = $1 AND archived_at IS NULL`, id).
		Scan(&r.ID, &r.EntityType, &r.LeftID, &r.RightID, &r.Confidence,
			&r.Evidence, &r.Disposition, &r.DisposedBy, &r.DisposedAt, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, apperrors.ErrNotFound
	}
	if err != nil {
		return r, fmt.Errorf("people: reading dedupe candidate: %w", err)
	}
	return r, nil
}

// OpenCandidatesNaming answers the open review-queue pairs that name one
// record, newest-strongest first.
//
// IT IS THE NARROW READ BEHIND A WRITE'S OWN ANSWER. ListDedupeCandidates
// serves the queue a human works — paged, filtered, cursored. This serves one
// question a create asks about itself immediately after committing: "did I just
// get filed against something?" A caller reaching for the paged list to answer
// that would have to walk pages looking for its own id.
//
// The permission check and the visibility clause are the LIST's own, called
// here rather than restated: a pair surfaces only when both sides are visible
// to the caller, because the evidence snapshot quotes both records, so naming a
// pair is a read of them. Two spellings of that rule would eventually disagree,
// and the one that drifted would be the one leaking a record's existence.
//
// The bound is deliberate and small. A create that collided with more than a
// handful of records has told the caller everything it usefully can; the point
// is "a human was asked", not an exhaustive listing.
func (s *Store) OpenCandidatesNaming(ctx context.Context, entityType string, id ids.UUID) ([]DedupeCandidateRow, error) {
	if err := requireDedupeRead(ctx, entityType); err != nil {
		return nil, err
	}
	column, ok := map[string][2]string{
		entityPerson:       {"left_person_id", "right_person_id"},
		entityOrganization: {"left_org_id", "right_org_id"},
		entityLead:         {"left_lead_id", "right_lead_id"},
	}[entityType]
	if !ok {
		// Not an error: most record types have no dedupe queue at all, and a
		// create of one is entitled to ask and be told nothing was filed.
		return nil, nil
	}
	args := []any{dispositionOpen, id}
	query := fmt.Sprintf(`
		SELECT id, entity_type, coalesce(left_person_id, left_org_id, left_lead_id),
		       coalesce(right_person_id, right_org_id, right_lead_id),
		       confidence, evidence, disposition, disposed_by, disposed_at, created_at
		  FROM dedupe_candidate
		 WHERE disposition = $1 AND archived_at IS NULL
		   AND (%s = $2 OR %s = $2)`, column[0], column[1])
	visClause, err := dedupeVisibilityClause(ctx, func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		return nil, err
	}
	if visClause != "" {
		query += " AND " + visClause
	}
	args = append(args, openCandidatesNamingLimit)
	query += fmt.Sprintf(" ORDER BY confidence DESC, id DESC LIMIT $%d", len(args))

	var rows []DedupeCandidateRow
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		res, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer res.Close()
		for res.Next() {
			var r DedupeCandidateRow
			if err := res.Scan(&r.ID, &r.EntityType, &r.LeftID, &r.RightID, &r.Confidence,
				&r.Evidence, &r.Disposition, &r.DisposedBy, &r.DisposedAt, &r.CreatedAt); err != nil {
				return err
			}
			rows = append(rows, r)
		}
		return res.Err()
	}); err != nil {
		return nil, fmt.Errorf("people: listing the candidates naming %s: %w", id, err)
	}
	return rows, nil
}

// openCandidatesNamingLimit bounds the answer above. A create that met more
// records than this has already made its point.
const openCandidatesNamingLimit = 5
