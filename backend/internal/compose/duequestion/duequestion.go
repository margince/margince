// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package duequestion asks the owner of a captured task about a due date far
// outside the allowed window, and writes the answer.
package duequestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dueday"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Kind is the approval kind of the question.
const Kind = approvalsubject.KindTaskDueDate

const (
	actor        = "agent:" + Kind
	summaryCode  = "approval.taskDueAsked"
	targetType   = "activity"
	identityTask = "task_id"
)

// Asker stages the question for the task's owner.
type Asker struct{ Approvals *approvals.Service }

// AskDueTx stages one question per task. A repeat joins the open one, and a
// question the owner declined is not asked again.
func (a Asker) AskDueTx(ctx context.Context, tx pgx.Tx, q capture.DueQuestion) error {
	day, err := dueday.DayOf(ctx, tx, q.Due)
	if err != nil {
		return err
	}
	body, err := json.Marshal(approvalsubject.TaskDueDate{
		TaskID: q.TaskID.UUID, OwnerID: q.OwnerID, Summary: q.Summary, DueDate: day, OddDueDate: day,
	})
	if err != nil {
		return fmt.Errorf("duequestion: encoding the question: %w", err)
	}
	identity, err := json.Marshal(map[string]string{identityTask: q.TaskID.String()})
	if err != nil {
		return fmt.Errorf("duequestion: encoding the identity: %w", err)
	}
	digest := sha256.Sum256(body)
	_, _, err = a.Approvals.StageUnlessDeclinedTx(principal.SystemActing(ctx, actor), tx, approvals.StageInput{
		Why:            approvals.HeadlineSaysWhy,
		Kind:           Kind,
		ProposedChange: body,
		DiffHash:       hex.EncodeToString(digest[:]),
		TargetType:     targetType,
		TargetID:       q.TaskID.UUID,
		Summary:        fmt.Sprintf(systemtext.Copy(summaryCode, textlang.English), q.Summary),
		SystemText:     &systemtext.Message{Version: 1, Subject: systemtext.Sentence(summaryCode, systemtext.Text(q.Summary))},
		JoinPending:    true,
		Identity:       identity,
	})
	if errors.Is(err, approvals.ErrNoDecider) {
		return nil
	}
	return err
}

// staleReason tells the decider why an answer wrote nothing.
const staleReason = "This task changed since the question was asked, so nothing was written."

// Precheck refuses an edited answer that reaches past the day. The task, owner,
// wording and original day stay as staged. The day may be cleared, kept, or
// replaced by a day inside the window.
func Precheck(db *database.DB, now func() time.Time) approvals.ReleasePrecheck {
	return func(ctx context.Context, staged, edited json.RawMessage) error {
		if len(edited) == 0 {
			return nil
		}
		var before, after approvalsubject.TaskDueDate
		if err := json.Unmarshal(staged, &before); err != nil {
			return fmt.Errorf("duequestion: decoding the staged question: %w", err)
		}
		if err := json.Unmarshal(edited, &after); err != nil {
			return &approvals.InvalidEditError{Cause: err}
		}
		pinned := after
		pinned.DueDate = before.DueDate
		if pinned != before {
			return &approvals.InvalidEditError{Cause: errors.New("only the due date may be edited")}
		}
		if after.DueDate == "" || after.DueDate == before.OddDueDate {
			return nil
		}
		return db.Tx(ctx, func(tx pgx.Tx) error {
			odd, err := dueday.Odd(ctx, tx, after.DueDate, "", now())
			if err != nil {
				return &approvals.InvalidEditError{Cause: err}
			}
			if odd != "" {
				return &approvals.InvalidEditError{Cause: errors.New("that date is still too far off: pick a nearer one or clear it")}
			}
			return nil
		})
	}
}

// Effect writes the owner's answer onto the task as the owner: the kept day,
// another day, or none when the day was cleared.
func Effect(svc *approvals.Service, tasks *activities.Store) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		var answer approvalsubject.TaskDueDate
		if err := json.Unmarshal(proposedChange, &answer); err != nil {
			return fmt.Errorf("duequestion: decoding the question: %w", err)
		}
		stale := false
		err := svc.RedeemAndApply(ctx, approvalID, Kind, diffHash, func(tx pgx.Tx) error {
			var due *time.Time
			if answer.DueDate != "" {
				end, err := dueday.EndOf(ctx, tx, answer.DueDate, "")
				if err != nil {
					return err
				}
				due = &end
			}
			kept := answer.DueDate != "" && answer.DueDate == answer.OddDueDate
			applied, err := tasks.AnswerDueTx(ctx, tx, ids.From[ids.ActivityKind](answer.TaskID), answer.OwnerID, due, kept)
			stale = !applied
			return err
		})
		if err == nil && stale {
			return &approvals.ObsoleteError{Reason: staleReason}
		}
		return err
	}
}
