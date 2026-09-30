// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The team board's columns are every one of them `required` in the contract, so
// there is no way to draw a column that has no reader. A source dropped from
// the wiring below would therefore arrive as a zero, and a zero on this board
// says a teammate is clear.
//
// attention's own tests cannot see this: they arrange the seams themselves, and
// a test that arranged them would keep passing while the shipped service lost
// one — which is the failure newAttentionService's doc comment already names as
// the reason it is separate from the handler.

import (
	"strings"
	"testing"
	"time"
)

func TestTheComposedFeedBindsEveryTeamBoardSource(t *testing.T) {
	t.Parallel()

	// No pool and no approvals service: binding a source constructs a store
	// over the handle, and none of them reads until a lane does. What is under
	// test is the wiring, which is decided before any query.
	svc := newAttentionService(nil, nil, func() time.Time { return time.Now().UTC() })

	if unbound := svc.UnboundBoardSources(); len(unbound) > 0 {
		t.Fatalf("the composed feed binds no reader for the board's %s column(s) — every one "+
			"is required, so the board would report a clean team rather than refusing",
			strings.Join(unbound, ", "))
	}
}
