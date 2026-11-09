// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"
	"time"
)

// A proposal link must still be open when it arrives. One whose expiry is not
// after its creation is refused by name, so the host knows to offer a later time.
func TestAProposalWithNoUsableLifetimeIsRefusedByName(t *testing.T) {
	created := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	if err := proposalLifetimeRefusal(created, created.Add(time.Hour)); err != nil {
		t.Errorf("a link open for an hour was refused: %v", err)
	}
	for name, expires := range map[string]time.Time{
		"expires as it is made":     created,
		"expires before it is made": created.Add(-time.Minute),
	} {
		var refusal *SchedulingArgumentError
		if err := proposalLifetimeRefusal(created, expires); !errors.As(err, &refusal) || refusal.Code != "no_usable_lifetime" {
			t.Errorf("%s: refusal %v, want no_usable_lifetime", name, err)
		}
	}
}
