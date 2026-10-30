// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Loss reasons are the administered list a lost deal names why from
// (deal_loss_reason). The free note beside the code stays: it explains the one
// deal, while the code is what a report counts.

import (
	"context"
	"errors"
	"fmt"
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
	lossReasonTable     = "deal_loss_reason"
	lossReasonCodeField = "lost_reason_code"
	lossBasisField      = "lost_reason_basis"
	lossMappingField    = "lost_reason_mapping_id"
	lossReasonColumns   = `id, key, label, label_key, sort_order, active, system, version, created_at, updated_at`

	// The two bases a lost_reason_code carries, as deal_lost_reason_basis_check
	// names them.
	lossBasisRecorded = "recorded"
	lossBasisMapped   = "mapped"
)

// UnknownLossReasonError refuses a code the catalog does not hold.
type UnknownLossReasonError struct{ Field string }

func (e *UnknownLossReasonError) Error() string {
	return e.Field + " names no loss reason in the catalog"
}

// FieldFault names the caller's own field.
func (e *UnknownLossReasonError) FieldFault() (field, code, message string) {
	return e.Field, "unknown_loss_reason", e.Error()
}

// RetiredLossReasonError refuses a retired code for a new loss.
type RetiredLossReasonError struct{ Field, Key string }

func (e *RetiredLossReasonError) Error() string {
	return "loss reason " + e.Key + " is retired and cannot be chosen"
}

// FieldFault names the caller's own field.
func (e *RetiredLossReasonError) FieldFault() (field, code, message string) {
	return e.Field, "loss_reason_retired", e.Error()
}

// LossReasonNotLostError refuses a code on a move that does not lose the deal:
// the column holds only a loss's reason.
type LossReasonNotLostError struct{}

func (e *LossReasonNotLostError) Error() string {
	return "lost_reason_code is only accepted on a move to a lost stage"
}

// FieldFault names the code field.
func (e *LossReasonNotLostError) FieldFault() (field, code, message string) {
	return lossReasonCodeField, "lost_reason_code_not_lost", e.Error()
}

// LossReasonOutcome is what a loss records: the note, and the code with the
// basis it was recorded on. A nil Code is the legacy path, a note alone.
type LossReasonOutcome struct {
	Note  *string
	Code  *string
	Basis *string
}

// ResolveLossReasonTx validates what a loss says. Every door that loses a deal
// reaches it through the stage advance: HTTP, MCP, an approval, an import and
// a review answer. A loss needs a code or a note, and a code must be active.
func ResolveLossReasonTx(ctx context.Context, tx pgx.Tx, note, code *string) (LossReasonOutcome, error) {
	var out LossReasonOutcome
	if note != nil {
		if trimmed := strings.TrimSpace(*note); trimmed != "" {
			out.Note = &trimmed
		}
	}
	if code != nil && strings.TrimSpace(*code) != "" {
		key := strings.TrimSpace(*code)
		if err := ensureChoosableLossReason(ctx, tx, lossReasonCodeField, key); err != nil {
			return LossReasonOutcome{}, err
		}
		basis := lossBasisRecorded
		out.Code, out.Basis = &key, &basis
	}
	if out.Note == nil && out.Code == nil {
		return LossReasonOutcome{}, &LostReasonRequiredError{}
	}
	return out, nil
}

// ensureChoosableLossReason locks the catalog row, so a retirement that
// commits between this check and the deal write cannot let the key through.
func ensureChoosableLossReason(ctx context.Context, tx pgx.Tx, field, key string) error {
	var active bool
	err := tx.QueryRow(ctx, `SELECT active FROM deal_loss_reason WHERE key = $1 FOR SHARE`, key).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return &UnknownLossReasonError{Field: field}
	}
	if err != nil {
		return fmt.Errorf("read loss reason: %w", err)
	}
	if !active {
		return &RetiredLossReasonError{Field: field, Key: key}
	}
	return nil
}

// CreateLossReasonInput adds one reason to the catalog.
type CreateLossReasonInput struct {
	Key       string
	Label     string
	SortOrder int
}

// UpdateLossReasonInput relabels, reorders or retires one. The key is absent:
// it is the value lost deals carry.
type UpdateLossReasonInput struct {
	Label     *string
	SortOrder *int
	Active    *bool
	IfVersion *int64
}

// ListLossReasons answers the whole catalog, retired entries included, so a
// deal carrying a retired code still renders its label.
func (s *Store) ListLossReasons(ctx context.Context) ([]crmcontracts.LossReason, error) {
	return listCatalog(ctx, s, lossReasonTable, lossReasonColumns,
		"d.lost_reason_code = deal_loss_reason.key", scanLossReason,
		func(reason *crmcontracts.LossReason, count *int) { reason.DealCount = count })
}

// CreateLossReason adds a reason.
func (s *Store) CreateLossReason(ctx context.Context, in CreateLossReasonInput) (crmcontracts.LossReason, error) {
	if err := auth.Require(ctx, acquisitionVocabularyObject, principal.ActionCreate); err != nil {
		return crmcontracts.LossReason{}, err
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		return crmcontracts.LossReason{}, &values.ParseError{Field: sourceLabelColumn, Code: codeRequired, Message: "label is required"}
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		key = deriveAcquisitionKey(label)
	}
	if !lossReasonKeyShape.MatchString(key) {
		return crmcontracts.LossReason{}, &values.ParseError{
			Field: sourceKeyColumn, Code: "invalid_key",
			Message: "key must start with a letter and hold only lowercase letters, digits and underscores",
		}
	}
	var out crmcontracts.LossReason
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		id := ids.NewV7()
		_, err := tx.Exec(ctx,
			`INSERT INTO deal_loss_reason (id, key, label, sort_order) VALUES ($1, $2, $3, $4)`,
			id, key, label, in.SortOrder)
		if storekit.IsUniqueViolation(err) {
			return apperrors.ErrConflict
		}
		if err != nil {
			return fmt.Errorf("insert loss reason: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "create", "deal_loss_reason", id, nil,
			map[string]any{sourceKeyColumn: key, sourceLabelColumn: label})
		if err != nil {
			return err
		}
		if err := announceLossReasonChange(ctx, tx, auditID, lossReasonTable, "create", id); err != nil {
			return err
		}
		out, err = readLossReason(ctx, tx, id)
		return err
	})
	return out, err
}

// UpdateLossReason relabels, reorders or retires one. There is no delete: a
// code a lost deal carries must stay resolvable.
func (s *Store) UpdateLossReason(ctx context.Context, id ids.UUID, in UpdateLossReasonInput) (crmcontracts.LossReason, error) {
	if err := auth.Require(ctx, acquisitionVocabularyObject, principal.ActionUpdate); err != nil {
		return crmcontracts.LossReason{}, err
	}
	if in.Label != nil && strings.TrimSpace(*in.Label) == "" {
		return crmcontracts.LossReason{}, &values.ParseError{Field: sourceLabelColumn, Code: codeRequired, Message: "label cannot be blank"}
	}
	var out crmcontracts.LossReason
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		lock, err := storekit.LockRow(ctx, tx, lossReasonTable, id, storekit.NoArchiveColumn)
		if err != nil {
			return err
		}
		before, err := readLossReason(ctx, tx, id)
		if err != nil {
			return err
		}
		if in.IfVersion != nil && before.Version != nil && *in.IfVersion != *before.Version {
			return apperrors.ErrConflict
		}
		patch := lossReasonPatch(before, in)
		if !patch.Empty() {
			if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
				return err
			}
			auditID, err := storekit.Audit(ctx, tx, "update", "deal_loss_reason", id, patch.Before(), patch.After())
			if err != nil {
				return err
			}
			if err := announceLossReasonChange(ctx, tx, auditID, lossReasonTable, "update", id); err != nil {
				return err
			}
		}
		out, err = readLossReason(ctx, tx, id)
		return err
	})
	return out, err
}

func lossReasonPatch(before crmcontracts.LossReason, in UpdateLossReasonInput) *storekit.Patch {
	patch := storekit.NewPatch()
	if in.Label != nil {
		patch.Set(sourceLabelColumn, before.Label, strings.TrimSpace(*in.Label))
	}
	if in.SortOrder != nil {
		patch.Set("sort_order", before.SortOrder, *in.SortOrder)
	}
	if in.Active != nil {
		patch.Set("active", before.Active, *in.Active)
	}
	return patch
}

// announceLossReasonChange is the event half of a catalog or mapping write:
// reporting.changed, which a report reader re-reads its labels on.
func announceLossReasonChange(ctx context.Context, tx pgx.Tx, auditID ids.UUID, object, action string, id ids.UUID) error {
	return storekit.Emit(ctx, tx, auditID, "reporting.changed", object, id,
		crmcontracts.InternalEventReportingChanged{Object: object, Action: action})
}

func readLossReason(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.LossReason, error) {
	return scanLossReason(tx.QueryRow(ctx, `SELECT `+lossReasonColumns+` FROM deal_loss_reason WHERE id = $1`, id))
}

// scanLossReason reads the catalog projection, then any columns a read
// selects after it into extra.
func scanLossReason(row pgx.Row, extra ...any) (crmcontracts.LossReason, error) {
	var out crmcontracts.LossReason
	var id ids.UUID
	var version int64
	dest := []any{&id, &out.Key, &out.Label, &out.LabelKey, &out.SortOrder, &out.Active, &out.System, &version, &out.CreatedAt, &out.UpdatedAt}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, apperrors.ErrNotFound
		}
		return out, fmt.Errorf("scan loss reason: %w", err)
	}
	out.Id = openapi_types.UUID(id)
	out.Version = &version
	return out, nil
}
