// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package tasktaken tells the owner of a record that a colleague took an
// unassigned task filed under it. Activities cannot import notices, so compose
// hands the take its notice as a hook that runs in the take's transaction.
package tasktaken

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/recordowner"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Hook writes one notice per distinct owner of the records the task is filed
// under, apart from the taker. A record with no owner, or whose owner can no
// longer hold work, tells nobody: automatic work has no fallback owner.
func Hook() activities.TaskTakenHook {
	store := notices.NewStore(nil)
	return func(ctx context.Context, tx pgx.Tx, taken activities.TaskTaken) error {
		if taken.Task.Links == nil || taken.Task.Version == nil {
			return nil
		}
		taker, err := recordowner.ColleagueNameTx(ctx, tx, taken.Taker.UUID)
		if err != nil {
			return err
		}
		told := map[ids.UUID]bool{taken.Taker.UUID: true}
		for _, link := range *taken.Task.Links {
			recordType := string(link.EntityType)
			if !recordowner.Known(recordType) {
				continue
			}
			owner, name, err := recordowner.ReadTx(ctx, tx, recordType, ids.UUID(link.EntityId))
			if errors.Is(err, apperrors.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if owner == nil || told[*owner] {
				continue
			}
			told[*owner] = true
			err = auth.EnsureAssignee(ctx, tx, *owner)
			if errors.As(err, new(*auth.AssigneeNotAllowedError)) {
				continue
			}
			if err != nil {
				return err
			}
			if err := tell(ctx, tx, store, taken, *owner, notice{recordType: recordType, record: ids.UUID(link.EntityId), recordName: name, taker: taker}); err != nil {
				return err
			}
		}
		return nil
	}
}

type notice struct {
	recordType, recordName, taker string
	record                        ids.UUID
}

func tell(ctx context.Context, tx pgx.Tx, store *notices.Store, taken activities.TaskTaken, owner ids.UUID, n notice) error {
	subject := systemtext.Sentence("notice.task_taken.subject", systemtext.Text(n.taker), systemtext.Text(n.recordName))
	body := systemtext.Sentence("notice.task_taken.body")
	_, err := store.CreateTx(ctx, tx, notices.NewNotice{
		Recipient: ids.From[ids.UserKind](owner), Kind: notices.KindTaskTaken,
		Subject: systemtext.Canonical(subject, textlang.English), Body: systemtext.Canonical(body, textlang.English),
		SystemText: &systemtext.Message{Version: 1, Subject: subject, Body: body},
		DedupeKey:  fmt.Sprintf("task-taken:%s:%d", taken.Task.Id, *taken.Task.Version),
		Target:     notices.Target{Type: n.recordType, ID: n.record},
	})
	if err != nil {
		return fmt.Errorf("telling the owner of a %s that a task on it was taken: %w", n.recordType, err)
	}
	return nil
}
