// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package loggedtouch answers draftcore.LoggedTouchReader from the activities
// store, so a draft counts a past meeting beyond its timeline page.
package loggedtouch

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordContact is what an activity link calls a contact.
const recordContact = "contact"

// Reader reads through the activities store's own gate. ConversationAsOf
// applies relstrength.HappenedByNowSQL, the SQL form of
// draftcore.IsLoggedTouch, so every row it returns is one the draft counts.
type Reader struct{ Store *activities.Store }

// NewestLoggedTouch is the newest past meeting filed on the record. A note is
// left out: it is no exchange (draftcore.CountsAsContact), and a newer note
// must not hide an older meeting behind the one-row limit. On
// a contact it is also the newest meeting they attended, which a calendar may
// not have linked. A caller who may not read activity gets nothing, as the
// draft's own view omits its timeline for them.
func (r Reader) NewestLoggedTouch(ctx context.Context, on draftcore.LoggedOn, now time.Time) ([]crmcontracts.Activity, error) {
	recordType, id := on.RecordType, on.ID
	filed, err := r.newest(ctx, activities.ListActivitiesInput{
		EntityType: &recordType, EntityID: &id,
		Kinds: []string{string(crmcontracts.ActivityKindMeeting)},
	}, on, now)
	if err != nil || on.RecordType != recordContact {
		return filed, err
	}
	party := ids.From[ids.ContactKind](on.ID)
	attended, err := r.newest(ctx, activities.ListActivitiesInput{
		PartyContactID: &party,
		Kinds:          []string{string(crmcontracts.ActivityKindMeeting)},
	}, on, now)
	if err != nil {
		return nil, err
	}
	for _, row := range attended {
		if !slices.ContainsFunc(filed, func(seen crmcontracts.Activity) bool { return seen.Id == row.Id }) {
			filed = append(filed, row)
		}
	}
	return filed, nil
}

// newest is the one newest row of in, read as the draft reads.
func (r Reader) newest(ctx context.Context, in activities.ListActivitiesInput, on draftcore.LoggedOn, now time.Time) ([]crmcontracts.Activity, error) {
	limit := 1
	in.Limit, in.WrittenByHuman, in.ConversationAsOf, in.WithinProjectID = &limit, true, &now, on.ProjectID
	rows, _, err := r.Store.ListActivities(ctx, in)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, nil
	}
	return rows, err
}
