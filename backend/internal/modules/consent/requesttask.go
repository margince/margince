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
	"fmt"
	"strings"
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

// RequestTask is one request a colleague must carry out by a date.
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
// follows the recipient and declares a deadline for it. It writes on the
// request's own transaction, and a task that cannot be written fails the
// request. So no request is recorded without the deadline work it owes.
func (s *Store) openRequestTaskTx(ctx context.Context, tx pgx.Tx, who ruleSubject, right, key string, received time.Time) error {
	if s.requestTasks == nil {
		return nil
	}
	task, owed, err := requestTaskFor(ctx, tx, who, right, key, received)
	if err != nil || !owed {
		return err
	}
	if err := s.requestTasks.OpenRequestTaskTx(ctx, tx, task); err != nil {
		return fmt.Errorf("consent: opening the task this request owes: %w", err)
	}
	return nil
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
		task.ReplyBy = subjectrightsrules.ReplyBy(received, rules.ReplyWithinWorkingDays, rules.UTCOffset)
	}
	return task, true, nil
}

// requestReceipt is the first answer a request is owed under a pack that
// follows the recipient and sets one.
var requestReceipt = receiptKind{owed: (*Store).requestReceiptOwedTx, render: RenderRequestReceipt}

// requestReceiptOwedTx answers whether the subject's pack owes a request its
// first answer, and when. The receipt prints its times in the pack's zone.
func (s *Store) requestReceiptOwedTx(ctx context.Context, tx pgx.Tx, sub subject) (receiptTerms, bool, error) {
	code, err := subjectDataProtectionTx(ctx, tx, ruleSubject{Kind: sub.entityType, ID: sub.id.String()})
	if err != nil || code == "" {
		return receiptTerms{}, false, err
	}
	rights, declared := subjectrightsrules.For(code)
	if !declared || rights.ReplyWithinWorkingDays <= 0 {
		return receiptTerms{}, false, nil
	}
	rules, _, _ := applicableRules(code)
	due := subjectrightsrules.ReplyBy(s.now(), rights.ReplyWithinWorkingDays, rights.UTCOffset)
	return receiptTerms{dueAt: &due, utcOffset: rules.OptOutAcknowledgementUTCOffset}, true, nil
}

// answerRequestTx answers a request the opt-out receipt does not cover: a
// restriction, or a withdrawal of a purpose that is not marketing. It stages
// the request receipt at once and opens the dated task.
func (s *Store) answerRequestTx(ctx context.Context, tx pgx.Tx, sub subject, right, key string, received time.Time) error {
	who := ruleSubject{Kind: sub.entityType, ID: sub.id.String()}
	task, owed, err := requestTaskFor(ctx, tx, who, right, key, received)
	if err != nil || !owed {
		return err
	}
	messageID := "request-ack-" + right + "-" + messageIDPart(key) + "@margince.invalid"
	// One receipt per act: a press that withdrew two purposes is one request.
	first, err := firstAcknowledgementOfActTx(ctx, tx, messageID)
	if err != nil || !first {
		return err
	}
	if err := s.stageReceiptTx(ctx, tx, requestReceipt, sub, messageID, ids.UUID{},
		RefusalTimes{ReceivedAt: received, StoppedAt: task.DueAt}); err != nil {
		return err
	}
	return s.openRequestTaskTx(ctx, tx, who, right, key, received)
}

// messageIDPart keeps the letters and digits of a request's key, so it can
// stand inside a message id. The key is unique per act already.
func messageIDPart(key string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, key)
}
