// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assignments

// Reading assignments, and the subject checks the writes lean on.
//
// A read joins the role and the subject so a row renders without a second
// lookup, and so a retired role or a deactivated user still shows a name: an
// assignment whose label disappeared would read as a bug rather than as
// history.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// sourceHuman is what a person acting in the UI writes. Connectors and imports
// would name themselves; nothing writes assignments but people today.
const sourceHuman = "human"

// assignmentSelect reads an assignment with the role and subject it names. The
// subject name comes from whichever arm is populated, and COALESCE collapses
// the two so the caller gets one column rather than a pair it has to choose
// between.
const assignmentSelect = `
	SELECT a.id,
	       a.company_id, a.deal_id, a.project_id,
	       a.user_id, a.team_id,
	       COALESCE(u.display_name, t.name) AS subject_name,
	       COALESCE(u.status <> 'active' OR u.archived_at IS NOT NULL, t.archived_at IS NOT NULL, false) AS subject_inactive,
	       a.role_id, r.key, r.label, r.active, r.sort_order,
	       a.source, a.version, a.created_at, a.updated_at, a.archived_at
	  FROM record_assignment a
	  JOIN record_role r ON r.id = a.role_id
	  LEFT JOIN app_user u ON u.id = a.user_id
	  LEFT JOIN team t ON t.id = a.team_id`

// assignmentRow is the stored shape behind the wire shape.
type assignmentRow struct {
	ID              ids.UUID
	RecordType      crmcontracts.AssignmentRecordType
	RecordID        ids.UUID
	SubjectKind     crmcontracts.AssignmentSubjectKind
	SubjectID       ids.UUID
	SubjectName     string
	SubjectInactive bool
	RoleID          ids.UUID
	RoleKey         string
	RoleLabel       string
	RoleActive      bool
	RoleSortOrder   int
	Source          *string
	Version         int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ArchivedAt      *time.Time
}

func scanAssignmentRow(sc rowScanner) (assignmentRow, error) {
	var r assignmentRow
	var company, deal, project, user, team *ids.UUID
	if err := sc.Scan(&r.ID, &company, &deal, &project, &user, &team,
		&r.SubjectName, &r.SubjectInactive,
		&r.RoleID, &r.RoleKey, &r.RoleLabel, &r.RoleActive, &r.RoleSortOrder,
		&r.Source, &r.Version, &r.CreatedAt, &r.UpdatedAt, &r.ArchivedAt); err != nil {
		return assignmentRow{}, err
	}
	rt, id, err := parentOf(company, deal, project)
	if err != nil {
		return assignmentRow{}, err
	}
	r.RecordType, r.RecordID = rt, id
	switch {
	case user != nil:
		r.SubjectKind, r.SubjectID = crmcontracts.AssignmentSubjectKindUser, *user
	case team != nil:
		r.SubjectKind, r.SubjectID = crmcontracts.AssignmentSubjectKindTeam, *team
	default:
		return assignmentRow{}, fmt.Errorf("record_assignment row has no assignee")
	}
	return r, nil
}

func (r assignmentRow) wire() crmcontracts.RecordAssignment {
	name, inactive := r.SubjectName, r.SubjectInactive
	key, label, active := r.RoleKey, r.RoleLabel, r.RoleActive
	version, created, updated := r.Version, r.CreatedAt, r.UpdatedAt
	return crmcontracts.RecordAssignment{
		Id:              openapi_types.UUID(r.ID),
		RecordType:      r.RecordType,
		RecordId:        openapi_types.UUID(r.RecordID),
		SubjectKind:     r.SubjectKind,
		SubjectId:       openapi_types.UUID(r.SubjectID),
		SubjectName:     &name,
		SubjectInactive: &inactive,
		RoleId:          openapi_types.UUID(r.RoleID),
		RoleKey:         &key,
		RoleLabel:       &label,
		RoleActive:      &active,
		Source:          r.Source,
		Version:         &version,
		CreatedAt:       &created,
		UpdatedAt:       &updated,
	}
}

// ListForRecord answers with the live assignments on one record.
//
// Authority is the parent's: a caller who cannot see the record gets not-found,
// so the list never confirms that a record they cannot open exists.
func (s *Store) ListForRecord(
	ctx context.Context, rt crmcontracts.AssignmentRecordType, id ids.UUID,
) ([]crmcontracts.RecordAssignment, error) {
	column, err := parentColumn(rt)
	if err != nil {
		return nil, err
	}
	var out []crmcontracts.RecordAssignment
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := ensureParentReadable(ctx, tx, rt, id); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			assignmentSelect+` WHERE a.`+column+` = $1 AND a.archived_at IS NULL
			 ORDER BY r.sort_order, r.label, subject_name`, id)
		if err != nil {
			return fmt.Errorf("list record assignments: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			row, err := scanAssignmentRow(rows)
			if err != nil {
				return fmt.Errorf("scan record assignment: %w", err)
			}
			out = append(out, row.wire())
		}
		return rows.Err()
	})
	return out, err
}

// parentColumn names the FK column for a record type.
func parentColumn(rt crmcontracts.AssignmentRecordType) (string, error) {
	switch rt {
	case crmcontracts.AssignmentRecordTypeCompany:
		return "company_id", nil
	case crmcontracts.AssignmentRecordTypeDeal:
		return "deal_id", nil
	case crmcontracts.AssignmentRecordTypeProject:
		return "project_id", nil
	}
	return "", &values.ParseError{
		Field:   recordTypeColumnOf,
		Code:    codeInvalidRecType,
		Message: "record_type is one of company, deal, project",
	}
}

// readAssignment serves one assignment back to a caller, so it bounds the row on
// its parent: the record ids it carries are references, and handing one back
// unbounded would tell a caller a record exists that they cannot open.
func readAssignment(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.RecordAssignment, error) {
	row := tx.QueryRow(ctx, assignmentSelect+` WHERE a.id = $1`, id)
	r, err := scanAssignmentRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.RecordAssignment{}, apperrors.ErrNotFound
	}
	if err != nil {
		return crmcontracts.RecordAssignment{}, fmt.Errorf("read record assignment: %w", err)
	}
	if err := ensureParentReadable(ctx, tx, r.RecordType, r.RecordID); err != nil {
		return crmcontracts.RecordAssignment{}, err
	}
	return r.wire(), nil
}

// lockAssignment takes the row lock the reassign and archive paths rely on, and
// checks write authority against the STORED parent before returning the row.
//
// The check lives HERE rather than in each caller so the row cannot leave this
// function unbounded: the parent comes off the stored row, so a caller naming a
// record they may write cannot reach an assignment on one they may not.
func lockAssignment(ctx context.Context, tx pgx.Tx, id ids.UUID) (assignmentRow, error) {
	row := tx.QueryRow(ctx, assignmentSelect+` WHERE a.id = $1 FOR UPDATE OF a`, id)
	r, err := scanAssignmentRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return assignmentRow{}, apperrors.ErrNotFound
	}
	if err != nil {
		return assignmentRow{}, fmt.Errorf("lock record assignment: %w", err)
	}
	if err := ensureParentWritable(ctx, tx, r.RecordType, r.RecordID); err != nil {
		return assignmentRow{}, err
	}
	return r, nil
}

// roleLookupError turns a missing role into the field error the caller can act
// on rather than a bare not-found, which would read as the RECORD being absent.
func roleLookupError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return &values.ParseError{
			Field: fieldRoleID, Code: "unknown_role",
			Message: "no such role",
		}
	}
	return fmt.Errorf("read record role: %w", err)
}

// ensureSubjectAssignable refuses an assignee that does not exist, and refuses
// a deactivated user for a NEW assignment while leaving existing ones standing:
// history keeps its name, but nobody is newly made responsible for work they
// have left.
func ensureSubjectAssignable(
	ctx context.Context, tx pgx.Tx, kind crmcontracts.AssignmentSubjectKind, id ids.UUID,
) error {
	if kind == crmcontracts.AssignmentSubjectKindTeam {
		var teamArchived *time.Time
		if err := tx.QueryRow(ctx, `SELECT archived_at FROM team WHERE id = $1`, id).Scan(&teamArchived); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return &values.ParseError{
					Field: fieldSubjectID, Code: "unknown_team", Message: "no such team",
				}
			}
			return fmt.Errorf("read team: %w", err)
		}
		if teamArchived != nil {
			return &values.ParseError{
				Field: fieldSubjectID, Code: "archived_team",
				Message: "that team is archived and cannot take on new responsibilities",
			}
		}
		return nil
	}
	var status string
	var archivedAt *time.Time
	if err := tx.QueryRow(ctx,
		`SELECT status, archived_at FROM app_user WHERE id = $1`, id).Scan(&status, &archivedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &values.ParseError{
				Field: fieldSubjectID, Code: "unknown_user", Message: "no such user",
			}
		}
		return fmt.Errorf("read app_user: %w", err)
	}
	// `invited` is deliberately assignable: staffing a record is part of
	// onboarding somebody, and refusing it would mean nobody could be given
	// work until their first login.
	if status == "suspended" || status == "deactivated" || archivedAt != nil {
		return &values.ParseError{
			Field: fieldSubjectID, Code: "inactive_user",
			Message: "that person is deactivated and cannot take on new responsibilities",
		}
	}
	return nil
}
