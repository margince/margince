// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assignments

// Reading, validating and scanning roles. Separated from the write verbs in
// roles.go so neither file grows past the length cap as the vocabulary does.

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

// recordTypeColumnOf and assigneeKindColumnOf name the assignment column each
// applicability set governs. refuseNarrowingInUse takes one of them so the two
// checks are the same code rather than two near-copies that can drift.
const (
	recordTypeColumnOf   = "record_type"
	assigneeKindColumnOf = "assignee_kind"
)

// validRecordTypes checks the set is non-empty and every member is a record
// type this system knows. The DB CHECK enforces the same thing; this exists so
// a bad request answers 422 with the offending field rather than 500 with a
// constraint name.
func validRecordTypes(in []crmcontracts.AssignmentRecordType) ([]string, error) {
	if len(in) == 0 {
		return nil, &values.ParseError{
			Field: "record_types", Code: codeRequired,
			Message: "a role must apply to at least one record type",
		}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, rt := range in {
		if !rt.Valid() {
			return nil, &values.ParseError{
				Field: "record_types", Code: "invalid_record_type",
				Message: "record_types are company, deal, project",
			}
		}
		if seen[string(rt)] {
			continue
		}
		seen[string(rt)] = true
		out = append(out, string(rt))
	}
	return out, nil
}

// validAssigneeKinds is its twin for who may hold the role.
func validAssigneeKinds(in []crmcontracts.AssignmentSubjectKind) ([]string, error) {
	if len(in) == 0 {
		return nil, &values.ParseError{
			Field: "assignee_kinds", Code: codeRequired,
			Message: "a role must be holdable by at least one of user, team",
		}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, k := range in {
		if !k.Valid() {
			return nil, &values.ParseError{
				Field: "assignee_kinds", Code: "invalid_assignee_kind",
				Message: "assignee_kinds are user, team",
			}
		}
		if seen[string(k)] {
			continue
		}
		seen[string(k)] = true
		out = append(out, string(k))
	}
	return out, nil
}

func recordTypeStrings(in []crmcontracts.AssignmentRecordType) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, string(v))
	}
	return out
}

func assigneeKindStrings(in []crmcontracts.AssignmentSubjectKind) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, string(v))
	}
	return out
}

// refuseNarrowingInUse blocks a re-scope that would strand a live assignment.
//
// The question it asks is the one that matters: after this edit, would any
// assignment still standing be one this role no longer permits? If so the edit
// is refused with a conflict, because the alternative is rows that validate
// today and cannot be re-validated tomorrow. Widening always passes.
func refuseNarrowingInUse(ctx context.Context, tx pgx.Tx, roleID ids.UUID, column string, allowed []string) error {
	// The assignment's own record type and assignee kind are derived from which
	// typed FK arm is populated, so the comparison happens in SQL against the
	// same CASE the reads use.
	var expr string
	switch column {
	case recordTypeColumnOf:
		expr = `CASE WHEN company_id IS NOT NULL THEN 'company'
		             WHEN deal_id    IS NOT NULL THEN 'deal'
		             ELSE 'project' END`
	case assigneeKindColumnOf:
		expr = `CASE WHEN user_id IS NOT NULL THEN 'user' ELSE 'team' END`
	default:
		return fmt.Errorf("refuseNarrowingInUse: unknown column %q", column)
	}
	var stranded int
	err := tx.QueryRow(ctx,
		`SELECT count(*) FROM record_assignment
		  WHERE role_id = $1 AND archived_at IS NULL AND NOT (`+expr+` = ANY($2))`,
		roleID, allowed).Scan(&stranded)
	if err != nil {
		return fmt.Errorf("count stranded assignments: %w", err)
	}
	if stranded > 0 {
		return apperrors.ErrConflict
	}
	return nil
}

// roleRow is the stored shape, before it becomes the wire shape.
type roleRow struct {
	ID            ids.UUID
	Key           string
	Label         string
	RecordTypes   []crmcontracts.AssignmentRecordType
	AssigneeKinds []crmcontracts.AssignmentSubjectKind
	SortOrder     int
	Active        bool
	System        bool
	Version       int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type rowScanner interface{ Scan(dest ...any) error }

func scanRoleRow(sc rowScanner) (roleRow, error) {
	var r roleRow
	var recordTypes, assigneeKinds []string
	if err := sc.Scan(&r.ID, &r.Key, &r.Label, &recordTypes, &assigneeKinds,
		&r.SortOrder, &r.Active, &r.System, &r.Version, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return roleRow{}, err
	}
	for _, v := range recordTypes {
		r.RecordTypes = append(r.RecordTypes, crmcontracts.AssignmentRecordType(v))
	}
	for _, v := range assigneeKinds {
		r.AssigneeKinds = append(r.AssigneeKinds, crmcontracts.AssignmentSubjectKind(v))
	}
	return r, nil
}

func (r roleRow) wire() crmcontracts.RecordRole {
	system, version := r.System, r.Version
	created, updated := r.CreatedAt, r.UpdatedAt
	return crmcontracts.RecordRole{
		Id:            openapi_types.UUID(r.ID),
		Key:           r.Key,
		Label:         r.Label,
		RecordTypes:   r.RecordTypes,
		AssigneeKinds: r.AssigneeKinds,
		SortOrder:     r.SortOrder,
		Active:        r.Active,
		System:        &system,
		Version:       &version,
		CreatedAt:     &created,
		UpdatedAt:     &updated,
	}
}

func scanRole(sc rowScanner) (crmcontracts.RecordRole, error) {
	r, err := scanRoleRow(sc)
	if err != nil {
		return crmcontracts.RecordRole{}, fmt.Errorf("scan record role: %w", err)
	}
	return r.wire(), nil
}

func readRole(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.RecordRole, error) {
	row := tx.QueryRow(ctx, `SELECT `+roleColumns+` FROM record_role WHERE id = $1`, id)
	r, err := scanRoleRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.RecordRole{}, apperrors.ErrNotFound
	}
	if err != nil {
		return crmcontracts.RecordRole{}, fmt.Errorf("read record role: %w", err)
	}
	return r.wire(), nil
}

// readRoleForUpdate takes the row lock the patch's version compare relies on,
// so two administrators editing the same role take turns rather than one
// silently overwriting the other.
func readRoleForUpdate(ctx context.Context, tx pgx.Tx, id ids.UUID) (roleRow, error) {
	row := tx.QueryRow(ctx, `SELECT `+roleColumns+` FROM record_role WHERE id = $1 FOR UPDATE`, id)
	r, err := scanRoleRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return roleRow{}, apperrors.ErrNotFound
	}
	if err != nil {
		return roleRow{}, fmt.Errorf("lock record role: %w", err)
	}
	return r, nil
}
