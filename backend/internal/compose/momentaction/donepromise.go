// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

// Which promise card a record page turns into "This may be done?". The contact
// page and the account card both decide it here, so a promise asked on one is
// asked on the other.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// DonePromise is an open claim of ours a card speaks for, and the message it
// was read from. Contact is who our later mail on that thread must reach.
// That contact's page keeps the reader's Not yet.
type DonePromise struct {
	ID      ids.UUID
	Source  ids.UUID
	Contact ids.UUID
}

// ClaimDone is the promise an open claim of ours speaks for. A claim that
// became a task is asked through the task (TasksDoneTx), so it raises no
// question of its own.
func ClaimDone(claim, source, contact ids.UUID, task *ids.UUID) (DonePromise, bool) {
	if task != nil {
		return DonePromise{}, false
	}
	return DonePromise{ID: claim, Source: source, Contact: contact}, true
}

// DoneFollowUpsTx reads, per claim, our later mail on its thread to its
// contact (activities.PromiseFollowUpsTx).
func DoneFollowUpsTx(ctx context.Context, tx pgx.Tx, promises []DonePromise, now time.Time) (map[ids.UUID]activities.PromiseFollowUp, error) {
	bases := make([]activities.PromiseBasis, 0, len(promises))
	for _, promise := range promises {
		bases = append(bases, activities.PromiseBasis{PromiseID: promise.ID, SourceActivityID: promise.Source, RecipientContactID: promise.Contact})
	}
	return activities.PromiseFollowUpsTx(ctx, tx, bases, now)
}

// AskPromiseDone turns a claim card into the question whether our later mail
// kept it. A claim with no such mail keeps its plain card.
func AskPromiseDone(
	said langcopy.Spoken, zone *time.Location, plain crmcontracts.ContactMoment, promise DonePromise,
	followUps map[ids.UUID]activities.PromiseFollowUp,
) (crmcontracts.ContactMoment, bool) {
	sent, ok := followUps[promise.ID]
	if !ok {
		return crmcontracts.ContactMoment{}, false
	}
	id := openapi_types.UUID(promise.ID)
	q := doneQuestion{Plain: plain, Kind: crmcontracts.ContactMomentMayBeDonePromiseTypeClaim, PromiseID: id, Sent: sent, Key: DoneKey(id)}
	// A claim has no task, so its fingerprint carries no holder.
	return askDone(said, zone, q, doneEvidence(said, q), DoneFingerprint(id, sent, "")), true
}

// AskTaskDone turns a task card into the question TasksDoneTx asked of the task.
func AskTaskDone(said langcopy.Spoken, zone *time.Location, plain crmcontracts.ContactMoment, task ids.UUID, asked TaskAsked) crmcontracts.ContactMoment {
	q := doneQuestion{Plain: plain, Kind: crmcontracts.ContactMomentMayBeDonePromiseTypeTask, PromiseID: openapi_types.UUID(task), Sent: asked.Sent, Key: asked.Key}
	return askDone(said, zone, q, doneEvidence(said, q), asked.Fingerprint)
}
