// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

var legacyTaskTextScope = `a.kind='task' AND a.archived_at IS NULL AND a.restricted_at IS NULL AND a.system_text IS NULL
 AND a.source_system=ANY(@sources) AND NOT EXISTS(SELECT 1 FROM audit_log edited
 WHERE edited.entity_type='activity' AND edited.entity_id=a.id AND edited.actor_type IN ('human','agent')
 AND edited.action='update' AND (edited.before->'subject' IS DISTINCT FROM edited.after->'subject'
 OR edited.before->'body' IS DISTINCT FROM edited.after->'body')) AND ` + SystemMintedExpr("a")

// LegacySystemTask carries the version guard for descriptor-only repairs.
type LegacySystemTask struct {
	ID                    ids.UUID
	Source, Subject, Body string
	Version               int64
}

// CountLegacyTaskText bounds estimates to eligible native tasks.
func CountLegacyTaskText(ctx context.Context, q datatask.Querier, maximum int64, sources []string) (int64, bool, error) {
	var n int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT a.id FROM activity a WHERE `+legacyTaskTextScope+` LIMIT @limit) candidates`, pgx.NamedArgs{"sources": sources, argLimit: maximum + 1}).Scan(&n)
	return min(n, maximum), n > maximum, err
}

// LegacySystemTasksTx locks a bounded page without admitting edited titles.
func LegacySystemTasksTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, sources []string) ([]LegacySystemTask, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id,a.source_system,coalesce(a.subject,''),coalesce(a.body,''),a.version
 FROM activity a WHERE `+legacyTaskTextScope+` AND a.id>@after ORDER BY a.id LIMIT @limit FOR UPDATE OF a`, pgx.NamedArgs{"sources": sources, repairAfterArg: after, argLimit: limit})
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[LegacySystemTask])
}

// RestoreTaskSystemTextTx preserves domain wording and audits descriptor restoration.
func RestoreTaskSystemTextTx(ctx context.Context, tx pgx.Tx, task LegacySystemTask, message *systemtext.Message) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}

	if message == nil {
		return false, nil
	}
	if message.Subject != nil && strings.HasPrefix(message.Subject.Code, "lead.") {
		for i, argument := range message.Subject.Args {
			if argument.Text != systemtext.Literal("lead.unnamed") {
				continue
			}
			var unnamed bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM activity_link link JOIN lead l ON l.id=link.lead_id WHERE link.activity_id=@id AND coalesce(nullif(btrim(l.full_name),''),nullif(btrim(l.email),'')) IS NULL)`, pgx.NamedArgs{"id": task.ID}).Scan(&unnamed); err != nil {
				return false, err
			}
			if unnamed {
				message = systemtext.ForDestination(message)
				message.Subject.Args[i] = systemtext.MessageArgument("lead.unnamed")
			}
		}
	}
	bound, err := systemtext.Bind(message, task.Subject, task.Body)
	if err != nil {
		return false, err
	}
	if bound.Subject == nil && bound.Body == nil {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `UPDATE activity SET system_text=@message
 WHERE id=@id AND version=@version AND system_text IS NULL AND archived_at IS NULL AND restricted_at IS NULL`, pgx.NamedArgs{"id": task.ID, "version": task.Version, "message": bound})
	if err != nil || tag.RowsAffected() == 0 {
		return false, err
	}
	audit, err := storekit.AuditWithEvidence(ctx, tx, "update", "activity", task.ID, map[string]any{"system_text": nil}, map[string]any{"system_text": bound}, map[string]any{automaticTaskRepairEvidence: "1"})
	if err != nil {
		return false, err
	}
	return true, storekit.EmitEvent(ctx, tx, audit, task.ID, crmcontracts.PublicEventActivityUpdated{ChangedFields: crmcontracts.PublicEventActivityChangedFields{AutomaticTaskRepaired: new(true)}})
}
