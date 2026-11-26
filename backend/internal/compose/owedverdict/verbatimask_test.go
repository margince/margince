// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package owedverdict

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
)

func TestARequestKeepsOnlyTheSendersOwnWords(t *testing.T) {
	t.Parallel()
	body := "Hi,\n\nthe rollout has to move to March.\nCan you update the\nclose date accordingly?\n\nBest, Anna"
	asks, informs := activities.OwedVerdictAsksUs, activities.OwedVerdictInformsUs
	for name, c := range map[string]struct{ verdict, ask, want string }{
		"a span across a line break":     {asks, "Can you update the close date accordingly?", "Can you update the close date accordingly?"},
		"words the sender never wrote":   {asks, "Please review the close date.", ""},
		"a message that asks nothing":    {informs, "the rollout has to move to March.", ""},
		"an ask longer than one request": {asks, strings.Repeat("a", AskLimit+1), ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := VerbatimAsk(c.verdict, c.ask, body); got != c.want {
				t.Fatalf("ask %q, want %q", got, c.want)
			}
		})
	}
}
