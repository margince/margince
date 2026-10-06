// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"testing"
	"time"

	"github.com/riverqueue/river"
)

// siteDeepReadKind is a declared kind whose queue is NOT the default, so a
// stamp that did nothing would be visible. It is read off the table rather
// than written here: the point of the helper is that nobody spells a queue
// twice, and a test that spelled this one would be the second copy.
const siteDeepReadKind = "site_deep_read"

type queuedAsTestArgs struct{}

func (queuedAsTestArgs) Kind() string { return siteDeepReadKind }

// The caller's own queue does not survive. A hand-built insert that named one
// is exactly how comms_send_email kept landing on `default` after its
// declaration moved, and the helper exists to make that unsayable.
func TestQueuedAsOverridesAQueueTheCallerNamed(t *testing.T) {
	t.Parallel()
	declared, ok := SpecFor(siteDeepReadKind)
	if !ok {
		t.Fatalf("%s is no longer declared; this test needs a kind with a non-default queue", siteDeepReadKind)
	}
	if declared.Queue == river.QueueDefault {
		t.Fatalf("%s now declares the default queue, so this test can no longer tell a stamp from a no-op", siteDeepReadKind)
	}

	got := QueuedAs[queuedAsTestArgs](&river.InsertOpts{Queue: "somewhere_else", MaxAttempts: 7})
	if got.Queue != declared.Queue {
		t.Errorf("queue = %q, want the declared %q", got.Queue, declared.Queue)
	}
	if got.MaxAttempts != 7 {
		t.Errorf("MaxAttempts = %d, want the caller's 7 — the helper answers the queue and nothing else", got.MaxAttempts)
	}
}

// A caller that asked for nothing else still gets its queue, and the caller's
// own options are not written through: two inserts of one kind must be able to
// differ without one editing the other's struct.
func TestQueuedAsLeavesTheCallersOptionsAlone(t *testing.T) {
	t.Parallel()
	if got := QueuedAs[queuedAsTestArgs](nil); got.Queue == "" {
		t.Error("a nil opts came back with no queue — River reads that as `default`, which is the pool nobody sized for this kind")
	}

	callers := &river.InsertOpts{MaxAttempts: 2, ScheduledAt: time.Time{}}
	QueuedAs[queuedAsTestArgs](callers)
	if callers.Queue != "" {
		t.Errorf("the caller's own struct now says queue %q — a helper that writes through it makes one insert's queue another's", callers.Queue)
	}
}

// An undeclared kind panics rather than falling through to River's default,
// which is the answer compose's fan-out helpers already give: the default is a
// pool nobody sized for this kind, and arriving there silently is the whole
// defect.
func TestQueuedAsRefusesAnUndeclaredKind(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("an undeclared kind was queued without a panic — it would land on `default` with nothing said")
		}
	}()
	QueuedAsKind("a_kind_the_contract_does_not_declare", nil)
}
