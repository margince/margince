// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assignments

// The administered vocabulary of responsibilities (record_role). Same catalog
// shape as deal_acquisition_source and lead_source — stable key, editable
// label, retire rather than delete — plus the two applicability sets that make
// a role mean something: which record kinds it can be held on, and whether a
// user, a team or either may hold it.

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	roleTable       = "record_role"
	roleKeyColumn   = "key"
	roleLabelColumn = "label"
	codeRequired    = "required"
)

// roleColumns is what a role read returns, shared by the list and the single
// read.
const roleColumns = `id, key, label, record_types, assignee_kinds, sort_order, active, system, version, created_at, updated_at`

var roleKeyJunk = regexp.MustCompile(`[^a-z0-9]+`)

// deriveRoleKey slugs a label into a key, so an administrator adding
// "Delivery lead" does not have to invent `delivery_lead` themselves.
func deriveRoleKey(label string) string {
	slug := roleKeyJunk.ReplaceAllString(strings.ToLower(strings.TrimSpace(label)), "_")
	return strings.Trim(slug, "_")
}

// CreateRecordRoleInput is the create call's arguments.
type CreateRecordRoleInput struct {
	Key           string
	Label         string
	RecordTypes   []crmcontracts.AssignmentRecordType
	AssigneeKinds []crmcontracts.AssignmentSubjectKind
	SortOrder     int
}

// UpdateRecordRoleInput is the patch call's arguments; a nil field is one the
// request did not mention and this write leaves alone.
type UpdateRecordRoleInput struct {
	Label         *string
	RecordTypes   *[]crmcontracts.AssignmentRecordType
	AssigneeKinds *[]crmcontracts.AssignmentSubjectKind
	SortOrder     *int
	Active        *bool
}

// ListRecordRoles answers with every role, retired ones included. Retired rows
// stay in the answer because an assignment made under one still has to render
// its label; the UI filters them out of the picker, which is a different job.
func (s *Store) ListRecordRoles(ctx context.Context) ([]crmcontracts.RecordRole, error) {
	if err := auth.Require(ctx, assignmentVocabularyObject, principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.RecordRole
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+roleColumns+` FROM record_role ORDER BY sort_order, label`)
		if err != nil {
			return fmt.Errorf("list record roles: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			role, err := scanRole(rows)
			if err != nil {
				return err
			}
			out = append(out, role)
		}
		return rows.Err()
	})
	return out, err
}

// CreateRecordRole adds a responsibility to the vocabulary.
func (s *Store) CreateRecordRole(
	ctx context.Context, in CreateRecordRoleInput,
) (crmcontracts.RecordRole, error) {
	if err := auth.Require(ctx, assignmentVocabularyObject, principal.ActionCreate); err != nil {
		return crmcontracts.RecordRole{}, err
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		return crmcontracts.RecordRole{}, &values.ParseError{
			Field: roleLabelColumn, Code: codeRequired, Message: "label is required",
		}
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		key = deriveRoleKey(label)
	}
	if key == "" || key != strings.ToLower(key) {
		return crmcontracts.RecordRole{}, &values.ParseError{
			Field:   roleKeyColumn,
			Code:    "invalid_key",
			Message: "key must be a non-empty lowercase value",
		}
	}
	recordTypes, err := validRecordTypes(in.RecordTypes)
	if err != nil {
		return crmcontracts.RecordRole{}, err
	}
	assigneeKinds, err := validAssigneeKinds(in.AssigneeKinds)
	if err != nil {
		return crmcontracts.RecordRole{}, err
	}
	var out crmcontracts.RecordRole
	err = s.tx(ctx, func(tx pgx.Tx) error {
		id := ids.NewV7()
		_, err := tx.Exec(ctx,
			`INSERT INTO record_role (id, key, label, record_types, assignee_kinds, sort_order)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			id, key, label, recordTypes, assigneeKinds, in.SortOrder)
		if storekit.IsUniqueViolation(err) {
			return apperrors.ErrConflict
		}
		if err != nil {
			return fmt.Errorf("insert record role: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "create", "record_role", id, nil,
			map[string]any{roleKeyColumn: key, roleLabelColumn: label}); err != nil {
			return err
		}
		out, err = readRole(ctx, tx, id)
		return err
	})
	return out, err
}

// UpdateRecordRole relabels, reorders, re-scopes or retires a role.
//
// Narrowing either applicability set is refused while a live assignment depends
// on what would be removed. Allowing it would leave running assignments the
// role no longer permits — rows that pass no validation anyone could re-run,
// and that the UI would have to render as valid anyway. Retire and replace is
// the supported way to change what a role means.
func (s *Store) UpdateRecordRole(
	ctx context.Context, id ids.UUID, in UpdateRecordRoleInput,
) (crmcontracts.RecordRole, error) {
	if err := auth.Require(ctx, assignmentVocabularyObject, principal.ActionUpdate); err != nil {
		return crmcontracts.RecordRole{}, err
	}
	recordTypes, assigneeKinds, err := checkRoleUpdate(in)
	if err != nil {
		return crmcontracts.RecordRole{}, err
	}
	var out crmcontracts.RecordRole
	err = s.tx(ctx, func(tx pgx.Tx) error {
		before, err := readRoleForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		patch := storekit.NewPatch()
		if in.Label != nil {
			patch.Set(roleLabelColumn, before.Label, strings.TrimSpace(*in.Label))
		}
		if in.RecordTypes != nil {
			if err := refuseNarrowingInUse(ctx, tx, id, recordTypeColumnOf, recordTypes); err != nil {
				return err
			}
			patch.Set("record_types", recordTypeStrings(before.RecordTypes), recordTypes)
		}
		if in.AssigneeKinds != nil {
			if err := refuseNarrowingInUse(ctx, tx, id, assigneeKindColumnOf, assigneeKinds); err != nil {
				return err
			}
			patch.Set("assignee_kinds", assigneeKindStrings(before.AssigneeKinds), assigneeKinds)
		}
		if in.SortOrder != nil {
			patch.Set("sort_order", before.SortOrder, *in.SortOrder)
		}
		if in.Active != nil {
			patch.Set("active", before.Active, *in.Active)
		}
		if patch.Empty() {
			out, err = readRole(ctx, tx, id)
			return err
		}
		if err := patch.ApplyWithVersion(ctx, tx, roleTable, id, before.Version); err != nil {
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "update", "record_role", id,
			patch.Before(), patch.After()); err != nil {
			return err
		}
		out, err = readRole(ctx, tx, id)
		return err
	})
	return out, err
}

// checkRoleUpdate validates everything about a patch that can be judged before
// a transaction opens, and returns the two applicability sets in the shape the
// column takes. Separate from the write so the write reads as the sequence of
// steps it is rather than as validation with a database call in the middle.
func checkRoleUpdate(in UpdateRecordRoleInput) (recordTypes, assigneeKinds []string, err error) {
	if in.Label != nil && strings.TrimSpace(*in.Label) == "" {
		return nil, nil, &values.ParseError{
			Field: roleLabelColumn, Code: codeRequired, Message: "label cannot be blank",
		}
	}
	if in.RecordTypes != nil {
		if recordTypes, err = validRecordTypes(*in.RecordTypes); err != nil {
			return nil, nil, err
		}
	}
	if in.AssigneeKinds != nil {
		if assigneeKinds, err = validAssigneeKinds(*in.AssigneeKinds); err != nil {
			return nil, nil, err
		}
	}
	return recordTypes, assigneeKinds, nil
}
