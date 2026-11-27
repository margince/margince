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
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// linkedRecords are the records a task is filed under, as the record types
// recordowner reads. The task's own links are read here and not
// taken from the answer to the taker, which leaves out records the taker
// cannot read.
const linkedRecords = `SELECT entity_type, coalesce(contact_id, company_id, deal_id, lead_id, project_id)
	FROM activity_link WHERE activity_id = $1 ORDER BY created_at, id`

type linkedRecord struct {
	recordType string
	id         ids.UUID
}

// Hook writes one notice per distinct owner of the records the task is filed
// under, apart from the taker. Four cases tell nobody.
// The owner has left. The owner can no longer hold work. The record has no
// owner, since automatic work has no fallback owner. The owner is outside the
// task's audience, so the task is not theirs to see.
func Hook() activities.TaskTakenHook {
	store := notices.NewStore(nil)
	return func(ctx context.Context, tx pgx.Tx, taken activities.TaskTaken) error {
		if taken.Task.Version == nil {
			return nil
		}
		records, err := linkedRecordsOf(ctx, tx, ids.UUID(taken.Task.Id))
		if err != nil {
			return err
		}
		taker, err := recordowner.ColleagueNameTx(ctx, tx, taken.Taker.UUID)
		if err != nil {
			return err
		}
		told := map[ids.UUID]bool{taken.Taker.UUID: true}
		for _, record := range records {
			owner, name, err := recordowner.ReadTx(ctx, tx, record.recordType, record.id)
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
			may, err := mayBeTold(ctx, tx, *owner, ids.UUID(taken.Task.Id))
			if err != nil {
				return err
			}
			if !may {
				continue
			}
			if err := tell(ctx, tx, store, taken, *owner, notice{recordType: record.recordType, record: record.id, recordName: name, taker: taker}); err != nil {
				return err
			}
		}
		return nil
	}
}

func linkedRecordsOf(ctx context.Context, tx pgx.Tx, task ids.UUID) ([]linkedRecord, error) {
	rows, err := tx.Query(ctx, linkedRecords, task)
	if err != nil {
		return nil, fmt.Errorf("tasktaken: reading what the task is filed under: %w", err)
	}
	defer rows.Close()
	var out []linkedRecord
	for rows.Next() {
		var record linkedRecord
		if err := rows.Scan(&record.recordType, &record.id); err != nil {
			return nil, fmt.Errorf("tasktaken: reading what the task is filed under: %w", err)
		}
		if recordowner.Known(record.recordType) {
			out = append(out, record)
		}
	}
	return out, rows.Err()
}

// mayBeTold is whether the owner holds a seat that can hold work. It also asks
// the task's content gate as that owner would read it.
func mayBeTold(ctx context.Context, tx pgx.Tx, owner, task ids.UUID) (bool, error) {
	err := auth.EnsureAssignee(ctx, tx, owner)
	if errors.As(err, new(*auth.AssigneeNotAllowedError)) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	members, err := identity.MemberPrincipalsTx(ctx, tx, []ids.UUID{owner})
	if err != nil {
		return false, err
	}
	reader, live := members[owner]
	if !live {
		return false, nil
	}
	err = auth.EnsureActivityContentVisibleLive(principal.WithActor(ctx, reader), tx, task)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
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
