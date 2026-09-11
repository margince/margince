// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assignments

// Making, moving and ending a responsibility.
//
// Every verb here takes its authority from the PARENT record, never from a
// permission of this module's own: being allowed to edit a deal is what being
// allowed to staff it means. The role is locked while it is validated, so a
// retirement committing mid-write cannot slip a withdrawn role through.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const assignmentTable = "record_assignment"

// CreateAssignmentInput is the create call's arguments. The parent is a
// (type, id) pair from the route; the store resolves it to the typed FK arm.
type CreateAssignmentInput struct {
	RecordType  crmcontracts.AssignmentRecordType
	RecordID    ids.UUID
	SubjectKind crmcontracts.AssignmentSubjectKind
	SubjectID   ids.UUID
	RoleID      ids.UUID
}

// UpdateAssignmentInput is the reassign call's arguments; a nil field is one
// the request did not mention. The parent is deliberately absent: an assignment
// never moves between records, because moving one is indistinguishable from
// ending it here and starting it there, and the latter keeps the history.
type UpdateAssignmentInput struct {
	SubjectKind *crmcontracts.AssignmentSubjectKind
	SubjectID   *ids.UUID
	RoleID      *ids.UUID
}

// CreateAssignment makes a person or team responsible for a record.
func (s *Store) CreateAssignment(
	ctx context.Context, in CreateAssignmentInput,
) (crmcontracts.RecordAssignment, error) {
	// captured_by comes from the authenticated principal, never from the request
	// body: who staffed a record is a fact about the session, and a caller who
	// could name someone else as the actor could forge that history.
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.RecordAssignment{}, err
	}
	var out crmcontracts.RecordAssignment
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// Authority first, and on the parent: a caller who cannot write the
		// record learns nothing about it, not even whether it exists.
		if err := ensureParentWritable(ctx, tx, in.RecordType, in.RecordID); err != nil {
			return err
		}
		if err := ensureAssignableRole(ctx, tx, in.RoleID, in.RecordType, in.SubjectKind); err != nil {
			return err
		}
		if err := ensureSubjectAssignable(ctx, tx, in.SubjectKind, in.SubjectID); err != nil {
			return err
		}
		id := ids.NewV7()
		company, deal, project := parentArms(in.RecordType, in.RecordID)
		user, team := subjectArms(in.SubjectKind, in.SubjectID)
		_, err = tx.Exec(ctx,
			`INSERT INTO record_assignment
			   (id, company_id, deal_id, project_id, user_id, team_id, role_id, source, captured_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			id, company, deal, project, user, team, in.RoleID, sourceHuman, by)
		if storekit.IsUniqueViolation(err) {
			// The live-uniqueness index fired: this subject already holds this
			// role on this record. Saying "already there" is the honest answer.
			return apperrors.ErrConflict
		}
		if err != nil {
			return fmt.Errorf("insert record assignment: %w", err)
		}
		_, err = storekit.Audit(ctx, tx, "create", "record_assignment", id, nil,
			map[string]any{
				"record_type":  string(in.RecordType),
				"record_id":    in.RecordID.String(),
				"subject_kind": string(in.SubjectKind),
				"subject_id":   in.SubjectID.String(),
				"role_id":      in.RoleID.String(),
			})
		if err != nil {
			return err
		}
		out, err = readAssignment(ctx, tx, id)
		return err
	})
	return out, err
}

// UpdateAssignment hands a responsibility to someone else, or changes its role.
func (s *Store) UpdateAssignment(
	ctx context.Context, id ids.UUID, in UpdateAssignmentInput,
) (crmcontracts.RecordAssignment, error) {
	var out crmcontracts.RecordAssignment
	err := s.tx(ctx, func(tx pgx.Tx) error {
		before, err := lockAssignment(ctx, tx, id)
		if err != nil {
			return err
		}
		// lockAssignment already checked write authority against the STORED
		// parent, which is what stops a caller naming a record they may write to
		// reach an assignment on one they may not.
		kind, subject := before.SubjectKind, before.SubjectID
		if in.SubjectKind != nil {
			kind = *in.SubjectKind
		}
		if in.SubjectID != nil {
			subject = *in.SubjectID
		}
		role := before.RoleID
		if in.RoleID != nil {
			role = *in.RoleID
		}
		if err := ensureAssignableRole(ctx, tx, role, before.RecordType, kind); err != nil {
			return err
		}
		if err := ensureSubjectAssignable(ctx, tx, kind, subject); err != nil {
			return err
		}
		user, team := subjectArms(kind, subject)
		patch := storekit.NewPatch()
		beforeUser, beforeTeam := subjectArms(before.SubjectKind, before.SubjectID)
		patch.Set("user_id", beforeUser, user)
		patch.Set("team_id", beforeTeam, team)
		patch.Set("role_id", before.RoleID, role)
		if patch.Empty() {
			out, err = readAssignment(ctx, tx, id)
			return err
		}
		if err := patch.ApplyWithVersion(ctx, tx, assignmentTable, id, before.Version); err != nil {
			// The live-uniqueness index fires when the reassignment would put a
			// subject where that subject already holds this role. "Already
			// there" is a conflict the caller can act on, not a server fault.
			if storekit.IsUniqueViolation(err) {
				return apperrors.ErrConflict
			}
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "update", "record_assignment", id,
			patch.Before(), patch.After()); err != nil {
			return err
		}
		out, err = readAssignment(ctx, tx, id)
		return err
	})
	return out, err
}

// ArchiveAssignment ends a responsibility, keeping it in history. There is no
// delete: who was responsible when is exactly the question an archived row
// exists to answer.
func (s *Store) ArchiveAssignment(ctx context.Context, id ids.UUID) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		before, err := lockAssignment(ctx, tx, id)
		if err != nil {
			return err
		}
		// Already archived is a conflict, not a second archive: the row is
		// history by then, and re-stamping it would move the moment
		// responsibility ended to whenever somebody pressed the button twice.
		if before.ArchivedAt != nil {
			return apperrors.ErrConflict
		}
		patch := storekit.NewPatch()
		patch.Set("archived_at", nil, time.Now())
		if err := patch.ApplyWithVersion(ctx, tx, assignmentTable, id, before.Version); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "archive", "record_assignment", id,
			patch.Before(), patch.After())
		return err
	})
}

// parentArms turns the logical parent into the three typed FK columns, exactly
// one of which is non-null.
func parentArms(rt crmcontracts.AssignmentRecordType, id ids.UUID) (company, deal, project *ids.UUID) {
	switch rt {
	case crmcontracts.AssignmentRecordTypeCompany:
		return &id, nil, nil
	case crmcontracts.AssignmentRecordTypeDeal:
		return nil, &id, nil
	default:
		return nil, nil, &id
	}
}

// subjectArms does the same for the assignee.
func subjectArms(kind crmcontracts.AssignmentSubjectKind, id ids.UUID) (user, team *ids.UUID) {
	if kind == crmcontracts.AssignmentSubjectKindTeam {
		return nil, &id
	}
	return &id, nil
}

// ensureAssignableRole refuses a role that is retired, or that does not apply
// to this record type or this kind of assignee.
//
// The row is LOCKED, not merely read: a retirement committing between this
// check and the insert would otherwise admit a role the administrator had just
// withdrawn — true when it ran, false by the time it mattered.
func ensureAssignableRole(
	ctx context.Context, tx pgx.Tx,
	roleID ids.UUID, rt crmcontracts.AssignmentRecordType, kind crmcontracts.AssignmentSubjectKind,
) error {
	var active bool
	var recordTypes, assigneeKinds []string
	err := tx.QueryRow(ctx,
		`SELECT active, record_types, assignee_kinds FROM record_role WHERE id = $1 FOR UPDATE`,
		roleID).Scan(&active, &recordTypes, &assigneeKinds)
	if err != nil {
		return roleLookupError(err)
	}
	if !active {
		return &values.ParseError{
			Field: fieldRoleID, Code: "retired_role",
			Message: "that role has been retired and cannot be newly assigned",
		}
	}
	if !contains(recordTypes, string(rt)) {
		return &values.ParseError{
			Field: fieldRoleID, Code: "role_not_applicable",
			Message: fmt.Sprintf("that role cannot be held on a %s", rt),
		}
	}
	if !contains(assigneeKinds, string(kind)) {
		return &values.ParseError{
			Field: fieldRoleID, Code: "role_wrong_assignee_kind",
			Message: fmt.Sprintf("that role cannot be held by a %s", kind),
		}
	}
	return nil
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}
