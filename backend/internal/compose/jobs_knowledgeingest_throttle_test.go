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

func TestAThrottledIngestSaysTheProviderWasTheCause(t *testing.T) {
	for name, cause := range map[string]error{
		"a throttle": fmt.Errorf("embedding: %w", ai.ErrProviderThrottled),
		"a quota":    fmt.Errorf("embedding: %w", ai.ErrProviderQuota),
	} {
		t.Run(name, func(t *testing.T) {
			detail := ingestDetail(cause)
			if strings.Contains(detail, "could not be read into passages") {
				t.Errorf("%q blames the file for what the AI provider refused", detail)
			}
			if !strings.Contains(strings.ToLower(detail), "ai provider") {
				t.Errorf("%q does not name the provider", detail)
			}
		})
	}
}

// A throttle asks the caller to come back later, so it is waited out, and a
// try is not spent inside the wait. It is waited out for a bounded time, or a
// provider that never recovers would hold a document in `running` forever.
func TestAThrottledIngestWaitsOutTheProviderForABoundedTime(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	throttled := fmt.Errorf("embedding: %w", ai.ErrProviderThrottled)

	if wait, snooze := ingestSnooze(throttled, now.Add(-time.Minute), now); !snooze || wait <= 0 {
		t.Errorf("a fresh throttled ingest was not snoozed: %v %v", wait, snooze)
	}
	if _, snooze := ingestSnooze(throttled, now.Add(-2*knowledgeIngestSnoozeWindow), now); snooze {
		t.Error("an ingest throttled past the window was still snoozed, so it would wait forever")
	}
	if _, snooze := ingestSnooze(errors.New("a database blip"), now, now); snooze {
		t.Error("a failure that is not a throttle was snoozed")
	}
}

// After the wait, a shipped page that stays failed must not tell an admin to
// upload a file nobody uploaded.
func TestAFailedHandbookPageDoesNotAskForAnUpload(t *testing.T) {
	detail := ingestDetail(fmt.Errorf("%q: %w", "meetings-and-booking.md", knowledge.ErrHandbookPageGone))

	if strings.Contains(strings.ToLower(detail), "upload") {
		t.Errorf("%q asks for an upload of a page that ships with the release", detail)
	}
	if !strings.Contains(detail, "release") {
		t.Errorf("%q does not say the release is the cause", detail)
	}
}

// A rollout can hand a page the new release added to a worker that lacks it.
// That clears when a newer worker takes the job. It waits for a bounded time
// and spends none of the attempts that mark a document failed.
func TestAHandbookPageAnOlderWorkerDoesNotShipWaitsForANewerOne(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	gone := fmt.Errorf("%q: %w", "meetings-and-booking.md", knowledge.ErrHandbookPageGone)

	if wait, snooze := ingestSnooze(gone, now.Add(-time.Minute), now); !snooze || wait <= 0 {
		t.Errorf("a page this worker does not ship was not snoozed: %v %v", wait, snooze)
	}
	if _, snooze := ingestSnooze(gone, now.Add(-2*knowledgeIngestSnoozeWindow), now); snooze {
		t.Error("a page still unshipped past the window was snoozed, so it would wait forever")
	}
}
