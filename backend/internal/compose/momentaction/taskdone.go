// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

// Whether our later mail kept a promise task, asked the same way on every
// surface that lists such tasks: the Worklist and the company page.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DoneCandidate is a promise task the reader may be asked about: the message
// the promise was read from, who holds it, and its open recipients.
type DoneCandidate struct {
	TaskID           ids.UUID
	SourceActivityID ids.UUID
	Assignee         *ids.UUID
	Recipients       []ids.UUID
}

// AskTasksDone answers, per task, the question its newest unanswered follow-up
// asks. It reads the follow-up rule the contact page reads
// (activities.PromiseFollowUpsTx). A Not yet the reader gave, on any surface,
// keeps the plain row.
func AskTasksDone(ctx context.Context, tx pgx.Tx, candidates []DoneCandidate, asOf time.Time) (map[ids.UUID]crmcontracts.WorklistMayBeDone, error) {
	bases, byBasis, keys := promiseBases(candidates)
	if len(bases) == 0 {
		return map[ids.UUID]crmcontracts.WorklistMayBeDone{}, nil
	}
	followUps, err := activities.PromiseFollowUpsTx(ctx, tx, bases, asOf)
	if err != nil {
		return nil, err
	}
	setAside, err := DoneSetAsidesTx(ctx, tx, keys)
	if err != nil {
		return nil, err
	}
	return askTasks(ctx, candidates, byBasis, followUps, setAside), nil
}

// promiseBasisOwner is the task and recipient one follow-up basis stands for.
type promiseBasisOwner struct{ task, contact ids.UUID }

// promiseBases builds one follow-up basis per open recipient of each task, and
// the question keys the reader's Not yet answers are read under.
func promiseBases(candidates []DoneCandidate) ([]activities.PromiseBasis, map[ids.UUID]promiseBasisOwner, []string) {
	var bases []activities.PromiseBasis
	byBasis := map[ids.UUID]promiseBasisOwner{}
	var keys []string
	for _, task := range candidates {
		keys = append(keys, DoneKey(openapi_types.UUID(task.TaskID)))
		for _, recipient := range task.Recipients {
			basis := ids.NewV7()
			byBasis[basis] = promiseBasisOwner{task.TaskID, recipient}
			bases = append(bases, activities.PromiseBasis{PromiseID: basis, SourceActivityID: task.SourceActivityID, RecipientContactID: recipient})
		}
	}
	return bases, byBasis, keys
}

// doneAsked is one recipient's question about a promise task.
type doneAsked struct {
	contact     ids.UUID
	sent        activities.PromiseFollowUp
	fingerprint string
}

// askTasks is the question each task's newest unanswered follow-up asks.
func askTasks(
	ctx context.Context, candidates []DoneCandidate, byBasis map[ids.UUID]promiseBasisOwner,
	followUps map[ids.UUID]activities.PromiseFollowUp, setAside map[DoneSetAside]bool,
) map[ids.UUID]crmcontracts.WorklistMayBeDone {
	holders := map[ids.UUID]string{}
	for _, task := range candidates {
		holders[task.TaskID] = HeldMarker(ctx, task.Assignee)
	}
	byTask := map[ids.UUID][]doneAsked{}
	for basis, sent := range followUps {
		owner := byBasis[basis]
		fingerprint := DoneFingerprint(openapi_types.UUID(owner.task), sent, holders[owner.task])
		byTask[owner.task] = append(byTask[owner.task], doneAsked{contact: owner.contact, sent: sent, fingerprint: fingerprint})
	}
	out := map[ids.UUID]crmcontracts.WorklistMayBeDone{}
	for _, task := range candidates {
		key := DoneKey(openapi_types.UUID(task.TaskID))
		found, ok := newestUnanswered(byTask[task.TaskID], func(a doneAsked) bool {
			return setAside[DoneSetAside{Contact: a.contact, Key: key, Fingerprint: a.fingerprint}]
		})
		if !ok {
			continue
		}
		out[task.TaskID] = crmcontracts.WorklistMayBeDone{
			ContactId: openapi_types.UUID(found.contact), EmailActivityId: openapi_types.UUID(found.sent.EmailID),
			WroteAt: found.sent.At, WrotePrecision: crmcontracts.OccurredPrecision(found.sent.Precision),
			ClaimKey: key, EvidenceFingerprint: found.fingerprint,
		}
	}
	return out
}

// newestUnanswered picks the follow-up a promise task asks about. That is the
// newest one, unless the reader already answered Not yet to it or a later one.
// One recipient's Not yet then holds for the task until we write to anybody
// on the thread again. Equal instants are ordered by the mail's id, as
// PromiseFollowUpsTx orders them.
func newestUnanswered(candidates []doneAsked, answered func(doneAsked) bool) (doneAsked, bool) {
	later := func(a, b activities.PromiseFollowUp) bool {
		if !a.At.Equal(b.At) {
			return a.At.After(b.At)
		}
		return a.EmailID.String() > b.EmailID.String()
	}
	var newest, newestAnswered doneAsked
	var have, haveAnswered bool
	for _, c := range candidates {
		if answered(c) {
			if !haveAnswered || later(c.sent, newestAnswered.sent) {
				newestAnswered, haveAnswered = c, true
			}
			continue
		}
		if !have || later(c.sent, newest.sent) {
			newest, have = c, true
		}
	}
	if !have || (haveAnswered && !later(newest.sent, newestAnswered.sent)) {
		return doneAsked{}, false
	}
	return newest, true
}
