// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Wiring the reversal: the evaluator's live-state readers, and the seam the
// privacy module receives as a port. A module never imports compose, so the
// direction is compose reaching down — the update path lives in six modules
// the history surface may not reach, and this is the one place that knows both.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// NewRestoreSeam assembles the reversal executor over the installation pool and
// the update dispatcher, with the evaluator's ports bound to the real readers.
//
// corrections is nil-safe: an installation wired without it simply offers no
// correction-aware reversal, every row falling through to the generic
// evaluator exactly as before this seam knew corrections existed.
func NewRestoreSeam(pool *pgxpool.Pool, dispatcher *Dispatcher, corrections *deals.Store) RestoreSeam {
	// The edge's rules are the people module's, and so is its table. This seam
	// reaches them through that module's own store rather than restating any of
	// them, which is also why it owns no relationship SQL.
	edges := people.NewStore(InstallationDB(pool))
	return RestoreSeam{
		pool:        pool,
		dispatcher:  dispatcher,
		visible:     recordIsVisibleToCaller,
		edges:       edges,
		corrections: corrections,
		evaluator: Evaluator{
			Archived:      recordIsArchived,
			Writable:      recordIsWritableByCaller,
			BehindErasure: rowIsBehindTheErasureBoundary,
			AlreadyUndone: rowIsAlreadyUndone,
			Unwritable:    valuesNoLongerWritable,
			EdgeFacts:     edges.EdgeFactsForReverse,
			EdgeWritable:  edgeIsWritableByCaller(edges),
			ExternallyGoverned: func(ctx context.Context) (bool, error) {
				return dispatcher.isOverlayUncached(ctx)
			},
		},
	}
}

// recordIsArchived reads the record's own archived_at. The table name is the
// record type, which servesRecordType has already closed to the six — no
// identifier ever reaches this statement from a request body.
func recordIsArchived(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID) (bool, error) {
	if !servesRecordType(entityType) {
		return false, fmt.Errorf("compose: undoability: %q is not a record type this path reads", entityType)
	}
	var archived bool
	err := tx.QueryRow(ctx,
		`SELECT archived_at IS NOT NULL FROM `+pgx.Identifier{entityType}.Sanitize()+` WHERE id = $1`,
		id).Scan(&archived)
	if err != nil {
		return false, err
	}
	return archived, nil
}

// recordVersionUnmoved refuses a decision taken from a screen the record has
// moved under. It is the route's REQUIRED If-Match, asked of the record whose
// history was open — the record path gets the same guard from the update it
// sends, and the edge path has to ask it here because its write lands on the
// link and never on the record.
//
// The table name is the record type, which servesRecordType has already closed
// to the six: no identifier reaches this statement from a request body.
func recordVersionUnmoved(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID, ifVersion int64) error {
	if !servesRecordType(entityType) {
		return fmt.Errorf("compose: undoability: %q is not a record type this path reads", entityType)
	}
	var version int64
	err := tx.QueryRow(ctx,
		`SELECT version FROM `+pgx.Identifier{entityType}.Sanitize()+` WHERE id = $1`,
		id).Scan(&version)
	if err != nil {
		return err
	}
	if version != ifVersion {
		return apperrors.ErrVersionSkew
	}
	return nil
}

// recordIsVisibleToCaller is the row-scope gate every read of this record
// takes, dispatching for activity exactly as the history read does. The
// reversal path takes it before it reads the audit row at all: anything that
// returns a record is a read, and a refusal that distinguishes a hidden record
// from an absent one has disclosed the record.
func recordIsVisibleToCaller(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID) error {
	if entityType == entityTypeActivity {
		return auth.EnsureActivityContentVisible(ctx, tx, id)
	}
	return auth.EnsureVisible(ctx, tx, entityType, id)
}

// recordIsWritableByCaller asks the same gate the record's own update path will
// ask. Asking it here is what makes the button honest instead of a 403 waiting
// to happen, and asking the SAME gate is what keeps the two answers from
// diverging.
//
// activity dispatches differently, exactly as the history read does: it carries
// no owner_id, so its write authority rides the link walk rather than the
// owner column. Reaching it through the ordinary gate refuses every activity,
// which is a button that says the caller may not edit a record they can.
func recordIsWritableByCaller(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID) error {
	if entityType == entityTypeActivity {
		return auth.EnsureActivityWritable(ctx, tx, id)
	}
	return auth.EnsureWritable(ctx, tx, entityType, id)
}

// edgeIsWritableByCaller asks both halves of an edge write's authority: the
// OBJECT grants the people store asks at its own entry, and the ROW scope on the
// ANCHOR the edge annotates.
//
// The anchor and not the record whose history was open, and the two are not
// symmetric: an employment anchors the PERSON, so a seat holding
// organization-write and not person-write is refused the button on the company's
// page. Asking the record instead would light a button the write then refuses.
//
// The entry's action travels with it because the object grant the inverse asks
// for is the people store's own to decide — reversing a create is an archive
// there, and the archive asks delete.
func edgeIsWritableByCaller(edges *people.Store) func(context.Context, pgx.Tx, people.EdgeFacts, string) error {
	return func(ctx context.Context, tx pgx.Tx, facts people.EdgeFacts, entryAction string) error {
		if err := edges.RefuseEdgeWrite(ctx, facts.Kind, entryAction); err != nil {
			return err
		}
		return recordIsWritableByCaller(ctx, tx, facts.Anchor, facts.AnchorID)
	}
}

// entityTypeActivity is the record kind whose row-scope checks dispatch
// differently, named rather than typed inline at the branch above.
const entityTypeActivity = "activity"

// entityTypeDeal is the record kind a machine correction is always about —
// deals.DealCorrection has no other subject today, so reverseCorrection
// filters an audit row on it before asking the corrections store anything.
const entityTypeDeal = "deal"

// rowIsBehindTheErasureBoundary reuses privacy's own boundary predicate rather
// than restating it. An Art. 17 erasure is one of the few rules where a second
// spelling that is merely ALMOST the same would resurrect what was certified
// destroyed.
//
// A LINK's row takes privacy's edge predicate instead of this one, and the
// difference is not a refinement. The boundary is keyed on the row's own
// (entity_type, entity_id), which for a link is ('relationship', edge_id) — an
// identity no write path in this tree ever records a scrub verb against. Asked
// here, every link that has ever existed answers "never erased", so the branch
// would be nominal and the refusal unreachable. What bounds a link's image is
// the erasure of the records it joins.
func rowIsBehindTheErasureBoundary(ctx context.Context, tx pgx.Tx, row AuditRow) (bool, error) {
	if row.EntityType == edgeEntityType {
		return privacy.EdgeBehindErasureBoundary(ctx, tx, row.ID)
	}
	// The placeholder is derived from the argument slice rather than typed:
	// the verb list reaches privacy's predicate as a POSITION, and nothing
	// checks that a hand-written one still matches the arguments beside it.
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	idPlaceholder := arg(row.ID)
	verbsPlaceholder := arg(privacy.ScrubVerbs())

	var readable bool
	err := tx.QueryRow(ctx, `
		SELECT `+privacy.UnscrubbedImageSQL("a", verbsPlaceholder)+`
		FROM audit_log a WHERE a.id = `+idPlaceholder,
		args...).Scan(&readable)
	if err != nil {
		return false, err
	}
	return !readable, nil
}

// rowIsAlreadyUndone reports whether a LIVE restore reverses this row: one that
// has not itself been reversed. It is not a terminal state — reversing that
// restore reopens this entry, which is what makes the trail navigable in both
// directions rather than a one-way ratchet.
//
// Scoped to the record's own rows, which idx_audit_entity already serves. There
// is no index on evidence, and a partial expression index is available if
// measurement asks for one; it is not added speculatively.
func rowIsAlreadyUndone(ctx context.Context, tx pgx.Tx, row AuditRow) (bool, error) {
	var undone bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM audit_log undo
		  WHERE undo.entity_type = $1 AND undo.entity_id = $2
		    AND undo.evidence ->> $3 = $4::text
		    AND NOT EXISTS (
		      SELECT 1 FROM audit_log reundo
		      WHERE reundo.entity_type = undo.entity_type
		        AND reundo.entity_id = undo.entity_id
		        AND reundo.evidence ->> $3 = undo.id::text))`,
		row.EntityType, row.EntityID, privacy.UndidAuditLogID, row.ID).Scan(&undone)
	if err != nil {
		return false, err
	}
	return undone, nil
}

// valuesNoLongerWritable names patch keys the update path could not write
// today. A cf_* key whose catalog entry was retired is the case that reaches a
// person as "unknown field cf_budget", which is not an answer to pressing Undo.
func valuesNoLongerWritable(ctx context.Context, tx pgx.Tx, entityType string, _ ids.UUID, patch map[string]json.RawMessage) ([]string, error) {
	var custom []string
	for key := range patch {
		if len(key) > 3 && key[:3] == "cf_" {
			custom = append(custom, key)
		}
	}
	if len(custom) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT k.key FROM unnest($2::text[]) AS k(key)
		WHERE NOT EXISTS (
		  SELECT 1 FROM custom_field cf
		  WHERE cf.object = $1 AND cf.column_name = k.key AND cf.status = 'active')
		ORDER BY 1`, entityType, custom)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var retired []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		retired = append(retired, key)
	}
	return retired, rows.Err()
}

// restoreSeamPort is RestoreSeam as privacy declares it. The assertion is here
// rather than at the assignment so a signature drift fails where the seam is
// defined, naming both sides.
var _ privacy.ChangeRestorer = RestoreSeam{}

// wireReversal gives the history surface both halves of the reversal: the
// executor that puts a change back, and the reader that says in advance which
// changes can be. They share ONE seam, so the button and the write cannot come
// to disagree about what is possible.
//
// It runs AFTER assembly and takes the server's OWN dispatcher rather than
// building one. A second dispatcher is a second per-workspace overlay cache and
// a second overlay meter, so the reversal path would answer "is this workspace
// overlay-governed" from a different reading than every other write the server
// makes — and two answers to that question is what the dispatcher exists to
// prevent.
func (s *Server) wireReversal(pool *pgxpool.Pool) {
	// ONE instance, given to the seam that WRITES a reversal and to the judge
	// that READS whether one is offered — the same reason the seam itself is
	// shared below. Two separately constructed stores would still ask the
	// database the same question, but a future difference between them (a
	// second gate, a second cache) is exactly the kind of drift this line
	// exists to make impossible rather than merely unlikely.
	corrections := deals.NewStore(InstallationDB(pool), DealsInstallation())
	seam := NewRestoreSeam(pool, s.sorDispatch, corrections)
	s.privacyHandlers = s.privacyHandlers.
		WithChangeRestorer(seam).
		WithUndoabilityReader(NewUndoabilityPage(seam))
	// The receipt's undo answer comes from THIS seam, not a second one built
	// for it: the line offering an Undo and the write performing it must agree,
	// and one evaluator is how they stay agreed. It also inherits the server's
	// own dispatcher, so the overlay question is answered once.
	//
	// Assembly order is load-bearing here and stated rather than assumed: the
	// receipt is built before this runs, and a nil service would leave every
	// line reading "not evaluated" with nothing failing to say so.
	if s.magicService == nil {
		panic("compose: the receipt must be assembled before its undo judge is bound")
	}
	s.magicService.WithUndoJudge(magicUndoJudge{
		seam: seam,
		// The corrections store is what lets a machine close-date change read
		// as undoable at all: the generic evaluator refuses exactly those rows,
		// because they write a field the ordinary update shape cannot spell.
		corrections: corrections,
	})
}
