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
// Three older workers were waived here while the question "does this one
// actually reach a gated write" was open for each. It is answered now, and two
// of the three were FALSE POSITIVES rather than defects: privacyRetentionWorker
// binds a system actor through retentionPassProvenance, and timeScanWorker's
// per-workspace turn binds one through scanLeadSLA — in both cases one call
// away, which is the only reason this gate could not see it.
//
// That is what the one-level follow below exists for. A binder is a helper by
// nature: it takes a context, returns one with the principal in it, and reads
// better beside the pass it names than inlined into a Work method. A gate that
// only looked at the Work body was therefore going to keep producing waivers
// for correct code, and every one of those is a place a real defect can hide.
//
// The third is genuine and stays waived: webhookRetryWorker reaches no gate
// under its own principal, and the one authority question its path asks is
// asked under the subscription OWNER's. Its entry says so and names the
// evidence.
//
// The waiver is a ratchet — a waived worker that starts binding an actor fails
// this test until its entry is removed.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// jobActorUnbound: workers that reach a store and bind no actor because they
// need none. The audit behind the one entry left is in this file's doc.
var jobActorUnbound = gatekit.Waive(map[string]string{
	"webhookRetryWorker": "the retry sweep resolves no principal of its own and reaches no RBAC gate under one. Its path is dueRetries → loadTarget → stillVisible → markVisibilityRevoked → deliverOnce, and none of those asks auth.Require; the two entry points in that store which do (ListDeliveries, requireReplay) are the human dead-letter surfaces this pass never touches. The one authority question it DOES ask — may this subscription's owner still see the record this delivery carries — is asked under the OWNER's principal, resolved per delivery from the row (Deliverer.canSee binds it with principal.WithActor from EffectiveRBAC). An actor bound here would be a second, wrong answer to that question. Held by webhookretry_pass_integration_test.go, whose webhookSweepCtx binds the tenant and nothing else",
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

	pkg := packageFunctions(t, files)
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
			if actorBinders.MatchString(withCalledHelpers(body.text, pkg)) {
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

// packageFunctions maps every top-level function and method in the compose
// package to its body, so a Work method's own text can be read together with
// the helpers it calls.
func packageFunctions(t *testing.T, files []os.DirEntry) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range files {
		name := f.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		indexFunctions(out, string(src))
	}
	return out
}

// indexFunctions records one file's RECEIVER-LESS functions by name.
//
// Receiver-less only, and that is what makes the lookup resolve rather than
// guess. A bare `foo()` in Go can only reach a package-level function, so
// indexing methods under the same bare name let an unrelated
// `(s *Store) mode()` answer for a call to `mode()` — a binder in a body the
// worker never reaches, reported as the worker's own. Go forbids two
// package-level functions of one name in a package, so every name that
// resolves here resolves to exactly one body.
func indexFunctions(out map[string]string, text string) {
	locs := anyFunc.FindAllStringSubmatchIndex(text, -1)
	for i, loc := range locs {
		if loc[2] >= 0 {
			continue // a method: unreachable by a bare call
		}
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		out[text[loc[4]:loc[5]]] = text[loc[0]:end]
	}
}

// anyFunc captures a top-level declaration's NAME, and separately whether it
// had a receiver, so a method can be skipped: the first group is the receiver
// clause when there is one, the second the name.
var anyFunc = regexp.MustCompile(`(?m)^func (\([^)]*\) )?(\w+)\(`)

// withCalledHelpers is the Work body plus the bodies of the compose-package
// functions it calls, one level deep.
//
// One level, because a binder is a HELPER by nature — it takes a context,
// returns one carrying the principal, and reads better beside the pass it names
// than inlined into a Work method. Two of the three workers waived here for
// weeks were binding correctly one call away, and a gate that produces waivers
// for correct code is a gate whose waiver list stops meaning anything.
//
// WHAT THIS CANNOT SEE, so nobody has to rediscover it: a helper that binds an
// actor for its own path leaves this body looking bound, so a SECOND, unbound
// gated call in the same Work method would pass. That is a narrower hole than
// the one it closes — every helper-bound worker was invisible before — and it
// is the shape to plant a case for if this gate is ever extended again. It also
// follows names, not call graphs: a call into another package (the automation
// scanner, the webhook deliverer) is not read at all, which is why a worker
// whose only binding lives in another module still needs an entry above.
//
// What it does NOT do is guess which body a name means — indexFunctions says
// why, and it is the reason this follow resolves rather than guesses.
func withCalledHelpers(body string, pkg map[string]string) string {
	var b strings.Builder
	b.WriteString(body)
	for _, call := range callee.FindAllStringSubmatch(afterSignature(body), -1) {
		if helper, ok := pkg[call[1]]; ok {
			b.WriteString(helper)
		}
	}
	return b.String()
}

// afterSignature drops a method's own declaration line, so the method's NAME is
// not read as a call it makes.
//
// It matters because the names here are shared: every worker's entry point is
// called Work, so reading the signature pulled every other worker's Work body
// in — and one of those binds an actor, which made this gate answer yes for a
// worker that binds none. It read as a working follow-one-level and was a
// self-satisfying lookup.
func afterSignature(body string) string {
	if open := strings.Index(body, "{\n"); open >= 0 {
		return body[open:]
	}
	return body
}

// callee captures a bare call's name. A selector call (`x.Method(`) is
// deliberately not matched: this gate reads the compose package's own source,
// and a method on somebody else's type is not in it.
var callee = regexp.MustCompile(`(?:^|[^.\w])(\w+)\(`)

// The follow resolves a name; it does not guess which body the name means.
//
// The rule and its reason are on indexFunctions; this is the case that fails
// when the rule goes. What it costs to lose is the gate's own failure
// direction: a worker reported as bound on the strength of a binder in a body
// it never runs, which is a waiver nobody asked for, granted silently.
func TestTheFollowDoesNotResolveABareCallToAMethod(t *testing.T) {
	t.Parallel()
	const pkg = `package compose

func (s *Store) mode(ctx context.Context) context.Context {
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem})
	return ctx
}
`
	const worker = `func (w *someWorker) Work(ctx context.Context) error {
	db := database.BindTo(w.pool, ws)
	return mode(ctx, db)
}
`
	index := map[string]string{}
	indexFunctions(index, pkg)
	if _, indexed := index["mode"]; indexed {
		t.Fatal("a method was indexed under its bare name, so a call no worker can make answers for one it does")
	}
	if actorBinders.MatchString(withCalledHelpers(worker, index)) {
		t.Error("the worker read as binding an actor on the strength of a method it cannot reach by that name")
	}
}
