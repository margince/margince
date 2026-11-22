// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/knowledge"
)

// The detail names the real cause. A provider refusal must not blame the file,
// and a shipped page that stays failed must not ask an admin to upload one.
func TestAFailedIngestSaysWhatActuallyWentWrong(t *testing.T) {
	gone := fmt.Errorf("%q: %w", "meetings-and-booking.md", knowledge.ErrHandbookPageGone)
	for name, tc := range map[string]struct {
		cause     error
		want, not []string
	}{
		"a throttle":     {fmt.Errorf("embedding: %w", ai.ErrProviderThrottled), []string{"ai provider"}, []string{"could not be read into passages"}},
		"a quota":        {fmt.Errorf("embedding: %w", ai.ErrProviderQuota), []string{"ai provider"}, []string{"could not be read into passages"}},
		"a shipped page": {gone, []string{"update every worker", "withdrawn"}, []string{"upload"}},
	} {
		detail := strings.ToLower(ingestDetail(tc.cause))
		for _, w := range tc.want {
			if !strings.Contains(detail, w) {
				t.Errorf("%s: %q does not name %q", name, detail, w)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(detail, n) {
				t.Errorf("%s: %q wrongly says %q", name, detail, n)
			}
		}
	}
}

// A throttle is waited out, spending no attempt, for a bounded time: a provider
// that never recovers would otherwise hold a document in `running` forever.
func TestAThrottledIngestWaitsOutTheProviderForABoundedTime(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	throttled := fmt.Errorf("embedding: %w", ai.ErrProviderThrottled)
	for name, tc := range map[string]struct {
		err  error
		age  time.Duration
		want bool
	}{
		"fresh":      {throttled, time.Minute, true},
		"past":       {throttled, 2 * knowledgeIngestSnoozeWindow, false},
		"not a wait": {errors.New("a database blip"), 0, false},
	} {
		if wait, snooze := ingestSnooze(tc.err, now.Add(-tc.age), now); snooze != tc.want || (snooze && wait <= 0) {
			t.Errorf("%s: snooze=%v wait=%v, want snooze=%v", name, snooze, wait, tc.want)
		}
	}
}
