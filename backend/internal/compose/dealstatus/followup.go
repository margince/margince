// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// The follow-up owed after a meeting on this deal. It is the reader's own
// meeting with nothing sent since, the same answer Home's meeting follow-up
// row gives, read through the same activities query.

import (
	"context"
	"errors"
	"fmt"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// followUpOwed is the newest meeting on the deal that awaits a follow-up and
// has somebody on file to write to. A meeting with nobody on file has no
// draft to offer, so the card's later rungs speak for it.
func followUpOwed(f facts) (activities.AwaitingReply, bool) {
	for _, met := range f.followUps {
		if len(met.Recipients) > 0 {
			return met, true
		}
	}
	return activities.AwaitingReply{}, false
}

// followUpMove opens the composer to everyone the meeting was with. The
// recipients ride as contact links, so the queue re-checks each one on read.
func followUpMove(f facts, met activities.AwaitingReply) crmcontracts.DealStatusCardMove {
	said := statusCopyFor(f.lang)
	meetingID := openapi_types.UUID(met.ActivityID)
	links := make([]map[string]any, 0, len(met.Recipients))
	for _, contact := range met.Recipients {
		links = append(links, map[string]any{argEntityType: linkContact, argEntityID: openapi_types.UUID(contact)})
	}
	subject := met.Subject
	if subject == "" {
		subject = said.Say(statusWords.Meeting)
	}
	held := untimed(&meetingID, fmt.Sprintf(said.Say(statusWords.Met), subject))
	held.OccurredAt = &met.SentAt
	sent := occurredday.Moment{At: met.SentAt, DateOnly: occurredday.IsDay(met.Precision)}
	if sent.DateOnly {
		held.OccurredPrecision = crmcontracts.OccurredPrecisionOccurredPrecisionDay
	}
	return move(ActionDraftEmail,
		fmt.Sprintf(said.Say(statusWords.FollowUp), said.since(f.now, sent, f.loc)),
		map[string]any{argActivityID: meetingID, argLinks: links}, held)
}

// followUpMeeting is the meeting a stored follow-up move names: a draft that
// names a meeting and the contacts it goes to.
func followUpMeeting(move crmcontracts.DealStatusCardMove) (ids.UUID, bool) {
	if move.Action != ActionDraftEmail || len(NamedContacts(move)) == 0 {
		return ids.UUID{}, false
	}
	return NamedActivity(move)
}

// dropSettledFollowUps removes each cached follow-up whose meeting is no
// longer owed, asked of the same query that offered it. Without the read a
// follow-up cannot be confirmed, so it is dropped rather than served.
func (s *Service) dropSettledFollowUps(ctx context.Context, moves map[ids.UUID]crmcontracts.DealStatusCardMove) error {
	var meetings []ids.UUID
	for _, move := range moves {
		if meeting, ok := followUpMeeting(move); ok {
			meetings = append(meetings, meeting)
		}
	}
	if len(meetings) == 0 {
		return nil
	}
	owed := map[ids.UUID]bool{}
	if s.activities != nil && s.now != nil {
		read, err := s.activities.MeetingsStillAwaitingFollowUp(ctx, s.now(), meetings)
		if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
			return err
		}
		owed = read
	}
	for deal, move := range moves {
		if meeting, ok := followUpMeeting(move); ok && !owed[meeting] {
			delete(moves, deal)
		}
	}
	return nil
}
