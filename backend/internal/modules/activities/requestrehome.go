// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	answererChangedReason      = "answerer_changed"
	answererChangedRuleVersion = "1"
)

// rehomedReminder is one reminder whose request another seat, or none, now
// answers.
type rehomedReminder struct {
	id, from ids.UUID
	version  int64
	to       *ids.UUID
	selected bool
}

// rehomeSharedReminders follows a change of who answers shared-mailbox mail:
// a contact or company changed owner, or a seat's own mailbox captured it.
// An untouched reminder this pass filed moves to the seat that answers now.
// One nobody answers any more, or one narrowed to its old holder, is retired.
// A reminder a human edited, completed or archived is left as it is.
func (s *Store) rehomeSharedReminders(ctx context.Context, tx pgx.Tx, asOf time.Time) error {
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	identities, err := s.mailSeatIdentitiesSQL(ctx, tx, arg)
	if err != nil {
		return err
	}
	answerer := fmt.Sprintf(emailRequestAssigneeSQL(identities), auth.AssigneeEligibleSQL("u"))
	query := `SELECT reminder.id, reminder.version, reminder.assignee_id, (` + answerer + `), reminder.audience = 'selected'
	  FROM activity reminder JOIN activity a ON a.id = reminder.source_activity_id
	 WHERE reminder.source_system = $` + fmt.Sprint(arg(EmailRequestTaskSource)) + `
	   AND reminder.captured_by = $` + fmt.Sprint(arg(OwedVerdictCapturedBy)) + `
	   AND ` + pristineRepairTaskSQL("reminder") + `
	   AND EXISTS (SELECT 1 FROM capture_import shared_import WHERE shared_import.activity_id = a.id AND shared_import.user_id IS NULL)
	   AND reminder.assignee_id IS DISTINCT FROM (` + answerer + `)
	 ORDER BY reminder.id LIMIT $` + fmt.Sprint(arg(agedReminderBatch)) + ` FOR UPDATE OF reminder SKIP LOCKED`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("activities: reading reminders whose request changed hands: %w", err)
	}
	reminders, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (rehomedReminder, error) {
		var r rehomedReminder
		return r, row.Scan(&r.id, &r.version, &r.from, &r.to, &r.selected)
	})
	if err != nil {
		return fmt.Errorf("activities: reading reminders whose request changed hands: %w", err)
	}
	for _, r := range reminders {
		if err := r.rehomeTx(ctx, tx, asOf); err != nil {
			return fmt.Errorf("activities: moving the reminder %s to its new holder: %w", r.id, err)
		}
	}
	return nil
}

func (r rehomedReminder) rehomeTx(ctx context.Context, tx pgx.Tx, asOf time.Time) error {
	if r.to == nil || r.selected {
		opts := collectArchiveOptions([]ArchiveOption{OnlyPristineAutomaticTask(), WithArchiveReason(answererChangedReason, answererChangedRuleVersion)})
		_, err := archiveActivityInTx(ctx, tx, ids.From[ids.ActivityKind](r.id), nil, opts, asOf)
		return err
	}
	to := ids.From[ids.UserKind](*r.to)
	if err := ensureAssigneeCanHoldWork(ctx, tx, &to); err != nil {
		return err
	}
	patch := storekit.NewPatch()
	patch.Set("assignee_id", r.from, *r.to)
	if patch.Empty() {
		return nil
	}
	version := r.version
	if err := patch.ApplyGuarded(ctx, tx, linkEntityActivity, r.id, &version); err != nil {
		return err
	}
	audit, err := storekit.AuditWithEvidence(ctx, tx, "update", linkEntityActivity, r.id, patch.Before(), patch.After(),
		map[string]any{"automatic_task_repair": "1", ruleVersionEvidenceKey: answererChangedRuleVersion, evidenceReasonKey: answererChangedReason})
	if err != nil {
		return err
	}
	assignee := openapi_types.UUID(*r.to)
	return storekit.EmitEvent(ctx, tx, audit, r.id, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{AssigneeId: &assignee, AutomaticTaskRepaired: new(true)},
	})
}
