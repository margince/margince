// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// leadMailDataTask files mail and meetings captured before capture knew leads
// under the live leads their participants' addresses name. It decides as the
// mailbox that captured each activity, by capture's own rule, and asks nobody.
func leadMailDataTask(pool *pgxpool.Pool) datatask.Task {
	db := InstallationDB(pool)
	tasks := newLeadWorkTasks(db, slog.Default())
	repair := capture.LeadFilingRepair{Authority: identity.NewService(pool), Filer: ActivityLeadFiler{Tasks: &tasks}}
	return datatask.Task{
		ID: "core/1792859022_captured_mail_filed_under_leads", Version: 1,
		Title: "Filing captured mail and meetings under their leads", Requires: []string{introductionValidationMigration},
		Steps: []datatask.Step{{
			Name: "file_captured_activity_under_lead", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: capture.EstimateLeadFiling,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				// The capturing seat's authority is resolved per workspace,
				// which the executor's system context does not carry.
				if _, bound := principal.WorkspaceID(ctx); !bound {
					ws, err := db.Workspace(ctx)
					if err != nil {
						return datatask.BatchOut{}, err
					}
					ctx = principal.WithWorkspaceID(ctx, ws.UUID)
				}
				return repair.BatchTx(ctx, tx, in)
			},
		}},
	}
}

// ActivityLeadFiler is capture.LeadFiler over the activities module, which
// owns activity_link and the relink event.
type ActivityLeadFiler struct {
	Tasks *leadWorkTasks
}

// FileOnLead is activities.FileOnLeadTx.
func (f ActivityLeadFiler) FileOnLead(ctx context.Context, tx pgx.Tx, id ids.ActivityID, lead ids.UUID, evidence map[string]any) (bool, error) {
	written, err := activities.FileOnLeadTx(ctx, tx, id, lead, evidence)
	if err != nil || !written || f.Tasks == nil {
		return written, err
	}
	var inbound bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE(direction = 'inbound', false) FROM activity WHERE id = $1 AND archived_at IS NULL`, id).Scan(&inbound); err != nil {
		return false, err
	}
	if !inbound {
		return true, nil
	}
	err = f.Tasks.FileLeadReplyTx(ctx, tx, ids.From[ids.LeadKind](lead), id.UUID)
	return err == nil, err
}
