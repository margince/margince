// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

// The record a draft follows up, such as the meeting a follow-up is about. It
// leads the conversation the writer reads, above the recent activity.

import (
	"context"
	"errors"
	"time"

	"github.com/margince/margince/backend/internal/compose/activitywhen"
	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AboutActivities reads the activity a draft follows up, under the caller's
// content visibility. One they may not use answers apperrors.ErrNotFound.
type AboutActivities interface {
	GetActivityContent(ctx context.Context, id ids.ActivityID, archived storekit.ArchivedFilter) (crmcontracts.Activity, error)
}

var errNoAboutReader = errors.New("contactdraft: about_activity_id named with no activity reader wired")

// WithAboutActivities binds the read that resolves about_activity_id.
func (s *Service) WithAboutActivities(reader AboutActivities) *Service {
	s.aboutReader = reader
	return s
}

// aboutActivity reads the named activity, refusing one the caller may not
// use the way an unknown id is refused, so its existence stays hidden.
func (s *Service) aboutActivity(ctx context.Context, id ids.ActivityID, now time.Time, zone *time.Location) (*ActIn, error) {
	if s.aboutReader == nil {
		return nil, errNoAboutReader
	}
	activity, err := s.aboutReader.GetActivityContent(ctx, id, storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, httperr.Validation("about_activity_id", "not_found",
			"that activity is not one you can see — draft without it")
	}
	if err != nil {
		return nil, err
	}
	return foldAbout(activity, now, zone), nil
}

// foldAbout is the activity as the writer reads it, with the deal it is filed
// under so the follow-up knows which deal it continues.
func foldAbout(activity crmcontracts.Activity, now time.Time, zone *time.Location) *ActIn {
	folded := ActIn{
		ID:      activity.Id.String(),
		Kind:    string(activity.Kind),
		At:      activitywhen.Text(activity, zone),
		Inbound: activity.Direction != nil && *activity.Direction == crmcontracts.ActivityDirectionInbound,
	}
	if activity.Subject != nil {
		folded.Subject = *activity.Subject
	}
	if activity.Body != nil && !activity.OccurredAt.After(now) {
		folded.Record = draftcore.TurnText(*activity.Body)
	}
	if activity.Links != nil {
		for _, link := range *activity.Links {
			if link.EntityType == crmcontracts.ActivityLinkEntityTypeDeal {
				folded.DealID = link.EntityId.String()
				break
			}
		}
	}
	return &folded
}

// withoutActivity drops the activity already leading the input from the
// recent list, so the writer does not read it twice.
func withoutActivity(recent []ActIn, id string) []ActIn {
	out := recent[:0:0]
	for _, act := range recent {
		if act.ID != id {
			out = append(out, act)
		}
	}
	return out
}
