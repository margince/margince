// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attentionAwaiting binds the follow-up lane to the activities module's own
// gated read, and names each row's message so the composer can draft from it.
type attentionAwaiting struct {
	store *activities.Store
}

func (w attentionAwaiting) AwaitingReplies(
	ctx context.Context, asOf time.Time,
) ([]attention.AwaitedReply, bool, error) {
	rows, _, err := w.store.AwaitingReplies(ctx, asOf)
	if err != nil {
		return nil, false, err
	}
	summaries := map[ids.UUID]crmcontracts.EmailSummary{}
	if len(rows) > 0 {
		sent := make([]ids.UUID, 0, len(rows))
		for _, row := range rows {
			sent = append(sent, row.ActivityID)
		}
		if summaries, err = w.store.EmailSummariesByID(ctx, sent); err != nil {
			return nil, false, err
		}
	}
	out := make([]attention.AwaitedReply, 0, len(rows))
	for _, row := range rows {
		awaited := attention.AwaitedReply{
			ActivityID: row.ActivityID, Subject: row.Subject, SentAt: row.SentAt,
			ContactID: row.ContactID, CompanyID: row.CompanyID, DealID: row.DealID,
		}
		if summary, ok := summaries[row.ActivityID]; ok {
			awaited.EmailSummary = &summary
		}
		out = append(out, awaited)
	}
	return out, len(rows) >= activities.AwaitingReplyScanCap, nil
}

func (w attentionAwaiting) MeetingFollowUps(
	ctx context.Context, asOf time.Time,
) ([]attention.AwaitedReply, bool, error) {
	rows, err := w.store.MeetingsAwaitingFollowUp(ctx, asOf)
	if err != nil {
		return nil, false, err
	}
	out := make([]attention.AwaitedReply, 0, len(rows))
	for _, row := range rows {
		out = append(out, attention.AwaitedReply{
			ActivityID: row.ActivityID, Subject: row.Subject, SentAt: row.SentAt,
			ContactID: row.ContactID, CompanyID: row.CompanyID, DealID: row.DealID,
		})
	}
	return out, len(rows) >= activities.AwaitingReplyScanCap, nil
}
