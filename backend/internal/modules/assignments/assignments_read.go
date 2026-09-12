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

// sourceHuman is what a human acting in the UI writes. Connectors and imports
// would name themselves; nothing writes assignments but humans today.
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

// lockAssignment holds the parent, then locks the assignment row the reassign
// and archive paths are about to change.
//
// THAT ORDER, and it is load-bearing. Art. 17 erasure locks its subject and
// then deletes the rows hanging off it, so a writer that took its own row first
// would deadlock against the eraser and, when the eraser lost, cost somebody
// their erasure. So the parent is read unlocked to learn WHICH record this
// assignment hangs on, the parent is held, and only then is the assignment row
// locked and re-read under that hold.
//
// The authority check lives here rather than in each caller so the row cannot
// leave this function unbounded: the parent comes off the STORED row, so a
// caller naming a record they may write cannot reach an assignment on one they
// may not. The first read is a lookup, not a decision — nothing is returned
// from it, and every value the caller acts on comes from the second.
func lockAssignment(ctx context.Context, tx pgx.Tx, id ids.UUID) (assignmentRow, error) {
	parent, err := parentOfAssignment(ctx, tx, id)
	if err != nil {
		return assignmentRow{}, err
	}
	if err := ensureParentWritable(ctx, tx, parent.recordType, parent.recordID); err != nil {
		return assignmentRow{}, err
	}
	row := tx.QueryRow(ctx, assignmentSelect+` WHERE a.id = $1 FOR UPDATE OF a`, id)
	r, err := scanAssignmentRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return assignmentRow{}, apperrors.ErrNotFound
	}
	if err != nil {
		return assignmentRow{}, fmt.Errorf("lock record assignment: %w", err)
	}
	// The parent cannot have moved — an assignment never changes the record it
	// hangs on — but the row was re-read under the hold, so this is the value
	// the caller acts on either way.
	return r, nil
}

// assignmentParent is which record an assignment hangs on, and nothing else.
type assignmentParent struct {
	recordType crmcontracts.AssignmentRecordType
	recordID   ids.UUID
}

// parentOfAssignment reads the parent arms alone, without a lock, so the caller
// can hold the PARENT before locking anything of its own.
func parentOfAssignment(ctx context.Context, tx pgx.Tx, id ids.UUID) (assignmentParent, error) {
	var company, deal, project *ids.UUID
	err := tx.QueryRow(ctx,
		`SELECT company_id, deal_id, project_id FROM record_assignment WHERE id = $1`, id).
		Scan(&company, &deal, &project)
	if errors.Is(err, pgx.ErrNoRows) {
		return assignmentParent{}, apperrors.ErrNotFound
	}
	if err != nil {
		return assignmentParent{}, fmt.Errorf("read record assignment parent: %w", err)
	}
	rt, recordID, err := parentOf(company, deal, project)
	if err != nil {
		return assignmentParent{}, err
	}
	return assignmentParent{recordType: rt, recordID: recordID}, nil
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
//
// FOR SHARE, not a bare read: the foreign key enforces that the subject EXISTS,
// never that it is still active, so without the lock a deactivation committing
// between this check and the insert would leave somebody newly responsible for
// work on the day they left. A share lock rather than an exclusive one because
// this transaction only needs the row to hold still — it writes nothing to it,
// and two assignments naming the same colleague must not queue behind each other.
func ensureSubjectAssignable(
	ctx context.Context, tx pgx.Tx, kind crmcontracts.AssignmentSubjectKind, id ids.UUID,
) error {
	if kind == crmcontracts.AssignmentSubjectKindTeam {
		var teamArchived *time.Time
		if err := tx.QueryRow(ctx, `SELECT archived_at FROM team WHERE id = $1 FOR SHARE`, id).Scan(&teamArchived); err != nil {
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
		`SELECT status, archived_at FROM app_user WHERE id = $1 FOR SHARE`, id).Scan(&status, &archivedAt); err != nil {
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
			Message: "that colleague is deactivated and cannot take on new responsibilities",
		}
	}
	return nil
}
