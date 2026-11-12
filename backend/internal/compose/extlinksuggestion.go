// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The core's half of extension.LinkSuggestionRepo: a unit proposes where a
// message belongs, on its own transaction, and the activities store records it.
// The proposal, its audit row and its event commit with the unit's own rows or
// not at all.

import (
	"context"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// extensionSuggestionsPerMinute is how many proposals one unit may make in a
// minute, across all its invocations.
const extensionSuggestionsPerMinute = 120

// extensionSuggestions meters proposals per unit. FailClosed: every proposal
// lands in a reader's queue, and an outage of the shared counter must not let a
// unit flood it.
var extensionSuggestions = ratelimit.New("extension-link-suggestion", ratelimit.FailClosed, extensionSuggestionsPerMinute, time.Minute)

//nolint:ireturn // returning the published repo IS the seam: a unit holds extension.LinkSuggestionRepo, never a core type.
func (c extensionCore) LinkSuggestions() extension.LinkSuggestionRepo {
	return extensionLinkSuggestions{core: c}
}

type extensionLinkSuggestions struct{ core extensionCore }

func (d extensionLinkSuggestions) Propose(ctx context.Context, in extension.LinkSuggestionRequest) (extension.LinkSuggestionOutcome, error) {
	if err := d.core.refuseUnattended(); err != nil {
		return "", err
	}
	ctx, err := d.core.invocationCtx(ctx)
	if err != nil {
		return "", err
	}
	if !extensionSuggestions.Allow(d.core.unit) {
		return "", extension.ErrRateLimited
	}
	activity, err := ids.ParseAs[ids.ActivityKind](in.Activity)
	if err != nil {
		return "", fmt.Errorf("%w: activity is not an id", extension.ErrInvalid)
	}
	target, err := ids.Parse(in.Target.ID)
	if err != nil {
		return "", fmt.Errorf("%w: target is not an id", extension.ErrInvalid)
	}
	outcome, err := activities.ProposeLinkSuggestionTx(ctx, d.core.tx, activities.LinkSuggestionInput{
		ActivityID: activity, EntityType: in.Target.Type, EntityID: target,
		Unit: d.core.unit, Reason: in.Reason, Score: in.Score,
	})
	if err != nil {
		return "", portRefusal(err)
	}
	return extension.LinkSuggestionOutcome(outcome), nil
}
