// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// The recheck asks about one deal. A confident answer naming any other deal
// marks the link uncertain, never relevant.
func TestTheRecheckMarksByTheDealItAskedAbout(t *testing.T) {
	t.Parallel()
	current, other := ids.NewV7(), ids.NewV7()
	for name, c := range map[string]struct {
		topic DealTopic
		want  string
	}{
		"the deal asked about":   {DealTopic{Deal: current, Confident: true}, relstrength.DealRelevant},
		"a deal not asked about": {DealTopic{Deal: other, Confident: true}, relstrength.DealUncertain},
		"none of them":           {DealTopic{Confident: true}, relstrength.DealUnrelated},
		"doubt":                  {DealTopic{Deal: current}, relstrength.DealUncertain},
	} {
		if got := recheckMark(c.topic, current); got != c.want {
			t.Errorf("%s: marked %q, want %q", name, got, c.want)
		}
	}
}
