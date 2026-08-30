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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

		p, err := projectUpdatePatch(current, in)
		if err != nil {
			return err
		}
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
// The date columns a project carries. Each is read by the patch builder, the
// clearable set and the row read, and a literal repeated across the three is
// how the three come to disagree about which column they mean.
const (
	startedAtColumn     = "started_at"
	targetEndDateColumn = "target_end_date"
	endedAtColumn       = "ended_at"
)

func projectUpdatePatch(current crmcontracts.Project, in UpdateProjectInput) (*storekit.Patch, error) {
	p := storekit.NewPatch()
	if err := storekit.ApplyClears(p, in.Clear, clearableProjectColumns(current)); err != nil {
		return nil, err
	}
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
		p.SetDate(startedAtColumn, storekit.PlainDate(current.StartedAt), in.StartedAt)
	}
	if in.TargetEndDate != nil {
		p.SetDate(targetEndDateColumn, storekit.PlainDate(current.TargetEndDate), in.TargetEndDate)
	}
	if in.EndedAt != nil {
		p.SetDate(endedAtColumn, storekit.PlainDate(current.EndedAt), in.EndedAt)
	}
	return p, nil
}

// clearableProjectColumns names the wire fields a project restore may set to
// NULL, against the column holding each.
//
// The KEY is a wire field name and the VALUE is a column: two vocabularies that
// spell the same words here by coincidence. The keys stay literals because
// clearablefields_test.go reads them as the declaration of what this store can
// clear — a constant in their place is invisible to it, and the census then
// under-reports rather than failing.
//
//nolint:goconst // wire field names against column names, each its own vocabulary — see clearablePersonColumns in the people module
func clearableProjectColumns(current crmcontracts.Project) map[string]storekit.Clearable {
	return map[string]storekit.Clearable{
		"description":     {Column: "description", Current: current.Description},
		"owner_id":        {Column: "owner_id", Current: current.OwnerId},
		"started_at":      {Column: startedAtColumn, Current: current.StartedAt},
		"target_end_date": {Column: targetEndDateColumn, Current: current.TargetEndDate},
		"ended_at":        {Column: endedAtColumn, Current: current.EndedAt},
	}
}
