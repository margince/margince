// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Partner stages are the ladder an installation keeps for its partner
// programmes (partner_stage). Each stage names one of six fixed meanings, and
// every rule and filter reads the meaning, so an administrator may rename,
// reorder and add stages without changing what the product does.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	partnerStageTable = "partner_stage"
	// partnerStageObject is what the audit row and the event name the change
	// by; the same word as the table, kept apart as the gate asks.
	partnerStageObject       = "partner_stage"
	partnerStageMeaningField = "meaning"
	partnerStageColumns      = `id, key, label, label_key, meaning, sort_order, active, system, version, created_at, updated_at,
		(SELECT count(*) FROM partner WHERE partner.relationship_stage = partner_stage.key)`
)

// partnerStageKeyShape is partner_stage_key_shape, checked before the insert so
// a bad key is a 422 that names the field.
var partnerStageKeyShape = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// CreatePartnerStageInput is one new stage.
type CreatePartnerStageInput struct {
	Key       string // optional; derived from Label when empty
	Label     string
	Meaning   crmcontracts.PartnerStageMeaning
	SortOrder int
}

// UpdatePartnerStageInput is a sparse patch; nil leaves the column alone. The
// key is absent: it is the value partners carry.
type UpdatePartnerStageInput struct {
	Label     *string
	Meaning   *crmcontracts.PartnerStageMeaning
	SortOrder *int
	Active    *bool
}

// PartnerStageMeaningError refuses a meaning outside the six.
type PartnerStageMeaningError struct{}

func (e *PartnerStageMeaningError) Error() string {
	return "meaning must be one of candidate, signed, ready, active, dormant or ended"
}

// FieldFault names the meaning as the invalid input.
func (e *PartnerStageMeaningError) FieldFault() (field, code, message string) {
	return partnerStageMeaningField, "invalid_value", e.Error()
}

// ListPartnerStages answers the whole ladder, retired stages included, so a
// partner on a retired stage still renders its label. It is read under the
// data-model grant every administered list shares.
func (s *Store) ListPartnerStages(ctx context.Context) (crmcontracts.PartnerStageListResponse, error) {
	if err := auth.Require(ctx, leadVocabularyObject, principal.ActionRead); err != nil {
		return crmcontracts.PartnerStageListResponse{}, err
	}
	out := crmcontracts.PartnerStageListResponse{Data: []crmcontracts.PartnerStage{}}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+partnerStageColumns+` FROM partner_stage ORDER BY sort_order, key`)
		if err != nil {
			return fmt.Errorf("list partner stages: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			stage, err := scanPartnerStage(rows)
			if err != nil {
				return err
			}
			out.Data = append(out.Data, stage)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return partnerCountsForCaller(ctx, tx, out.Data)
	})
	return out, err
}

// CreatePartnerStage adds a stage. A key already in the list is a 409; a blank
// label, a bad key or an unknown meaning is a 422.
func (s *Store) CreatePartnerStage(ctx context.Context, in CreatePartnerStageInput) (crmcontracts.PartnerStage, error) {
	if err := auth.Require(ctx, leadVocabularyObject, principal.ActionCreate); err != nil {
		return crmcontracts.PartnerStage{}, err
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		return crmcontracts.PartnerStage{}, &values.ParseError{Field: leadOriginLabelField, Code: codeRequired, Message: msgLabelRequired}
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		key = deriveSourceKey(label)
	}
	if !partnerStageKeyShape.MatchString(key) {
		return crmcontracts.PartnerStage{}, &values.ParseError{
			Field: leadOriginKeyField, Code: "invalid_key",
			Message: "key must start with a letter and hold only lowercase letters, digits and underscores",
		}
	}
	if !in.Meaning.Valid() {
		return crmcontracts.PartnerStage{}, &PartnerStageMeaningError{}
	}
	var out crmcontracts.PartnerStage
	err := s.tx(ctx, func(tx pgx.Tx) error {
		id := ids.NewV7()
		_, err := tx.Exec(ctx,
			`INSERT INTO partner_stage (id, key, label, meaning, sort_order) VALUES ($1, $2, $3, $4, $5)`,
			id, key, label, string(in.Meaning), in.SortOrder)
		if storekit.IsUniqueViolation(err) {
			return apperrors.ErrConflict
		}
		if err != nil {
			return fmt.Errorf("insert partner stage: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "create", partnerStageObject, id, nil,
			map[string]any{leadOriginKeyField: key, leadOriginLabelField: label, partnerStageMeaningField: string(in.Meaning)})
		if err != nil {
			return err
		}
		if err := announcePartnerStageChange(ctx, tx, auditID, "create", id); err != nil {
			return err
		}
		if out, err = readPartnerStage(ctx, tx, id); err != nil {
			return err
		}
		return partnerCountsForCaller(ctx, tx, []crmcontracts.PartnerStage{out})
	})
	return out, err
}

// UpdatePartnerStage renames, reorders, re-means or retires a stage. The
// meaning is fixed while any partner carries the stage, and the last active
// candidate stage stays: every new programme starts on one.
func (s *Store) UpdatePartnerStage(ctx context.Context, id ids.UUID, in UpdatePartnerStageInput) (crmcontracts.PartnerStage, error) {
	if err := auth.Require(ctx, leadVocabularyObject, principal.ActionUpdate); err != nil {
		return crmcontracts.PartnerStage{}, err
	}
	if in.Label != nil && strings.TrimSpace(*in.Label) == "" {
		return crmcontracts.PartnerStage{}, &values.ParseError{Field: leadOriginLabelField, Code: codeRequired, Message: msgLabelNotEmpty}
	}
	if in.Meaning != nil && !in.Meaning.Valid() {
		return crmcontracts.PartnerStage{}, &PartnerStageMeaningError{}
	}
	var out crmcontracts.PartnerStage
	err := s.tx(ctx, func(tx pgx.Tx) error {
		lock, before, err := lockPartnerStage(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := refuseStageMeaningChange(before, in); err != nil {
			return err
		}
		stillCandidate := (in.Active == nil || *in.Active) &&
			(in.Meaning == nil || *in.Meaning == crmcontracts.PartnerStageMeaningCandidate)
		if !stillCandidate {
			if err := refuseLastCandidateStage(ctx, tx, before); err != nil {
				return err
			}
		}
		p := partnerStagePatch(before, in)
		if !p.Empty() {
			if err := p.ApplyLocked(ctx, tx, lock); err != nil {
				return err
			}
			auditID, err := storekit.Audit(ctx, tx, "update", partnerStageObject, id, p.Before(), p.After())
			if err != nil {
				return err
			}
			if err := announcePartnerStageChange(ctx, tx, auditID, "update", id); err != nil {
				return err
			}
		}
		if out, err = readPartnerStage(ctx, tx, id); err != nil {
			return err
		}
		return partnerCountsForCaller(ctx, tx, []crmcontracts.PartnerStage{out})
	})
	return out, err
}

// partnerCountsForCaller replaces each stage's partner_count with the live
// partners of companies this caller may read, and drops it for a seat that may
// not read partners and companies: the count is a reading of the register. The
// refusals that rest on use are decided before this, from every partner.
func partnerCountsForCaller(ctx context.Context, tx pgx.Tx, stages []crmcontracts.PartnerStage) error {
	if !auth.ReadGranted(ctx, partnerMaskObject) || !auth.ReadGranted(ctx, companyEntity) {
		for i := range stages {
			stages[i].PartnerCount = nil
		}
		return nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	visible, err := visibleCompanySQL(ctx, "o", arg)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT p.relationship_stage, count(*)::int FROM partner p
		 WHERE %s AND EXISTS (SELECT 1 FROM company o WHERE o.id = p.company_id AND %s)
		 GROUP BY p.relationship_stage`, livePartnerSQL("p"), visible), args...)
	if err != nil {
		return fmt.Errorf("count the visible partners per stage: %w", err)
	}
	counts := map[string]int{}
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			rows.Close()
			return err
		}
		counts[key] = count
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range stages {
		count := counts[stages[i].Key]
		stages[i].PartnerCount = &count
	}
	return nil
}

// DeletePartnerStage removes a stage nothing depends on. A built-in stage, one
// any partner carries (archived ones included), or the last active candidate
// stage is a 409: retire it instead.
func (s *Store) DeletePartnerStage(ctx context.Context, id ids.UUID) error {
	if err := auth.Require(ctx, leadVocabularyObject, principal.ActionDelete); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		_, current, err := lockPartnerStage(ctx, tx, id)
		if err != nil {
			return err
		}
		if *current.System || *current.PartnerCount > 0 {
			return apperrors.ErrConflict
		}
		if err := refuseLastCandidateStage(ctx, tx, current); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM partner_stage WHERE id = $1`, id); err != nil {
			return fmt.Errorf("delete partner stage: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "erase", partnerStageObject, id,
			map[string]any{leadOriginKeyField: current.Key, leadOriginLabelField: current.Label}, nil)
		if err != nil {
			return err
		}
		return announcePartnerStageChange(ctx, tx, auditID, "erase", id)
	})
}

// refuseStageMeaningChange keeps the meaning of a stage partners stand on: a
// partner on a candidate stage must not become active by an edit to the list.
func refuseStageMeaningChange(before crmcontracts.PartnerStage, in UpdatePartnerStageInput) error {
	if in.Meaning == nil || *in.Meaning == before.Meaning || *before.PartnerCount == 0 {
		return nil
	}
	return apperrors.ErrConflict
}

// refuseLastCandidateStage refuses to take away the only active candidate
// stage, which defaultPartnerStageTx starts every new programme on. Two writers
// each retiring a different candidate would both see the other still active,
// so every such write takes one lock first and counts under it.
func refuseLastCandidateStage(ctx context.Context, tx pgx.Tx, stage crmcontracts.PartnerStage) error {
	if !stage.Active || stage.Meaning != crmcontracts.PartnerStageMeaningCandidate {
		return nil
	}
	if err := storekit.LockWriteIdentity(ctx, tx, partnerStageObject, "active_candidates"); err != nil {
		return err
	}
	var others bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM partner_stage
		                WHERE active AND meaning = 'candidate' AND id <> $1)`, ids.UUID(stage.Id)).Scan(&others); err != nil {
		return fmt.Errorf("count candidate partner stages: %w", err)
	}
	if !others {
		return apperrors.ErrConflict
	}
	return nil
}

func partnerStagePatch(before crmcontracts.PartnerStage, in UpdatePartnerStageInput) *storekit.Patch {
	p := storekit.NewPatch()
	if in.Label != nil {
		p.Set(leadOriginLabelField, before.Label, strings.TrimSpace(*in.Label))
	}
	if in.Meaning != nil {
		p.Set(partnerStageMeaningField, string(before.Meaning), string(*in.Meaning))
	}
	if in.SortOrder != nil {
		p.Set("sort_order", before.SortOrder, *in.SortOrder)
	}
	if in.Active != nil {
		p.Set("active", before.Active, *in.Active)
	}
	return p
}

// announcePartnerStageChange is the event half of every catalog write:
// reporting.changed, which a reader of stage labels re-reads on.
func announcePartnerStageChange(ctx context.Context, tx pgx.Tx, auditID ids.UUID, action string, id ids.UUID) error {
	return storekit.Emit(ctx, tx, auditID, "reporting.changed", partnerStageObject, id,
		crmcontracts.InternalEventReportingChanged{Object: partnerStageObject, Action: action})
}

// lockPartnerStage takes the row FOR UPDATE and reads it, so a patch or a
// delete decided from the read cannot be overtaken by a concurrent writer.
func lockPartnerStage(ctx context.Context, tx pgx.Tx, id ids.UUID) (storekit.RowLock, crmcontracts.PartnerStage, error) {
	lock, err := storekit.LockRow(ctx, tx, partnerStageTable, id, storekit.NoArchiveColumn)
	if err != nil {
		return lock, crmcontracts.PartnerStage{}, err
	}
	stage, err := readPartnerStage(ctx, tx, id)
	return lock, stage, err
}

func readPartnerStage(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.PartnerStage, error) {
	return scanPartnerStage(tx.QueryRow(ctx, `SELECT `+partnerStageColumns+` FROM partner_stage WHERE id = $1`, id))
}

func scanPartnerStage(row pgx.Row) (crmcontracts.PartnerStage, error) {
	var out crmcontracts.PartnerStage
	var id ids.UUID
	var meaning string
	var system bool
	var version crmcontracts.RowVersion
	var count int
	if err := row.Scan(&id, &out.Key, &out.Label, &out.LabelKey, &meaning, &out.SortOrder, &out.Active, &system,
		&version, &out.CreatedAt, &out.UpdatedAt, &count); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, apperrors.ErrNotFound
		}
		return out, fmt.Errorf("scan partner stage: %w", err)
	}
	out.Id = openapi_types.UUID(id)
	out.Meaning = crmcontracts.PartnerStageMeaning(meaning)
	out.System, out.Version, out.PartnerCount = &system, &version, &count
	return out, nil
}
