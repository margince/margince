// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assignments

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// InspectMemberAssignmentsTx returns no record details outside the caller's scope.
func InspectMemberAssignmentsTx(ctx context.Context, tx pgx.Tx, member ids.UUID, receiver *principal.Principal) (workforce.MemberWorkSnapshot, error) {
	snapshot, err := memberAssignmentSummaryTx(ctx, tx, member)
	if err != nil {
		return snapshot, err
	}
	rows, err := memberAssignmentsTx(ctx, tx, member)
	if err != nil {
		return snapshot, err
	}
	for _, row := range rows {
		live, err := assignmentParentLive(ctx, tx, row)
		if err != nil {
			return snapshot, err
		}
		if !live {
			continue
		}
		err = ensureParentReadable(ctx, tx, row.RecordType, row.RecordID, true)
		if err == nil {
			snapshot.Visible++
		} else if !assignmentTransferRefused(err) {
			return snapshot, err
		}
		if receiver == nil {
			continue
		}
		allowed, err := assignmentTransferAllowed(ctx, tx, row, *receiver)
		if err != nil {
			return snapshot, err
		}
		if allowed {
			snapshot.Transferable++
		}
	}
	return snapshot, nil
}

func memberAssignmentsTx(ctx context.Context, tx pgx.Tx, member ids.UUID) ([]assignmentRow, error) {
	if err := auth.Require(ctx, "user_admin", principal.ActionUpdate); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM record_assignment WHERE user_id=@member AND archived_at IS NULL ORDER BY id`, pgx.StrictNamedArgs{memberTransferArgument: member})
	if err != nil {
		return nil, err
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, err
	}
	visible := []assignmentRow{}
	for _, id := range candidates {
		row, err := readAssignmentRow(ctx, tx, id)
		if assignmentTransferRefused(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		visible = append(visible, row)
	}
	return visible, nil
}

const memberTransferArgument = "member"

const memberAssignmentParents = ` FROM record_assignment a
 LEFT JOIN company c ON c.id=a.company_id
 LEFT JOIN deal d ON d.id=a.deal_id
 LEFT JOIN project p ON p.id=a.project_id
 WHERE a.user_id=@member AND a.archived_at IS NULL`

func memberAssignmentSummaryTx(ctx context.Context, tx pgx.Tx, member ids.UUID) (workforce.MemberWorkSnapshot, error) {
	var snapshot workforce.MemberWorkSnapshot
	if err := auth.Require(ctx, "user_admin", principal.ActionUpdate); err != nil {
		return snapshot, err
	}
	var stamp string
	err := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE COALESCE(c.archived_at,d.archived_at,p.archived_at) IS NULL),
 COALESCE(string_agg(a.id::text || ':' || a.version::text || ':' || COALESCE(c.version,d.version,p.version)::text || ':' ||
 (COALESCE(c.archived_at,d.archived_at,p.archived_at) IS NULL)::text,';' ORDER BY a.id),'')`+memberAssignmentParents, pgx.StrictNamedArgs{memberTransferArgument: member}).Scan(&snapshot.Owned, &stamp)
	snapshot.Stamp = []string{"record_assignment:" + stamp}
	return snapshot, err
}

func assignmentParentLive(ctx context.Context, tx pgx.Tx, row assignmentRow) (bool, error) {
	if _, err := parentColumn(row.RecordType); err != nil {
		return false, err
	}
	query := fmt.Sprintf("SELECT archived_at IS NULL FROM %s WHERE id=@id", pgx.Identifier{string(row.RecordType)}.Sanitize())
	var live bool
	err := tx.QueryRow(ctx, query, pgx.StrictNamedArgs{"id": row.RecordID}).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return live, err
}

func assignmentTransferAllowed(ctx context.Context, tx pgx.Tx, row assignmentRow, receiver principal.Principal) (bool, error) {
	if receiver.Type != principal.PrincipalHuman {
		return false, nil
	}
	if err := readAssignableRole(ctx, tx, row.RoleID, row.RecordType, crmcontracts.AssignmentSubjectKindUser, false); err != nil {
		if assignmentTransferRefused(err) {
			return false, nil
		}
		return false, err
	}
	allowed, err := auth.WritableLiveBy(ctx, tx, string(row.RecordType), row.RecordID)
	if err != nil || !allowed {
		return false, err
	}
	return auth.WritableLiveBy(principal.WithActor(ctx, receiver), tx, string(row.RecordType), row.RecordID)
}

// ArchiveRetiredMemberAssignmentsTx ends stale responsibilities without changing a retired parent.
func ArchiveRetiredMemberAssignmentsTx(ctx context.Context, tx pgx.Tx, member ids.UUID) error {
	if err := auth.Require(ctx, "user_admin", principal.ActionUpdate); err != nil {
		return err
	}
	for _, kind := range []crmcontracts.AssignmentRecordType{crmcontracts.AssignmentRecordTypeCompany, crmcontracts.AssignmentRecordTypeDeal, crmcontracts.AssignmentRecordTypeProject} {
		column, err := parentColumn(kind)
		if err != nil {
			return err
		}
		query := fmt.Sprintf(`SELECT 1 FROM %s p JOIN record_assignment a ON a.%s=p.id
   WHERE a.user_id=@member AND a.archived_at IS NULL ORDER BY p.id FOR UPDATE OF p`, pgx.Identifier{string(kind)}.Sanitize(), pgx.Identifier{column}.Sanitize())
		if _, err := tx.Exec(ctx, query, pgx.StrictNamedArgs{memberTransferArgument: member}); err != nil {
			return err
		}
	}
	rows, err := tx.Query(ctx, `SELECT a.id,a.version`+memberAssignmentParents+` AND COALESCE(c.archived_at,d.archived_at,p.archived_at) IS NOT NULL ORDER BY a.id FOR UPDATE OF a`, pgx.StrictNamedArgs{memberTransferArgument: member})
	if err != nil {
		return err
	}
	type ended struct {
		ID      ids.UUID
		Version int64
	}
	endedRows, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ended])
	if err != nil {
		return err
	}
	for _, row := range endedRows {
		if err := archiveAssignmentTx(ctx, tx, assignmentRow{ID: row.ID, Version: row.Version}, map[string]any{"ended_with_parent": true}); err != nil {
			return err
		}
	}
	return nil
}
