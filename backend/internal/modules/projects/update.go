// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// The project partial-update path. `phase` is deliberately absent from the
// input: a transition moves through AdvanceProjectPhase so the row change
// and its history row are written from one transaction, and the move emits
// project.phase_changed rather than a diff a consumer has to interpret.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// UpdateProjectInput is one project partial update: every field optional.
type UpdateProjectInput struct {
	// Clear names the wire fields to set to NULL. A JSON null cannot say so —
	// it decodes to a nil pointer and reads as "not supplied" — so the
	// reversal path names them here instead.
	Clear []string
	// Trail names what the audit trail calls this write; zero is an update.
	Trail         storekit.AuditTrail
	Name          *string
	OwnerID       *ids.UserID
	Description   *string
	StartedAt     *time.Time
	TargetEndDate *time.Time
	EndedAt       *time.Time
	IfVersion     *int64
	// CustomFields carries the request body's extra top-level keys
	// (additionalProperties); only active cf_* catalog columns land,
	// drop-on-mismatch (storekit customcolumns).
	CustomFields map[string]any
}

// UpdateProject applies a partial update. It cannot move the phase: that
// would write a history row, and only AdvanceProjectPhase does.
func (s *Store) UpdateProject(ctx context.Context, id ids.ProjectID, in UpdateProjectInput) (crmcontracts.Project, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionUpdate); err != nil {
		return crmcontracts.Project{}, err
	}
	active, err := s.catalogColumns(ctx)
	if err != nil {
		return crmcontracts.Project{}, err
	}
	var out crmcontracts.Project
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, projectObject, id.UUID); err != nil {
			return err
		}
		// current reads WITH active columns so the patch's audit
		// before-image carries the honest pre-update cf values.
		current, err := readProject(ctx, tx, id, storekit.LiveOnly, active)
		if err != nil {
			return fmt.Errorf("read project before update: %w", err)
		}

		p := projectUpdatePatch(current, in)
		storekit.SetCustomFieldPatch(p, active, in.CustomFields, current.AdditionalProperties)
		if p.Empty() {
			out = current
			return nil
		}
		if err := p.ApplyGuarded(ctx, tx, projectObject, id.UUID, in.IfVersion); err != nil {
			if constraint, ok := storekit.CheckViolation(err); ok {
				if refusal := projectCheckError(constraint, submittedDateField(in.StartedAt, in.TargetEndDate, in.EndedAt)); refusal != nil {
					return refusal
				}
			}
			return fmt.Errorf("apply project patch: %w", err)
		}

		auditID, err := storekit.AuditWithTrail(ctx, tx, in.Trail, projectObject, id.UUID, p.Before(), p.After())
		if err != nil {
			return fmt.Errorf("audit project update: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventProjectUpdated{
			ChangedFields: p.After(),
		}); err != nil {
			return fmt.Errorf("emit project.updated: %w", err)
		}
		updated, err := readProject(ctx, tx, id, storekit.LiveOnly, active)
		if err != nil {
			return fmt.Errorf("read updated project: %w", err)
		}
		out, err = s.maskProjectForCaller(ctx, tx, updated)
		return err
	})
	return out, err
}

// projectUpdatePatch builds the column patch. Every FK it can set points
// at app_user, which carries no row scope of its own — any workspace
// member may own a project — so the composite FK is the whole check; the
// anchor company is not re-pointable here, because moving a project to
// another company would silently orphan the deals that inherited it.
func projectUpdatePatch(current crmcontracts.Project, in UpdateProjectInput) *storekit.Patch {
	p := storekit.NewPatch()
	applyClears(p, in.Clear, clearableProjectColumns(current))
	if in.Name != nil {
		p.Set("name", current.Name, *in.Name)
	}
	if in.OwnerID != nil {
		p.Set("owner_id", current.OwnerId, *in.OwnerID)
	}
	if in.Description != nil {
		p.Set("description", current.Description, *in.Description)
	}
	if in.StartedAt != nil {
		p.Set("started_at", current.StartedAt, *in.StartedAt)
	}
	if in.TargetEndDate != nil {
		p.Set("target_end_date", current.TargetEndDate, *in.TargetEndDate)
	}
	if in.EndedAt != nil {
		p.Set("ended_at", current.EndedAt, *in.EndedAt)
	}
	return p
}

// clearable is one column a caller may set to NULL, and what the row holds
// there now. The current value is carried so the audit image says what the
// field was cleared FROM.
//
//craft:ignore naked-any the value is whichever type the column holds; the patch seam takes it as the audit image does
type clearable struct {
	column  string
	current any
}

// applyClears sets each named field to NULL. A name this map does not hold is
// ignored here and refused by the reversal path before the write, so a caller
// cannot reach a column this store did not name.
func applyClears(p *storekit.Patch, clear []string, columns map[string]clearable) {
	for _, field := range clear {
		target, clearableHere := columns[field]
		if !clearableHere {
			continue
		}
		p.Set(target.column, target.current, nil)
	}
}

// clearableProjectColumns names the wire fields a project restore may set to
// NULL, with literal column names.
func clearableProjectColumns(current crmcontracts.Project) map[string]clearable {
	return map[string]clearable{
		"description":     {"description", current.Description},
		"owner_id":        {"owner_id", current.OwnerId},
		"started_at":      {"started_at", current.StartedAt},
		"target_end_date": {"target_end_date", current.TargetEndDate},
		"ended_at":        {"ended_at", current.EndedAt},
	}
}
