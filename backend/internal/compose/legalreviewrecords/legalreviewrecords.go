// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package legalreviewrecords reads the agreements, touches and open tasks a
// legal review pack carries. Contracts and activities belong to other modules
// and the pack to deals, and none imports another, so this package makes the
// edge.
//
// Every read runs under the caller's principal and goes through the owning
// store's own gate. A mail activity follows its audience, so a pack holds what
// the requester could read and shows a reviewer what they can.
package legalreviewrecords

import (
	"context"
	"errors"
	"slices"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	dealType = "deal"
	// touchLimit and taskLimit keep the pack short: it is a briefing.
	touchLimit = 5
	taskLimit  = 10
	// touchSweep reads past the automatic mail that is not a touch.
	touchSweep = 25
)

// touchKinds are the kinds a person does with the buyer. A task is listed
// apart.
var touchKinds = []string{
	string(crmcontracts.ActivityKindCall), string(crmcontracts.ActivityKindEmail),
	string(crmcontracts.ActivityKindMeeting), string(crmcontracts.ActivityKindMessage),
	string(crmcontracts.ActivityKindNote),
}

// Reader implements deals.LegalReviewRecords.
type Reader struct {
	contracts  *contracts.Store
	activities *activities.Store
}

var _ deals.LegalReviewRecords = Reader{}

// New builds the reader over the two owners' stores.
func New(contractStore *contracts.Store, activityStore *activities.Store) Reader {
	return Reader{contracts: contractStore, activities: activityStore}
}

// unreadable is a refusal that means the caller may not read this kind of
// record, which the pack shows as none.
func unreadable(err error) bool {
	return errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied)
}

// ReviewContracts lists the deal's agreements, newest term first.
func (r Reader) ReviewContracts(ctx context.Context, dealID ids.UUID) ([]deals.PacketContract, error) {
	page, err := r.contracts.ListDealContracts(ctx, ids.From[ids.DealKind](dealID))
	if unreadable(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]deals.PacketContract, 0, len(page.Data))
	for _, c := range page.Data {
		packed := deals.PacketContract{
			ContractID: ids.UUID(c.Id), ContractNumber: c.ContractNumber, Title: c.Title,
			StartsOn: dateOf(c.StartsOn), EndsOn: dateOf(c.EndsOn), RenewalOn: dateOf(c.RenewalOn),
			CancellationNoticeOn: dateOf(c.CancellationNoticeOn),
			NoticePeriodDays:     c.NoticePeriodDays, AutoRenew: c.AutoRenew,
		}
		if c.Status != nil {
			packed.Status = string(*c.Status)
		}
		out = append(out, packed)
	}
	return out, nil
}

// ReviewActivity lists the deal's latest touches and its open tasks.
func (r Reader) ReviewActivity(ctx context.Context, dealID ids.UUID) ([]deals.PacketTouch, []deals.PacketTask, error) {
	entity := dealType
	limit := touchSweep
	rows, _, err := r.activities.ListActivities(ctx, activities.ListActivitiesInput{
		EntityType: &entity, EntityID: &dealID, Kinds: touchKinds, Limit: &limit,
	})
	if unreadable(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var touches []deals.PacketTouch
	for _, a := range rows {
		if a.AutomaticKind != nil {
			continue
		}
		touches = append(touches, deals.PacketTouch{ActivityID: ids.UUID(a.Id), Kind: string(a.Kind), OccurredAt: a.OccurredAt})
		if len(touches) == touchLimit {
			break
		}
	}
	open, _, err := r.activities.ListOpenTasks(ctx, activities.ListOpenTasksInput{
		EntityType: &entity, EntityID: &dealID, ExcludeSystemMinted: true, Limit: taskLimit,
	})
	if unreadable(err) {
		return touches, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	tasks := make([]deals.PacketTask, 0, len(open))
	for _, t := range open {
		tasks = append(tasks, deals.PacketTask{ActivityID: t.ID, DueAt: t.DueAt})
	}
	return touches, tasks, nil
}

// OpenableActivity asks the activity list's own gate which of these the
// caller can read, so mail outside their audience answers absent.
func (r Reader) OpenableActivity(ctx context.Context, dealID ids.UUID, activityIDs []ids.UUID) (map[ids.UUID]*string, error) {
	out := make(map[ids.UUID]*string, len(activityIDs))
	if len(activityIDs) == 0 {
		return out, nil
	}
	entity := dealType
	limit := max(len(activityIDs), 1)
	wanted := make([]ids.ActivityID, 0, len(activityIDs))
	for _, id := range activityIDs {
		wanted = append(wanted, ids.From[ids.ActivityKind](id))
	}
	rows, _, err := r.activities.ListActivities(ctx, activities.ListActivitiesInput{
		EntityType: &entity, EntityID: &dealID, IDs: wanted, Limit: &limit,
	})
	if unreadable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, a := range rows {
		id := ids.UUID(a.Id)
		if !slices.Contains(activityIDs, id) {
			continue
		}
		if a.ContentState != nil && *a.ContentState == crmcontracts.ActivityContentStateWithheld {
			out[id] = nil
			continue
		}
		out[id] = a.Subject
	}
	return out, nil
}

func dateOf(d *openapi_types.Date) *time.Time {
	if d == nil {
		return nil
	}
	return &d.Time
}
