// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// FloorDraft is the no-model draft of a reply to one activity. The HTTP route
// and the agent tool both fall back to it, so the greeting and the thread they
// answer cannot differ by transport.
func (s *Store) FloorDraft(ctx context.Context, anchor ids.UUID, activity crmcontracts.Activity, intent string) (subject, body string) {
	answering := DraftContext{
		Band:      convstate.BandFresh,
		Threaded:  IsMailThread(activity.Kind, activity.Direction),
		Recipient: s.GreetingName(ctx, ids.From[ids.ActivityKind](anchor)),
	}
	if activity.Subject != nil {
		answering.Topic = *activity.Subject
	}
	if activity.Body != nil {
		answering.Body = *activity.Body
	}
	return DeterministicEmailDraft(answering, intent)
}
