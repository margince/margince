// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The reads, guards and patch shaping behind exitcriteria.go's four entry
// points, kept beside them rather than inside them so each entry point reads
// as the transaction it commits.

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	maxCriterionLabel = 120
	maxCriterionKey   = 64
)

const criterionKeyField = "key"

// criterionRequiredColumn is the requiredness column's one spelling, shared by
// the patch and the audit image so the two name the same field.
const criterionRequiredColumn = "required"

// criterionPositionColumn is position's one spelling, shared by the patch and
// the audit image. The SELECT and INSERT text quotes it because it reads as a
// reserved word there; an audit key never carries those quotes.
const criterionPositionColumn = "position"

// criterionKeyShape mirrors stage_exit_criterion_key_shape in the migration.
// Two spellings of one rule, so the refusal names the field instead of the
// constraint; TestACriterionKeyRefusalMatchesTheColumnCheck holds them together.
var criterionKeyShape = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// lockCriteriaList takes the STAGE row for update, which is the one
// serialization point for reshaping that stage's criteria list.
//
// It exists for the same reason lockStageConfig does one level up: the add
// computes a tail position from a count, and the archive renumbers the rows
// above the hole. Two of those running concurrently would either duplicate a
// position or leave a gap, and the unique index does not cover position. One
// row taken first by all three writers removes the race rather than
// diagnosing it afterwards.
func lockCriteriaList(ctx context.Context, tx pgx.Tx, stageID ids.StageID) (storekit.RowLock, error) {
	return storekit.LockRow(ctx, tx, "stage", stageID.UUID, storekit.LiveOnly)
}

// stageSemanticOf answers a live stage's semantic, or ErrNotFound.
func stageSemanticOf(ctx context.Context, tx pgx.Tx, stageID ids.StageID) (StageSemantic, error) {
	var semantic string
	err := tx.QueryRow(ctx,
		`SELECT semantic FROM stage WHERE id = $1 AND archived_at IS NULL`, stageID).Scan(&semantic)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperrors.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read the stage's semantic: %w", err)
	}
	return StageSemantic(semantic), nil
}

// refuseCriterionOnTerminalStage holds the rule the DDL cannot see: won and
// lost are where a deal STOPS, so "what it takes to leave" has no referent
// there. A CHECK on the criterion row cannot read the stage row, so the
// obligation lives here and TestATerminalStageCarriesNoExitCriteria holds it.
func refuseCriterionOnTerminalStage(ctx context.Context, tx pgx.Tx, stageID ids.StageID) error {
	semantic, err := stageSemanticOf(ctx, tx, stageID)
	if err != nil {
		return err
	}
	if semantic.Terminal() {
		return &values.ParseError{
			Field: stageSemanticField, Code: codeTerminalStageNoCriteria,
			Message: "a won or lost stage is where a deal stops, so it requires nothing to leave it",
		}
	}
	return nil
}

// refuseTakenCriterionKey answers the unique index in the caller's own
// vocabulary rather than letting a 23505 surface as a 500. The index is
// partial, so only a LIVE row takes a key: archiving one frees it again.
func refuseTakenCriterionKey(ctx context.Context, tx pgx.Tx, stageID ids.StageID, key string) error {
	var taken bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM stage_exit_criterion
			 WHERE stage_id = $1 AND key = $2 AND archived_at IS NULL)`,
		stageID, key).Scan(&taken); err != nil {
		return fmt.Errorf("check the criterion key: %w", err)
	}
	if taken {
		return &values.ParseError{
			Field: criterionKeyField, Code: codeCriterionKeyTaken,
			Message: "this stage already asks for a criterion under that key",
		}
	}
	return nil
}

// nextCriterionPosition answers the end of the stage's live list.
//
// A criterion always appends. The caller cannot choose a slot: honouring one
// would let two criteria share a position, and the list would then order
// arbitrarily between them. The archive's renumbering is what keeps the run
// contiguous from the other end.
func nextCriterionPosition(
	ctx context.Context, tx pgx.Tx, stageID ids.StageID,
) (int, error) {
	var next int
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(max("position") + 1, 0) FROM stage_exit_criterion
		 WHERE stage_id = $1 AND archived_at IS NULL`, stageID).Scan(&next); err != nil {
		return 0, fmt.Errorf("find the end of the stage's criteria: %w", err)
	}
	return next, nil
}

// readCriteria answers a stage's criteria in position order.
func readCriteria(
	ctx context.Context, tx pgx.Tx, stageID ids.StageID, archived storekit.ArchivedFilter,
) ([]crmcontracts.StageExitCriterion, error) {
	q := `SELECT ` + criterionColumns + ` FROM stage_exit_criterion WHERE stage_id = $1`
	if archived == storekit.LiveOnly {
		q += liveRowsClause
	}
	q += ` ORDER BY "position", created_at`
	rows, err := tx.Query(ctx, q, stageID)
	if err != nil {
		return nil, fmt.Errorf("list exit criteria: %w", err)
	}
	defer rows.Close()
	out := []crmcontracts.StageExitCriterion{}
	for rows.Next() {
		c, err := scanCriterion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read exit criteria: %w", err)
	}
	return out, nil
}

// readCriterion answers one criterion by id.
func readCriterion(
	ctx context.Context, tx pgx.Tx, id ids.ExitCriterionID, archived storekit.ArchivedFilter,
) (crmcontracts.StageExitCriterion, error) {
	q := `SELECT ` + criterionColumns + ` FROM stage_exit_criterion WHERE id = $1`
	if archived == storekit.LiveOnly {
		q += liveRowsClause
	}
	c, err := scanCriterion(tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.StageExitCriterion{}, apperrors.ErrNotFound
	}
	return c, err
}

// scannable is the narrow interface tx.QueryRow's pgx.Row and a pgx.Rows
// cursor both satisfy, which is what lets scanCriterion serve the list and
// the single read from one body.
type scannable interface {
	Scan(dest ...any) error
}

func scanCriterion(row scannable) (crmcontracts.StageExitCriterion, error) {
	var out crmcontracts.StageExitCriterion
	var id, stageID ids.UUID
	var kind string
	var version int64
	if err := row.Scan(&id, &stageID, &out.Key, &out.Label, &kind, &out.Required,
		&out.Hint, &out.Position, &version, &out.CreatedAt, &out.UpdatedAt,
		&out.ArchivedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
		return out, fmt.Errorf("scan exit criterion: %w", err)
	}
	out.Id = openapi_types.UUID(id)
	out.StageId = openapi_types.UUID(stageID)
	out.Kind = crmcontracts.StageCriterionKind(kind)
	out.Version = &version
	return out, nil
}

// criterionVersion answers the row's optimistic-concurrency version.
func criterionVersion(c crmcontracts.StageExitCriterion) int64 {
	if c.Version == nil {
		return 0
	}
	return *c.Version
}

func openapiUUID(u ids.UUID) openapi_types.UUID { return openapi_types.UUID(u) }

// criterionUpdatePatch names only the columns this edit actually moves, so
// an update naming nothing writes nothing and audits nothing.
func criterionUpdatePatch(
	current crmcontracts.StageExitCriterion, in UpdateCriterionInput,
) *storekit.Patch {
	p := storekit.NewPatch()
	if in.Label != nil {
		p.Set("label", current.Label, *in.Label)
	}
	if in.Kind != nil {
		p.Set("kind", string(current.Kind), *in.Kind)
	}
	if in.Required != nil {
		p.Set(criterionRequiredColumn, current.Required, *in.Required)
	}
	// SetHint distinguishes "leave the hint alone" from "clear it": a nil
	// Hint with SetHint false is an absent field, with SetHint true it is an
	// explicit null the caller sent.
	//
	// The clear passes an untyped nil and the set passes the value, rather
	// than the pointer either way. Both a typed and an untyped nil marshal to
	// JSON null, so the audit image reads the same — but a *string in the map
	// is a pointer every later reader of these images has to dereference, and
	// the images beside it hold plain values.
	if in.SetHint {
		if in.Hint == nil {
			p.Set("hint", current.Hint, nil)
		} else {
			p.Set("hint", current.Hint, *in.Hint)
		}
	}
	return p
}

// criterionAfter is the audit after-image of a whole criterion row — used by
// create and archive, which have no column-level patch to derive one from.
func criterionAfter(c crmcontracts.StageExitCriterion) map[string]any {
	return map[string]any{
		"stage_id": c.StageId.String(), "key": c.Key, "label": c.Label,
		"kind": string(c.Kind), criterionRequiredColumn: c.Required,
		criterionPositionColumn: c.Position, "hint": c.Hint,
	}
}

// validCriterionKey mirrors the column's shape CHECK so a bad key is a 422
// naming the field rather than a 500 from the constraint.
func validCriterionKey(key string) error {
	if !criterionKeyShape.MatchString(key) || len(key) > maxCriterionKey {
		return &values.ParseError{
			Field: criterionKeyField, Code: "invalid_criterion_key",
			Message: "key starts with a letter and holds lower-case letters, digits and underscores",
		}
	}
	return nil
}

// validCriterionLabel mirrors the column's length CHECK.
func validCriterionLabel(label string) error {
	// RUNES, not bytes: the CHECK counts characters (Postgres length() does),
	// so len() would refuse text the column accepts.
	if n := len([]rune(label)); n == 0 || n > maxCriterionLabel {
		return &values.ParseError{
			Field: "label", Code: "invalid_criterion_label",
			Message: fmt.Sprintf("label says what the criterion is, in 1 to %d characters", maxCriterionLabel),
		}
	}
	return nil
}

// refuseTerminalWithCriteria holds the other half of the terminal rule.
//
// refuseCriterionOnTerminalStage stops a criterion reaching a stage that is
// ALREADY won or lost. This stops the stage becoming terminal while it still
// carries criteria — the same invariant approached from the other side, and
// without it the contract's promise that a terminal stage lists none is false
// for every stage that was open when its criteria were written.
//
// The admin's way forward is stated: archive the criteria, then close the
// stage. Silently archiving them here would discard configuration on a write
// that never mentioned it.
func refuseTerminalWithCriteria(
	ctx context.Context, tx pgx.Tx, stageID ids.StageID, currentSemantic string, wanted *string,
) error {
	if wanted == nil || !StageSemantic(*wanted).Terminal() || StageSemantic(currentSemantic).Terminal() {
		return nil
	}
	var live int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM stage_exit_criterion
		 WHERE stage_id = $1 AND archived_at IS NULL`, stageID).Scan(&live); err != nil {
		return fmt.Errorf("count the stage's exit criteria: %w", err)
	}
	if live == 0 {
		return nil
	}
	return &values.ParseError{
		Field: stageSemanticField, Code: codeTerminalStageNoCriteria,
		Message: "this stage still asks for exit criteria; remove them before closing it as won or lost",
	}
}

// requireStage answers nil when the stage exists, ErrNotFound otherwise.
//
// LiveOnly is the write path's question — a criterion may only be added to a
// stage still in use. IncludeArchived is the read path's: an archived stage's
// criteria are still readable, because the evidence citing them is.
func requireStage(
	ctx context.Context, tx pgx.Tx, stageID ids.StageID, archived storekit.ArchivedFilter,
) error {
	q := `SELECT EXISTS (SELECT 1 FROM stage WHERE id = $1`
	if archived == storekit.LiveOnly {
		q += liveRowsClause
	}
	q += `)`
	var exists bool
	if err := tx.QueryRow(ctx, q, stageID).Scan(&exists); err != nil {
		return fmt.Errorf("resolve the stage: %w", err)
	}
	if !exists {
		return apperrors.ErrNotFound
	}
	return nil
}
