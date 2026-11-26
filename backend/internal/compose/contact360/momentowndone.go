// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// Done on the two cards that rest on our last message going unanswered. It is
// the Worklist's Done on that message, one reader's own, so a Done pressed on
// either surface hides the card and the row together.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ourLastMessageDone reads the reader's standing on the last message we sent
// the contact. With no such message there is nothing a Done could name.
func (s *Service) ourLastMessageDone(ctx context.Context, tx pgx.Tx, now time.Time, page *crmcontracts.Contact360) (activities.OwnMessageDone, error) {
	if page.LastOutboundActivityId == nil || s.messages == nil {
		return activities.OwnMessageDone{}, nil
	}
	return s.messages.OwnMessageDoneState(ctx, tx, ids.From[ids.ActivityKind](ids.UUID(*page.LastOutboundActivityId)), now)
}

// restsOnOurLastMessage names the cards built from our last message alone: the
// wait for their reply, and the silence it turns into from day seven.
func restsOnOurLastMessage(moment crmcontracts.ContactMoment) bool {
	return moment.Rule == crmcontracts.ContactMomentRuleAwaitingReply ||
		moment.Rule == crmcontracts.ContactMomentRuleGoneQuiet
}

// cardsPutAway is whether the reader put a card away. They dismissed it against
// the evidence it still shows. Or it rests on our last message, which they
// marked done, whatever rule fires on that message today.
func cardsPutAway(dismissals map[string]string, ourLast activities.OwnMessageDone) func(crmcontracts.ContactMoment) bool {
	return func(moment crmcontracts.ContactMoment) bool {
		if ourLast.SetAside && restsOnOurLastMessage(moment) {
			return true
		}
		stored, ok := dismissals[moment.ClaimKey]
		return ok && stored == moment.EvidenceFingerprint
	}
}

// offerDone sets the card's Done only where pressing it changes something. The
// card rests on our last message, and this reader may mark it and has not set
// it aside.
func offerDone(moment *crmcontracts.ContactMoment, standing activities.OwnMessageDone, page *crmcontracts.Contact360) {
	if !restsOnOurLastMessage(*moment) || !standing.CanMark || standing.SetAside || page.LastOutboundActivityId == nil {
		return
	}
	moment.Done = &crmcontracts.ContactMomentDone{ActivityId: *page.LastOutboundActivityId}
}
