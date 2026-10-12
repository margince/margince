// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// builtFor is a dispatcher that remembers the pace it was built with.
type builtFor struct{ pace sendPace }

func (builtFor) DispatchWithWait(context.Context, ids.UUID) (comms.Outcome, time.Duration, error) {
	return comms.OutcomeSent, 0, nil
}

// The rate counts live in the dispatcher, so rebuilding it on every send
// would let every mailbox send without limit.
func TestThePacedDispatcherIsRebuiltOnlyWhenThePaceChanges(t *testing.T) {
	builds := 0
	paced := &pacedDispatcher{build: func(pace sendPace) deliveryDispatcher {
		builds++
		return builtFor{pace: pace}
	}}
	first := sendPace{limit: 30, window: time.Minute, maxAge: 24 * time.Hour}

	paced.dispatcherFor(first)
	paced.dispatcherFor(first)
	if builds != 1 {
		t.Fatalf("two sends at one pace built %d dispatchers, want 1", builds)
	}

	slower := first
	slower.limit = 5
	got := paced.dispatcherFor(slower)
	if builds != 2 {
		t.Fatalf("a changed pace built %d dispatchers in all, want 2", builds)
	}
	if built, ok := got.(builtFor); !ok || built.pace != slower {
		t.Errorf("after the change the worker dispatches through %+v, want one built for %+v", got, slower)
	}
}
