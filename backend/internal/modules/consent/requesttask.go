// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// A request from a subject whose pack follows the recipient becomes a task
// with that pack's deadline. The requests are a correction, an erasure, a
// restriction, an objection and a withdrawal of consent. The task shows the
// date, and the task list marks it overdue once the date passes.
//
// The activities module writes tasks, and a module never imports a sibling,
// so compose injects the writer and this file holds only the seam.

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/subjectrightsrules"
)

// The rights a request task can be about. Rectify and erasure are the request
// queue's own kinds.
const (
	RequestTaskRectify     = "rectify"
	RequestTaskErasure     = dsrKindErasure
	RequestTaskRestriction = dsrKindRestriction
	RequestTaskObjection   = "objection"
	RequestTaskWithdrawal  = "withdrawal"
)

// RequestTask is one request a person must carry out by a date.
type RequestTask struct {
	SubjectType string
	SubjectID   ids.UUID
	// Right is one of the RequestTask* kinds.
	Right string
	// Key names the act, so a replayed request opens no second task.
	Key string
	// ReplyBy is when the first answer is owed; zero when the pack sets none.
	ReplyBy time.Time
	DueAt   time.Time
}

// RequestTasks writes a request task on the caller's transaction.
type RequestTasks interface {
	OpenRequestTaskTx(ctx context.Context, tx pgx.Tx, task RequestTask) error
}

// WithRequestTasks wires the writer of request tasks.
func (s *Store) WithRequestTasks(w RequestTasks) *Store {
	s.requestTasks = w
	return s
}

// WithRequestTasks wires the writer of request tasks on the handlers' store.
func (h Handlers) WithRequestTasks(w RequestTasks) Handlers {
	h.store = h.store.WithRequestTasks(w)
	return h
}

// deadlineKindOf is the request kind whose deadline a right takes. Decree
// 356/2025 Art. 5.2 gives an objection and a withdrawal the restriction's.
func deadlineKindOf(right string) subjectrightsrules.RequestKind {
	switch right {
	case RequestTaskRectify:
		return subjectrightsrules.RequestKind(RequestTaskRectify)
	case RequestTaskErasure:
		return subjectrightsrules.RequestKind(dsrKindErasure)
	}
	return subjectrightsrules.RequestKind(dsrKindRestriction)
}

// openRequestTaskTx opens the task one request owes, when the subject's pack
// follows the recipient and declares a deadline for it. It runs in a
// savepoint, so a task that cannot be written never undoes the request. The
// request is recorded already and still answers the subject.
func (s *Store) openRequestTaskTx(ctx context.Context, tx pgx.Tx, who ruleSubject, right, key string, received time.Time) error {
	if s.requestTasks == nil {
		return nil
	}
	task, owed, err := requestTaskFor(ctx, tx, who, right, key, received)
	if err != nil || !owed {
		return err
	}
	nested, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	if err := s.requestTasks.OpenRequestTaskTx(ctx, nested, task); err != nil {
		if rollbackErr := nested.Rollback(ctx); rollbackErr != nil {
			return errors.Join(err, rollbackErr)
		}
		slog.ErrorContext(ctx, "the task for a data-subject request could not be written; the request stays recorded",
			"subject_type", task.SubjectType, "subject_id", task.SubjectID, "right", right, "cause", err)
		return nil
	}
	return nested.Commit(ctx)
}

// requestTaskFor builds the task, and answers false where no pack that
// follows the recipient sets a deadline for this right.
func requestTaskFor(ctx context.Context, tx pgx.Tx, who ruleSubject, right, key string, received time.Time) (RequestTask, bool, error) {
	code, err := subjectDataProtectionTx(ctx, tx, who)
	if err != nil || code == "" {
		return RequestTask{}, false, err
	}
	rules, declared := subjectrightsrules.For(code)
	deadline, ok := rules.Deadlines[deadlineKindOf(right)]
	if !declared || !ok {
		return RequestTask{}, false, nil
	}
	id, err := ids.Parse(who.ID)
	if err != nil {
		return RequestTask{}, false, err
	}
	task := RequestTask{
		SubjectType: who.Kind, SubjectID: id, Right: right, Key: key,
		DueAt: deadline.Due(received),
	}
	if rules.ReplyWithinWorkingDays > 0 {
		task.ReplyBy = subjectrightsrules.ReplyBy(received, rules.ReplyWithinWorkingDays)
	}
	return task, true, nil
}
