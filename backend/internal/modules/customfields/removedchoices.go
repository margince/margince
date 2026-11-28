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

// The constraints a record raises when it refuses the clear: live work a
// departed member still owns, and an activity under a retention hold.
const (
	departedMemberWork = "member_work_reference_retired"
	restrictedActivity = "activity_restricted_immutable"
)

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

// lockForTheAlter takes the lock the CHECK rebuild needs before the clear runs.
// Then no record can take a removed choice between the clear and the ALTER,
// and a busy table refuses before any row is read.
func lockForTheAlter(ctx context.Context, tx pgx.Tx, object string) error {
	if !validIdentifier(object) {
		return fmt.Errorf("customfields: invalid object identifier %q", object)
	}
	if _, err := tx.Exec(ctx, `LOCK TABLE `+quoteIdentifier(object)+` IN ACCESS EXCLUSIVE MODE`); err != nil {
		if lockTimedOut(err) {
			return ErrTableBusy
		}
		return fmt.Errorf("customfields: locking %s: %w", object, err)
	}
	return nil
}

// clearRemovedChoices clears removed choices off the records. It changes
// records, so the caller needs the grant to update the field's object too.
func (s *Service) clearRemovedChoices(ctx context.Context, tx pgx.Tx, f lockedField, removed []string) (int, error) {
	if len(removed) == 0 {
		return 0, nil
	}
	if err := auth.Require(ctx, f.Object, principal.ActionUpdate); err != nil {
		return 0, err
	}
	remove, ok := s.removers[f.Object]
	if !ok {
		return 0, nil
	}
	changed, err := remove(ctx, tx, fieldcatalog.Column{Name: f.ColumnName, Type: f.Type}, removed)
	if err == nil {
		return changed, nil
	}
	if constraint, violated := storekit.CheckViolation(err); violated {
		return 0, refusedClear(f.Object, constraint)
	}
	if lockTimedOut(err) {
		return 0, ErrTableBusy
	}
	return 0, fmt.Errorf("customfields: clearing removed choices: %w", err)
}

// refusedClear names why a record that holds a removed choice refused to change.
func refusedClear(object, constraint string) error {
	switch constraint {
	case departedMemberWork:
		return fmt.Errorf("a %s that holds a removed choice belongs to a member who has left; hand it to a colleague first: %w",
			object, apperrors.ErrConflict)
	case restrictedActivity:
		return fmt.Errorf("an activity under a retention hold holds a removed choice, and a held activity cannot change: %w",
			apperrors.ErrConflict)
	default:
		return fmt.Errorf("a %s that holds a removed choice cannot be changed now: %w", object, apperrors.ErrConflict)
	}
}

// ChoiceUse answers how many records hold each current choice of a picklist or
// multiselect field, archived records included, because an options edit clears
// them all. It is a read: it needs custom_field read and read on the field's
// object, since it counts that object's records.
func (s *Service) ChoiceUse(ctx context.Context, id ids.UUID) (crmcontracts.CustomFieldChoiceUse, error) {
	use := crmcontracts.CustomFieldChoiceUse{Choices: []crmcontracts.CustomFieldChoiceCount{}}
	if err := auth.Require(ctx, rbacObject, principal.ActionRead); err != nil {
		return use, err
	}
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
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
		if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
			return err
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
