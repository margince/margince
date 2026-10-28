// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// outsideTheRescuer names a kind that declares no timeout and holds no session
// lock, with what takes a row of it back when its worker dies.
var outsideTheRescuer = gatekit.Waive(map[string]string{
	"embed_reindex": "search.ReembedClaim.StealAfter takes a stalled reindex back from its binding marker",
})

// A kind with no timeout is outside River's rescuer for good, so a row its
// killed worker left running stays running. Its worker holds a session lock,
// which the dead worker's connection frees, or the kind names its own takeover.
func TestAKindTheRescuerNeverReachesCanStillBeTakenBack(t *testing.T) {
	t.Parallel()
	defer outsideTheRescuer.AssertAllMatched(t)
	reg, _ := wireJobs(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), censusJobConfig())
	var unrescued int
	for kind, wired := range reg.wired {
		spec, ok := jobs.SpecFor(kind)
		if !ok || !spec.Timeout.None {
			continue
		}
		unrescued++
		if _, locked := wired.args.(sessionLocked); locked || outsideTheRescuer.Waived(t, kind) {
			continue
		}
		t.Errorf("%s declares no timeout and holds no session lock, so a row its killed worker "+
			"left running blocks the kind forever. Take holdSessionLock in its worker and mark the "+
			"args SessionLocked, or name its takeover in outsideTheRescuer", kind)
	}
	if unrescued == 0 {
		t.Fatal("the census wired no kind without a timeout, so this check judged nothing")
	}
}
