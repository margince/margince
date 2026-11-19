// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package customfields

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// ChoiceRemover clears choices removed from a field off the records of one
// object, inside the options edit's transaction, and answers how many records
// changed. The record modules own those tables, so compose injects one per
// object.
type ChoiceRemover func(ctx context.Context, tx pgx.Tx, column fieldcatalog.Column, removed []string) (int, error)

// departedMemberWork is the constraint the retired-member trigger raises when
// a write touches live work a departed member still owns.
const departedMemberWork = "member_work_reference_retired"

// WithChoiceRemovers injects the removers, keyed by object. Without one for an
// object, an options edit that removes a choice its records hold is refused by
// the regenerated CHECK, as before.
func (h Handlers) WithChoiceRemovers(removers map[string]ChoiceRemover) Handlers {
	svc := *h.svc
	svc.removers = removers
	h.svc = &svc
	return h
}

// removedChoices is the choices in before that after no longer lists.
func removedChoices(before, after []string) []string {
	var removed []string
	for _, choice := range before {
		if !slices.Contains(after, choice) {
			removed = append(removed, choice)
		}
	}
	return removed
}

func (s *Service) clearRemovedChoices(ctx context.Context, tx pgx.Tx, f lockedField, removed []string) (int, error) {
	remove, ok := s.removers[f.Object]
	if !ok || len(removed) == 0 {
		return 0, nil
	}
	changed, err := remove(ctx, tx, fieldcatalog.Column{Name: f.ColumnName, Type: f.Type}, removed)
	if err == nil {
		return changed, nil
	}
	if constraint, violated := storekit.CheckViolation(err); violated && constraint == departedMemberWork {
		return 0, fmt.Errorf("a %s that holds a removed choice belongs to a member who has left; hand it to a colleague first: %w",
			f.Object, apperrors.ErrConflict)
	}
	if lockTimedOut(err) {
		return 0, ErrTableBusy
	}
	return 0, fmt.Errorf("customfields: clearing removed choices: %w", err)
}

// ChoiceUse answers how many records hold each current choice of a picklist or
// multiselect field, archived records included, because an options edit clears
// them all. It asks for the grant that edits the options, since the answer is
// for somebody about to edit them.
func (s *Service) ChoiceUse(ctx context.Context, id ids.UUID) (crmcontracts.CustomFieldChoiceUse, error) {
	use := crmcontracts.CustomFieldChoiceUse{Choices: []crmcontracts.CustomFieldChoiceCount{}}
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return use, err
	}
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var object, column, fieldType string
		var raw []byte
		err := tx.QueryRow(ctx, `SELECT object, column_name, type, options FROM custom_field WHERE id = $1`, id).
			Scan(&object, &column, &fieldType, &raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("customfields: reading catalog row: %w", err)
		}
		if !hasOptions(fieldType) {
			return ErrNotPicklist
		}
		options, err := unmarshalOptions(raw)
		if err != nil {
			return err
		}
		counts, err := countChoices(ctx, tx, object, column, fieldType)
		if err != nil {
			return err
		}
		for _, choice := range options {
			use.Choices = append(use.Choices, crmcontracts.CustomFieldChoiceCount{Choice: choice, Records: counts[choice]})
		}
		return nil
	})
	return use, err
}

// countChoices counts the records holding each value of one choice column.
// object and column come from the catalog row and are quoted as identifiers.
func countChoices(ctx context.Context, tx pgx.Tx, object, column, fieldType string) (map[string]int, error) {
	value := quoteIdentifier(column)
	from := quoteIdentifier(object)
	if fieldType == TypeMultiselect {
		from += ", unnest(" + value + ") AS choice"
		value = "choice"
	}
	rows, err := tx.Query(ctx, `SELECT `+value+`, count(*) FROM `+from+` WHERE `+value+` IS NOT NULL GROUP BY 1`)
	if err != nil {
		return nil, fmt.Errorf("customfields: counting choices: %w", err)
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var choice string
		var records int
		if err := rows.Scan(&choice, &records); err != nil {
			return nil, fmt.Errorf("customfields: counting choices: %w", err)
		}
		counts[choice] = records
	}
	return counts, rows.Err()
}

// ListCustomFieldChoiceUse serves GET /custom-fields/{id}/choices, asked before
// an options edit is confirmed.
func (h Handlers) ListCustomFieldChoiceUse(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	use, err := h.svc.ChoiceUse(r.Context(), ids.UUID(id))
	if err != nil {
		writeCustomFieldErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, use)
}
