// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every worker that reaches a store must bind an actor as well as a workspace.

import (
	"go/scanner"
	"go/token"
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
	"webhookRetryWorker": "the retry sweep resolves no principal of its own and reaches no RBAC gate under one. Its path is dueRetries → loadTarget → stillVisible → markVisibilityRevoked → deliverOnce, and none of those asks auth.Require; the two entry points in that store which do (ListDeliveries, requireReplay) are the human dead-letter surfaces this pass never touches. The one authority question it DOES ask — may this subscription's owner still see the record this delivery carries — is asked under the OWNER's principal, resolved per delivery from the row (Deliverer.canSee binds it with principal.WithActor from EffectiveRBAC). An actor bound here would be a second, wrong answer to that question. SweepOnce binds the tenant from its own handle and nothing else. Held by webhookretry_unreachable_integration_test.go, whose job-runner test calls the sweep through this worker and whose other tests call it with a bare context",
})

// A worker may construct its store through a helper; trace the data handle too.
var storeBuilders = regexp.MustCompile(`(providerRunStore|InstallationDB|database\.BindTo)\(`)

// A nested tool’s passport does not authorize the worker that called it.
var actorBinders = regexp.MustCompile(
	`\w+\s*(=|:=)\s*(forColleague|providerJobActor|reconcileWorkerCtx|partSlimJobActor|geocodeJobActor|technicalActor|vatCheckJobActor|principal\.(WithActor|SystemActing))\(`)

// Work signatures may omit the job argument; the receiver identifies the worker.
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

// Only unqualified package functions can inherit their caller’s actor binding.
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

// Follow local helpers so a binding one call below Work counts.
// Imported functions and methods need their own binding.
func withCalledHelpers(body string, pkg map[string]string) string {
	var b strings.Builder
	b.WriteString(body)
	bound := namesBoundIn(body)
	for _, call := range callee.FindAllStringSubmatch(afterSignature(body), -1) {
		// A local name shadows the package helper and cannot borrow its body.
		if bound[call[1]] {
			continue
		}
		if helper, ok := pkg[call[1]]; ok {
			b.WriteString(helper)
		}
	}
	return b.String()
}

// Parameters and control-clause declarations also shadow package helpers.
func namesBoundIn(body string) map[string]bool {
	bound := map[string]bool{}
	for _, m := range localBinding.FindAllStringSubmatch(codeOnly(body), -1) {
		for part := range strings.SplitSeq(m[1], ",") {
			// The comma-separated identifiers precede their shared type.
			words := strings.Fields(part)
			if len(words) == 0 {
				continue
			}
			bound[words[len(words)-1]] = true
		}
	}
	return bound
}

// Only the left side declares a shadow; selector assignments do not.
var localBinding = regexp.MustCompile(`(?:^|[^\w.])(?:var\s+)?([\w][\w, ]*?)\s*:?=[^=]`)

// Quoted source and comments cannot bind an actor.
func codeOnly(body string) string {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(body))
	var sc scanner.Scanner
	// A nil handler drops scan errors, which is right for a fragment: this
	// starts mid-declaration and is read for its shape, not compiled.
	sc.Init(file, []byte(body), nil, scanner.ScanComments)
	out := []byte(body)
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.COMMENT && tok != token.STRING && tok != token.CHAR {
			continue
		}
		start := fset.Position(pos).Offset
		for i := start; i < start+len(lit) && i < len(out); i++ {
			out[i] = ' '
		}
	}
	return string(out)
}

// A method’s own name cannot count as a binder in its body.
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

// Helper lookup preserves its declaration’s package and receiver.
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

// A shadowed helper cannot lend the worker an actor.
func TestTheFollowDoesNotBorrowAShadowedHelpersBody(t *testing.T) {
	t.Parallel()
	// The helper binds in the ASSIGNMENT form actorBinders recognises — a bare
	// `return principal.WithActor(…)` is not one, and a fixture written that
	// way would pass whatever the guard did.
	const pkg = `package compose

func bindActor(ctx context.Context) context.Context {
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem})
	return ctx
}
`
	const worker = `func (w *someWorker) Work(ctx context.Context) error {
	db := database.BindTo(w.pool, ws)
	bindActor := w.transform
	ctx = bindActor(ctx)
	return run(ctx, db)
}
`
	index := map[string]string{}
	indexFunctions(index, pkg)
	if _, indexed := index["bindActor"]; !indexed {
		t.Fatal("the fixture's package helper was not indexed, so this case proves nothing")
	}
	if actorBinders.MatchString(withCalledHelpers(worker, index)) {
		t.Error("the worker read as binding an actor through a name it had shadowed with a field of its own")
	}
}

// Control-clause bindings shadow helpers before their bodies run.
func TestTheFollowSeesAShadowDeclaredInAControlClause(t *testing.T) {
	t.Parallel()
	const pkg = `package compose

func bindActor(ctx context.Context) context.Context {
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem})
	return ctx
}
`
	for _, tc := range []struct {
		name, body string
	}{
		{"an if initializer", "\tif bindActor := w.transform; bindActor != nil {\n\t\tctx = bindActor(ctx)\n\t}\n"},
		{"a for range", "\tfor _, bindActor := range w.transforms {\n\t\tctx = bindActor(ctx)\n\t}\n"},
		{"a switch initializer", "\tswitch bindActor := w.transform; {\n\tdefault:\n\t\tctx = bindActor(ctx)\n\t}\n"},
		// The one that is not at the start of its line, which is why the scan
		// is not anchored to one.
		{"an else-if initializer", "\tif w.pool == nil {\n\t\treturn nil\n\t} else if bindActor := w.transform; bindActor != nil {\n\t\tctx = bindActor(ctx)\n\t}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worker := "func (w *someWorker) Work(ctx context.Context) error {\n" +
				"\tdb := database.BindTo(w.pool, ws)\n" + tc.body + "\treturn run(ctx, db)\n}\n"
			index := map[string]string{}
			indexFunctions(index, pkg)
			if actorBinders.MatchString(withCalledHelpers(worker, index)) {
				t.Error("the worker read as binding an actor through a name it had shadowed in a control clause")
			}
		})
	}
}

// Quoted code and comments must not manufacture local declarations.
func TestTheFollowIgnoresAnAssignmentInsideACommentOrAString(t *testing.T) {
	t.Parallel()
	const pkg = `package compose

func bindActor(ctx context.Context) context.Context {
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem})
	return ctx
}
`
	for _, tc := range []struct {
		name, line string
	}{
		{"a line comment", "\t// A worker may write bindActor := w.transform to swap the pass.\n"},
		{"a block comment", "\t/* bindActor := w.transform\n\t   is what an older draft did. */\n"},
		{"a string literal", "\tslog.Info(\"bindActor := w.transform is not supported\")\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worker := "func (w *someWorker) Work(ctx context.Context) error {\n" +
				"\tdb := database.BindTo(w.pool, ws)\n" + tc.line +
				"\tctx = bindActor(ctx)\n\treturn run(ctx, db)\n}\n"
			index := map[string]string{}
			indexFunctions(index, pkg)
			if _, indexed := index["bindActor"]; !indexed {
				t.Fatal("the fixture's package helper was not indexed, so this case proves nothing")
			}
			if !actorBinders.MatchString(withCalledHelpers(worker, index)) {
				t.Error("the worker read as UNBOUND because prose in its body was taken for a binding, " +
					"so the follow never reached the helper it really calls")
			}
		})
	}
}

func TestColleagueJobsRecognizeBothBoundContexts(t *testing.T) {
	bound := "books, work, err = forColleague(ctx, users, requester, job)"
	unbound := "forColleague(ctx, users, requester, job)"
	if !actorBinders.MatchString(bound) || actorBinders.MatchString(unbound) {
		t.Fatal("colleague binding must retain its returned contexts")
	}
}
