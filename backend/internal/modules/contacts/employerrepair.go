// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EmployerRepairPage records progress even when a contact has no employer evidence.
type EmployerRepairPage struct {
	Scanned []ids.UUID
	Changed int
}

const missingContactEmployer = `c.archived_at IS NULL AND c.merged_into_id IS NULL AND c.visibility = 'workspace'
 AND NOT EXISTS (SELECT 1 FROM relationship r WHERE r.contact_id = c.id AND r.kind = 'employment')`

// ReconcileContactEmployersTx reuses retained lead and address evidence without replacing an employer.
func (s *Store) ReconcileContactEmployersTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (EmployerRepairPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return EmployerRepairPage{}, err
	}
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return EmployerRepairPage{}, err
	}
	args := []any{after, limit}
	rows, err := tx.Query(ctx, storekit.SQLf(`SELECT c.id FROM contact c WHERE c.id > $%d AND `+missingContactEmployer+` ORDER BY c.id LIMIT $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return EmployerRepairPage{}, err
	}
	page := EmployerRepairPage{}
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return EmployerRepairPage{}, err
		}
		page.Scanned = append(page.Scanned, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return EmployerRepairPage{}, err
	}
	for _, id := range page.Scanned {
		changed := false
		err := pgx.BeginFunc(ctx, tx, func(savepoint pgx.Tx) error {
			var err error
			changed, err = s.reconcileContactEmployerTx(ctx, savepoint, ids.From[ids.ContactKind](id))
			return err
		})
		if reason := EmployerRefusalReason(err); reason != "" {
			_, err = storekit.LogSystem(ctx, tx, "contact_employer_repair.skipped", map[string]any{string(consentSubjectContact): id, auditKeyReason: reason})
			changed = false
		}
		if err != nil {
			return EmployerRepairPage{}, err
		}
		if changed {
			page.Changed++
		}
	}
	return page, nil
}

func (s *Store) reconcileContactEmployerTx(ctx context.Context, tx pgx.Tx, id ids.ContactID) (bool, error) {
	if err := lockContactForAttach(ctx, tx, id); err != nil {
		return false, err
	}
	held, err := hasEmploymentHistory(ctx, tx, id)
	if err != nil || held {
		return false, err
	}
	contact, err := readContactRow(ctx, tx, id, storekit.LiveOnly, nil)
	if err != nil {
		return false, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	spec := ContactSpec{Source: contact.Source, Title: contact.Title, OwnerID: ownerFromUUID(uuidPtrToIDs(contact.OwnerId)), CapturedBy: by}
	if contact.ConvertedFromLeadId != nil {
		lead, err := readLead(ctx, tx, ids.From[ids.LeadKind](ids.UUID(*contact.ConvertedFromLeadId)), storekit.IncludeArchived, nil)
		if err != nil {
			return false, err
		}
		// A shipped repair keeps the rule it was released with: typed text creates.
		if spec.Employer, _, err = s.promotionEmployer(ctx, tx, lead, true); err != nil {
			return false, err
		}
	}
	var result EnsureCounterpartyResult
	spec.EmployerResult = &result
	if err := s.resolveCreatedEmployer(ctx, tx, id, spec); err != nil {
		return false, err
	}
	employer, err := currentEmployerID(ctx, tx, id)
	return employer != nil || result.TriagePending, err
}

// CountMissingContactEmployers bounds the data task estimate to its supplied cap.
func CountMissingContactEmployers(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, maximum int64,
) (int64, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return 0, err
	}
	args := []any{maximum + 1}
	var count int64
	err := q.QueryRow(ctx, storekit.SQLf(`SELECT count(*) FROM (SELECT 1 FROM contact c WHERE `+missingContactEmployer+` LIMIT $%d) candidates`, len(args)), args...).Scan(&count)
	return count, err
}

func hasEmploymentHistory(ctx context.Context, tx pgx.Tx, id ids.ContactID) (bool, error) {
	args := []any{id}
	var held bool
	err := tx.QueryRow(ctx, storekit.SQLf(`SELECT EXISTS (SELECT 1 FROM relationship WHERE contact_id = $%d AND kind = 'employment')`, len(args)), args...).Scan(&held)
	return held, err
}

// EmployerRefusalReason classifies permanent evidence refusals without exposing hidden records.
func EmployerRefusalReason(err error) string {
	if errors.Is(err, apperrors.ErrNotFound) {
		return "unavailable"
	}
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return "permission"
	}
	if detail, ok := errors.AsType[*httperr.DetailedError](err); ok && detail.Code == "validation_error" {
		for _, field := range detail.Fields {
			if field.Field == fieldCompanyName || field.Field == endCompanyColumn {
				return "conflicting_evidence"
			}
		}
	}
	return ""
}
