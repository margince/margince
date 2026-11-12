// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

// Whether our later mail kept a promise task. Every surface that lists such
// tasks asks it the same way: the Worklist, the company page and the contact
// page.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OpenRecipient says whether a recipient's claim on a promise still waits on it.
// The Worklist and the company page both build a candidate's Recipients from it.
func OpenRecipient(status string) bool {
	return status == string(crmcontracts.ConversationClaimStatusOpen)
}

// DoneCandidate is a promise task the reader may be asked about: the message
// the promise was read from, who holds it, and its open recipients.
type DoneCandidate struct {
	TaskID           ids.UUID
	SourceActivityID ids.UUID
	Assignee         *ids.UUID
	Recipients       []ids.UUID
}

// AskTasksDone answers, per task, the question its newest unanswered follow-up
// asks, in the shape the Worklist and the company page's rows carry.
func AskTasksDone(ctx context.Context, tx pgx.Tx, candidates []DoneCandidate, asOf time.Time) (map[ids.UUID]crmcontracts.WorklistMayBeDone, error) {
	asked, err := TasksDoneTx(ctx, tx, candidates, asOf)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]crmcontracts.WorklistMayBeDone, len(asked))
	for task, q := range asked {
		out[task] = crmcontracts.WorklistMayBeDone{
			ContactId: openapi_types.UUID(q.Contact), EmailActivityId: openapi_types.UUID(q.Sent.EmailID),
			WroteAt: q.Sent.At, WrotePrecision: crmcontracts.OccurredPrecision(q.Sent.Precision),
			ClaimKey: q.Key, EvidenceFingerprint: q.Fingerprint,
		}
	}
	return out, nil
}

// TaskAsked is the question one promise task asks. It names the recipient our
// mail reached, that mail, and the key and fingerprint a Not yet is tied to.
// The recipient's contact page keeps the Not yet.
type TaskAsked struct {
	Contact     ids.UUID
	Sent        activities.PromiseFollowUp
	Key         string
	Fingerprint string
}

// TasksDoneTx is, per task, the question its newest unanswered follow-up asks.
// Every surface that lists a promise task asks through it. It reads the
// follow-up rule claims are asked by (activities.PromiseFollowUpsTx). A Not
// yet the reader gave, on any surface, keeps the plain row. Only a reader who
// may complete a task is asked.
func TasksDoneTx(ctx context.Context, tx pgx.Tx, candidates []DoneCandidate, asOf time.Time) (map[ids.UUID]TaskAsked, error) {
	candidates, err := completable(ctx, tx, candidates)
	if err != nil {
		return nil, err
	}
	bases, byBasis, keys := promiseBases(candidates)
	if len(bases) == 0 {
		return map[ids.UUID]TaskAsked{}, nil
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

func completable(ctx context.Context, tx pgx.Tx, candidates []DoneCandidate) ([]DoneCandidate, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	named := make([]ids.UUID, len(candidates))
	for i, task := range candidates {
		named[i] = task.TaskID
	}
	writable, err := auth.ActivityWritableSubset(ctx, tx, named)
	if err != nil {
		return nil, err
	}
	out := candidates[:0:0]
	for _, task := range candidates {
		if writable[task.TaskID] {
			out = append(out, task)
		}
	}
	return out, nil
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
) map[ids.UUID]TaskAsked {
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
	out := map[ids.UUID]TaskAsked{}
	for _, task := range candidates {
		key := DoneKey(openapi_types.UUID(task.TaskID))
		found, ok := newestUnanswered(byTask[task.TaskID], func(a doneAsked) bool {
			return setAside[DoneSetAside{Contact: a.contact, Key: key, Fingerprint: a.fingerprint}]
		})
		if !ok {
			continue
		}
		out[task.TaskID] = TaskAsked{Contact: found.contact, Sent: found.sent, Key: key, Fingerprint: found.fingerprint}
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

// TaskClaims reads the recipient claims filed on tasks.
type TaskClaims interface {
	ClaimsOnTasks(ctx context.Context, tasks []ids.UUID) ([]contacts.TaskClaim, error)
}

// PromiseTask is a task a surface lists: the system that wrote it, the message
// it was read out of, and who holds it.
type PromiseTask struct {
	ID       ids.UUID
	System   string
	Source   *ids.UUID
	Assignee *ids.UUID
}

// DoneCandidates are the listed tasks written for a promise read out of a
// message, each with the recipients whose claim on it is still open. A reply
// reminder has its own judge, and a hand task names no promise.
func DoneCandidates(ctx context.Context, claims TaskClaims, tasks []PromiseTask) ([]DoneCandidate, error) {
	byTask := map[ids.UUID]*DoneCandidate{}
	var named []ids.UUID
	for _, task := range tasks {
		if task.System != activities.CommitmentTaskSource || task.Source == nil {
			continue
		}
		byTask[task.ID] = &DoneCandidate{TaskID: task.ID, SourceActivityID: *task.Source, Assignee: task.Assignee}
		named = append(named, task.ID)
	}
	if len(named) == 0 {
		return nil, nil
	}
	recipients, err := claims.ClaimsOnTasks(ctx, named)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, claim := range recipients {
		if claim.Task == nil || !OpenRecipient(claim.Status) {
			continue
		}
		if candidate, ok := byTask[*claim.Task]; ok {
			candidate.Recipients = append(candidate.Recipients, claim.Contact)
		}
	}
	out := make([]DoneCandidate, 0, len(named))
	for _, id := range named {
		out = append(out, *byTask[id])
	}
	return out, nil
}
