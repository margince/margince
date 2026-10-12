// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// Tag suggestions: a suggestible tag proposed on a contact or company because
// captured mail or a meeting note matched the tag's description. Compose reads
// the evidence and decides; this file is the writer that holds what a
// suggestion may be. Nothing here applies a tag: a user's Accept does.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// tagSuggestionEntity names a suggestion in its audit rows.
const tagSuggestionEntity = "tag_suggestion"

// The suggestion lifecycle. Only open is ever shown to a reader.
const (
	TagSuggestionOpen       = "open"
	TagSuggestionAccepted   = "accepted"
	TagSuggestionDismissed  = "dismissed"
	TagSuggestionSuperseded = "superseded"
)

// tagSuggestionColumn is the record column a suggestion on each record type
// fills. A closed map of literals, so a column name is never built from input.
var tagSuggestionColumn = map[string]string{"contact": "contact_id", "company": companyIDField}

// TagSuggestionEvidence is one cited activity.
type TagSuggestionEvidence struct {
	ActivityID ids.UUID
	OccurredAt time.Time
}

// TagSuggestionDraft is what the scout proposes: one tag on one record.
type TagSuggestionDraft struct {
	TagID      ids.TagID
	EntityType string
	EntityID   ids.UUID
	Evidence   []TagSuggestionEvidence
}

// ErrTagSuggestionDraftInvalid refuses a draft that breaks the suggestion's own
// shape. The scout builds drafts, so this is a defect in the scout, not input.
var ErrTagSuggestionDraftInvalid = errors.New("collections: the tag suggestion draft is malformed")

func validTagSuggestionDraft(d TagSuggestionDraft) error {
	if d.TagID.IsZero() || d.EntityID.IsZero() || tagSuggestionColumn[d.EntityType] == "" || len(d.Evidence) == 0 {
		return ErrTagSuggestionDraftInvalid
	}
	for _, e := range d.Evidence {
		if e.ActivityID.IsZero() || e.OccurredAt.IsZero() {
			return ErrTagSuggestionDraftInvalid
		}
	}
	return nil
}

// TagSuggestionFloorExpr is the instant evidence must be newer than for this
// tag on this record: the latest decision on an earlier suggestion. A user
// already judged the older evidence, so only new evidence re-asks. NULL while
// nothing was decided. tag and record are SQL expressions.
func TagSuggestionFloorExpr(entityType, tag, record string) string {
	return `(SELECT max(fs.decided_at) FROM tag_suggestion fs
	  WHERE fs.tag_id = ` + tag + ` AND fs.` + tagSuggestionColumn[entityType] + ` = ` + record + `
	    AND fs.state IN ('accepted', 'dismissed'))`
}

// SuggestableTagClause says the tag may be proposed on the record. The tag is
// live and suggestible. The record is live and does not carry it. No suggestion
// of it is open there. The scout and the writer both ask it.
func SuggestableTagClause(entityType, tag, record string) string {
	column := tagSuggestionColumn[entityType]
	return `(EXISTS (SELECT 1 FROM tag st WHERE st.id = ` + tag + `
	          AND st.archived_at IS NULL AND st.suggestible)
	  AND EXISTS (SELECT 1 FROM ` + entityType + ` sr WHERE sr.id = ` + record + ` AND sr.archived_at IS NULL)
	  AND NOT EXISTS (SELECT 1 FROM taggable stg WHERE stg.tag_id = ` + tag + `
	          AND stg.entity_type = '` + entityType + `' AND stg.entity_id = ` + record + `)
	  AND NOT EXISTS (SELECT 1 FROM tag_suggestion so WHERE so.tag_id = ` + tag + `
	          AND so.` + column + ` = ` + record + ` AND so.state = 'open'))`
}

// RecordTagSuggestionTx writes one open suggestion with its evidence, and
// reports whether it was written. System-only: a seat able to plant one could
// put a card in every colleague's Worklist.
//
// It writes nothing, without error, when SuggestableTagClause refuses the pair.
// It also writes nothing when any evidence is not newer than the last decision
// on this tag and record.
func RecordTagSuggestionTx(ctx context.Context, tx pgx.Tx, d TagSuggestionDraft) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	if err := validTagSuggestionDraft(d); err != nil {
		return false, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	column := tagSuggestionColumn[d.EntityType]
	var floor *time.Time
	if err := tx.QueryRow(ctx, `SELECT `+TagSuggestionFloorExpr(d.EntityType, "$1::uuid", "$2::uuid"),
		d.TagID, d.EntityID).Scan(&floor); err != nil {
		return false, fmt.Errorf("collections: reading the tag suggestion floor: %w", err)
	}
	through := d.Evidence[0].OccurredAt
	for _, e := range d.Evidence {
		if floor != nil && !e.OccurredAt.After(*floor) {
			return false, nil
		}
		if e.OccurredAt.After(through) {
			through = e.OccurredAt
		}
	}
	var id ids.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO tag_suggestion (tag_id, `+column+`, evidence_count, evidence_through, captured_by)
		SELECT $1::uuid, $2::uuid, $3::integer, $4::timestamptz, $5::text
		 WHERE `+SuggestableTagClause(d.EntityType, "$1::uuid", "$2::uuid")+`
		ON CONFLICT DO NOTHING
		RETURNING id`,
		d.TagID, d.EntityID, len(d.Evidence), through, by).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("collections: recording a tag suggestion: %w", err)
	}
	for _, e := range d.Evidence {
		if _, err := tx.Exec(ctx, `
			INSERT INTO tag_suggestion_evidence (suggestion_id, activity_id, occurred_at)
			VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, id, e.ActivityID, e.OccurredAt); err != nil {
			return false, fmt.Errorf("collections: recording tag suggestion evidence: %w", err)
		}
	}
	// The image names neither the tag nor the record. The compliance log crosses
	// activity audiences, and the pair would tell its reader what owner-only
	// mail said about somebody.
	auditID, err := storekit.AuditEvent(ctx, tx, "create", tagSuggestionEntity, id,
		map[string]any{"evidence_count": len(d.Evidence)})
	if err != nil {
		return false, fmt.Errorf("collections: auditing a tag suggestion: %w", err)
	}
	if err := storekit.EmitPipelinePayload(ctx, tx, auditID, crmcontracts.InternalEventTagSuggestionCreated{
		EvidenceCount: len(d.Evidence),
	}); err != nil {
		return false, fmt.Errorf("collections: emitting tag_suggestion.created: %w", err)
	}
	return true, nil
}

// SupersedeStaleTagSuggestionsTx retires every open suggestion that no longer
// stands (tagSuggestionStandsClause), and reports how many. A suggestion
// nobody can see would otherwise block its tag and record for good.
// System-only, for the same reason recording is.
func SupersedeStaleTagSuggestionsTx(ctx context.Context, tx pgx.Tx) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `
		UPDATE tag_suggestion s SET state = 'superseded', decided_at = now()
		 WHERE s.state = 'open' AND NOT `+tagSuggestionStandsClause("s")+`
		RETURNING s.id`)
	if err != nil {
		return 0, fmt.Errorf("collections: superseding tag suggestions: %w", err)
	}
	retired, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, fmt.Errorf("collections: reading superseded tag suggestions: %w", err)
	}
	for _, id := range retired {
		if err := recordTagSuggestionState(ctx, tx, id, TagSuggestionSuperseded); err != nil {
			return 0, err
		}
	}
	return len(retired), nil
}

// columnState is the audited name of the lifecycle column.
const columnState = "state"

// tagSuggestionEvents are the internal events a move out of open emits.
var tagSuggestionEvents = map[string]events.Payload{
	TagSuggestionAccepted:   crmcontracts.InternalEventTagSuggestionAccepted{},
	TagSuggestionDismissed:  crmcontracts.InternalEventTagSuggestionDismissed{},
	TagSuggestionSuperseded: crmcontracts.InternalEventTagSuggestionSuperseded{},
}

// recordTagSuggestionState writes the audit row and the event for one
// suggestion leaving open, in the caller's transaction.
func recordTagSuggestionState(ctx context.Context, tx pgx.Tx, id ids.UUID, state string) error {
	auditID, err := storekit.Audit(ctx, tx, "update", tagSuggestionEntity, id,
		map[string]any{columnState: TagSuggestionOpen}, map[string]any{columnState: state})
	if err != nil {
		return fmt.Errorf("collections: auditing a %s tag suggestion: %w", state, err)
	}
	if err := storekit.EmitPipelinePayload(ctx, tx, auditID, tagSuggestionEvents[state]); err != nil {
		return fmt.Errorf("collections: emitting tag_suggestion.%s: %w", state, err)
	}
	return nil
}

// tagSuggestionStandsClause is a suggestion that still means something for
// every reader. Its tag is live and suggestible. Its record is live and does
// not carry the tag. Every evidence row it was written with is still present on
// a live activity. alias names tag_suggestion in the outer query.
func tagSuggestionStandsClause(alias string) string {
	return fmt.Sprintf(`(EXISTS (SELECT 1 FROM tag tt WHERE tt.id = %[1]s.tag_id
	          AND tt.archived_at IS NULL AND tt.suggestible)
	  AND (%[1]s.contact_id IS NULL OR EXISTS (SELECT 1 FROM contact tc
	          WHERE tc.id = %[1]s.contact_id AND tc.archived_at IS NULL
	            AND NOT EXISTS (SELECT 1 FROM taggable tg WHERE tg.tag_id = %[1]s.tag_id
	                  AND tg.entity_type = 'contact' AND tg.entity_id = tc.id)))
	  AND (%[1]s.company_id IS NULL OR EXISTS (SELECT 1 FROM company tco
	          WHERE tco.id = %[1]s.company_id AND tco.archived_at IS NULL
	            AND NOT EXISTS (SELECT 1 FROM taggable tg WHERE tg.tag_id = %[1]s.tag_id
	                  AND tg.entity_type = 'company' AND tg.entity_id = tco.id)))
	  AND (SELECT count(*) FROM tag_suggestion_evidence te
	        JOIN activity ta ON ta.id = te.activity_id AND ta.archived_at IS NULL AND %[2]s
	       WHERE te.suggestion_id = %[1]s.id) = %[1]s.evidence_count)`, alias, auth.ActivityAvailableClause("ta"))
}
