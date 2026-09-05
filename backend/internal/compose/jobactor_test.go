// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every job worker that reaches a store must bind an ACTOR, not just a
// workspace.
//
// A queue is not a request: it inherits no principal, so a worker that binds
// only its tenant reaches the first RBAC gate with nothing to check and the
// write is refused. The provider-run workers shipped that way — the poll job
// failed with "no actor bound to context" on every pass, so a paid enrichment
// run sat in_progress forever and the values it bought reached nobody. The
// money was spent and there was nothing to show for it.
//
// Nothing could have caught that but this shape of test. The failure is in the
// wiring rather than in any store, it needs a real queue to reproduce, and it
// looks exactly like a slow provider from the outside.
//
// So this reads the wiring itself: every Work method that builds a store must
// also bind an actor. Derived from the tree rather than a hand-kept list,
// because the next job added is the one nobody remembers to check.
//
// Three older workers are waived below rather than fixed. They have the same
// shape, but whether each one actually reaches a gated write is a question
// about three other subsystems, and answering it wrongly would either break
// working jobs or paper over a live defect. Issue #1127 tracks that audit; the
// waiver is a ratchet — a waived worker that starts binding an actor fails
// this test until its entry is removed.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// jobActorUnbound: workers that predate this gate and bind no actor. Each
// needs its own subsystem checked before it is changed — see #1127.
var jobActorUnbound = gatekit.Waive(map[string]string{
	"privacyRetentionWorker": "retention sweep; whether its writes are gated is #1127's question",
	"timeScanWorker":         "automation time-scan; same audit",
	"webhookRetryWorker":     "webhook delivery retry; same audit",
})

// storeBuilders are the handle constructors a store is built on. A Work method
// that calls one is about to reach an RBAC-gated entry point.
//
// InstallationDB belongs here for the same reason workspaceJobDB does, and it
// had to be added rather than assumed: a pass that collapses out of the
// per-tenant fan-out (ADR-0103 §1) stops calling workspaceJobDB and starts
// calling this one, so a regex naming only the old constructor would let every
// collapsed worker slip out of this gate's sight while reading as green.
// database.BindTo joined the list with ADR-0103. workspaceJobDB reads the
// workspace off a job's ARGS, and a collapsed pass has none — it takes the
// workspace from the fleet walk instead and binds the handle directly, so a
// pass that reaches a store now does it through BindTo. Leaving it out would
// have let every collapsed pass build a store unwatched.
var storeBuilders = regexp.MustCompile(`(providerRunStore|workspaceJobDB|InstallationDB|database\.BindTo)\(`)

// actorBinders are the ways a worker legitimately names its principal: one of
// this package's context helpers, or principal.WithActor directly.
//
// reconcileWorkerCtx is one of those helpers — it binds the workspace, the
// actor and a correlation id together and returns the context. It is listed for
// the same reason providerJobActor is, and it was found the way a missing entry
// here always will be: as a false positive, on a worker whose actor is bound
// one call away.
//
// The ASSIGNMENT is part of the pattern, not decoration. Both calls return a
// new context and mutate nothing, so `providerJobActor(wsCtx)` on its own line
// compiles, reads like a binding, and leaves the store holding a context with
// no actor in it — the precise bug this gate exists to catch, sailing past a
// check that only asked whether the name appeared.
var actorBinders = regexp.MustCompile(
	`\w+\s*(=|:=)\s*(providerJobActor|reconcileWorkerCtx|principal\.WithActor)\(`)

// workMethod matches a River worker's entry point and captures its receiver,
// which is the worker's name in the failure message.
// A worker's ENTRY POINTS, which is Work and — since ADR-0103 collapsed the
// workspace dispatchers — the per-workspace turn Work walks the fleet with.
//
// Both, because the collapse MOVED the store build out of Work. A collapsed
// pass's Work is one line handing runPerWorkspace a method, and the store is
// built inside that method, per tenant. Reading Work alone would have quietly
// emptied this gate of subjects exactly as the passes were rewritten — every
// one of them would have stopped being checked, and the gate would have gone
// green by having nothing left to look at.
var workMethod = regexp.MustCompile(`func \(\w+ \*(\w+)\) (?:Work|\w*[Ww]orkspace)\(`)

func TestEveryJobWorkerThatReachesAStoreBindsAnActor(t *testing.T) {
	defer jobActorUnbound.AssertAllMatched(t)
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the compose package: %v", err)
	}

	offenders := map[string]string{}
	var checked int
	for _, f := range files {
		// Every .go file in the package, not just jobs_*.go: a worker lives
		// wherever its author put it (capturejobs.go, for one), and a gate that
		// trusts a filename convention misses the file that broke it.
		name := f.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for _, body := range workMethodBodies(string(src)) {
			checked++
			if !storeBuilders.MatchString(body.text) {
				continue
			}
			if actorBinders.MatchString(body.text) {
				continue
			}
			if jobActorUnbound.Waived(t, body.worker) {
				continue
			}
			offenders[body.worker] = name
		}
	}

	if checked == 0 {
		t.Fatal("no job Work methods found — this gate would pass vacuously; check the jobs_*.go naming")
	}

	names := make([]string, 0, len(offenders))
	for w := range offenders {
		names = append(names, w)
	}
	sort.Strings(names)
	for _, w := range names {
		t.Errorf("%s (%s) builds a store but binds no actor — its first RBAC-gated write will be refused with \"no actor bound to context\", and a queue carries no principal to inherit. Bind one the way providerJobActor does",
			w, offenders[w])
	}
}

type workBody struct {
	worker string
	text   string
}

// workMethodBodies splits a file into its Work methods. A method runs to the
// next top-level `func ` or to end of file, which is enough structure here: the
// question is only which calls appear inside one.
func workMethodBodies(src string) []workBody {
	locs := workMethod.FindAllStringSubmatchIndex(src, -1)
	out := make([]workBody, 0, len(locs))
	for i, loc := range locs {
		end := len(src)
		if next := regexp.MustCompile(`(?m)^func `).FindStringIndex(src[loc[1]:]); next != nil {
			end = loc[1] + next[0]
		}
		if i+1 < len(locs) && locs[i+1][0] < end {
			end = locs[i+1][0]
		}
		out = append(out, workBody{worker: src[loc[2]:loc[3]], text: src[loc[0]:end]})
	}
	return out
}
