// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// DealFiling is what an automatic deal filing did.
type DealFiling string

// The outcomes of an automatic deal filing.
const (
	// DealFilingLanded wrote the link, its audit row and its event.
	DealFilingLanded DealFiling = "landed"
	// DealFilingAlreadyFiled found a deal link there under the lock, and kept it.
	DealFilingAlreadyFiled DealFiling = "already_filed"
	// DealFilingRefused wrote nothing: the row is gone, or the writer may not change it.
	DealFilingRefused DealFiling = "refused"
)

// FileOnDealIfUnfiledTx is the automatic deal filing: the guarded relink write,
// taken only if the activity still has no deal link under its row lock. A deal
// link that landed first, a human's included, is kept and reported. The link
// is written with its audit row, the evidence given, and the relink event a
// filing's consumers read.
func FileOnDealIfUnfiledTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, deal ids.UUID, evidence map[string]any) (DealFiling, error) {
	in := RelinkActivityInput{EntityType: linkEntityDeal, EntityID: deal, unfiledOnly: true, evidence: evidence}
	wrote, _, err := relinkActivityRow(ctx, tx, id, in, linkColumn(linkEntityDeal))
	switch {
	case errors.Is(err, apperrors.ErrNotFound), errors.Is(err, apperrors.ErrPermissionDenied):
		return DealFilingRefused, nil
	case err != nil:
		return "", err
	case wrote:
		return DealFilingLanded, nil
	}
	filed, err := hasLinkOfType(ctx, tx, id, linkEntityDeal)
	if err != nil || !filed {
		return DealFilingRefused, err
	}
	return DealFilingAlreadyFiled, nil
}

// FileOnLeadTx is the automatic lead filing: the guarded relink write of one
// lead link, with its audit row, the evidence given and the relink event. It
// answers whether the link landed. A link already there, a row gone or a writer
// refused all answer false.
func FileOnLeadTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, lead ids.UUID, evidence map[string]any) (bool, error) {
	in := RelinkActivityInput{EntityType: linkEntityLead, EntityID: lead, evidence: evidence}
	wrote, _, err := relinkActivityRow(ctx, tx, id, in, linkColumn(linkEntityLead))
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	return wrote, err
}

// RetractMachineLeadTx takes one lead link off an activity, only while no human
// or agent ever filed the activity under that lead or took it off.
func RetractMachineLeadTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, lead ids.UUID, evidence map[string]any) (bool, error) {
	human := filingByHand{sql: provenance.HumanLeadFilingSQL("$1", "$2::uuid"), args: []any{id, lead}}
	return retractMachineLinkTx(ctx, tx, id, linkEntityLead, lead, human, evidence)
}

// RetractMachineDealTx takes one deal link off an activity, only while no human
// or agent ever filed the activity on a deal.
func RetractMachineDealTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, deal ids.UUID, evidence map[string]any) (bool, error) {
	human := filingByHand{sql: provenance.HumanDealFilingSQL("$1"), args: []any{id}}
	return retractMachineLinkTx(ctx, tx, id, linkEntityDeal, deal, human, evidence)
}

// filingByHand is the provenance test that a human or an agent filed the
// activity, with its arguments.
type filingByHand struct {
	sql  string
	args []any
}

// retractMachineLinkTx takes one automatic link off under the activity's row
// lock, unless the human test finds a human's filing. It writes the audit row with the evidence given and the relink event,
// as the filing did, and answers whether the link came off.
func retractMachineLinkTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, entityType string, target ids.UUID, human filingByHand, evidence map[string]any,
) (bool, error) {
	if _, err := storekit.LockRow(ctx, tx, "activity", id.UUID, storekit.LiveOnly); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	var filedByHand bool
	if err := tx.QueryRow(ctx, `SELECT `+human.sql, human.args...).Scan(&filedByHand); err != nil {
		return false, fmt.Errorf("activities: reading who filed the activity under a %s: %w", entityType, err)
	}
	if filedByHand {
		return false, nil
	}
	tag, err := tx.Exec(ctx, storekit.SQLf(`DELETE FROM activity_link WHERE activity_id = $1 AND entity_type = $2 AND %s = $3`,
		linkColumn(entityType)), id, entityType, target)
	if err != nil || tag.RowsAffected() == 0 {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE activity SET updated_at = now() WHERE id = $1`, id); err != nil {
		return false, err
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "activity_relink", "activity", id.UUID,
		map[string]any{fieldEntityType: entityType, fieldEntityID: target}, nil, evidence)
	if err != nil {
		return false, err
	}
	return true, storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: relinkedChangedFields(entityType, target),
	})
}

func hasLinkOfType(ctx context.Context, tx pgx.Tx, id ids.ActivityID, entityType string) (bool, error) {
	var filed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity_link WHERE activity_id = $1 AND entity_type = $2)`,
		id, entityType).Scan(&filed); err != nil {
		return false, fmt.Errorf("activities: reading the activity's %s links: %w", entityType, err)
	}
	return filed, nil
}
