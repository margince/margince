// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

// Every write of a shareable record reaches a write-authority probe.
//
// This is the EXISTENCE half of a pair. Its sibling in writeauthority_test.go
// judges the SPELLING of probes that are there — a mutation that gates on
// visibility alone accepts a read share as a licence to write. Neither of the
// two gates that existed before this one could see a mutation with NO row probe
// at all: TestEveryStoreEntryPointIsAuthGated admits any mention of the auth
// package, so a bare object gate satisfies it, and the spelling gate iterates
// probes, so a function holding none produces no sites and passes silently.
//
// The gap matters here more than it would elsewhere. Person, organization,
// lead, deal and project are read by every seat in the workspace, so the write
// predicate is the ONLY thing that scopes them — a mutation that forgets it is
// not caught by a narrower read, it simply works for everyone.
//
// The two gates keep separate waiver maps on purpose. A waiver here says "this
// write needs no row probe"; one there says "this probe decides something
// else". Merging them would let a waiver written for one obligation quietly
// discharge the other.

import (
	"go/ast"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// wantMinimumShareableWriters is the anti-vacuity floor, and it is the check
// that matters most: an extractor which stops recognising this tree's SQL finds
// no subjects and reports nothing, which reads exactly like a clean tree.
// Twenty-four files hold a literal UPDATE or DELETE of one of the five tables
// today, and the patch and lock shapes add more on top, so the floor sits well
// below the real count — losing one writer stays an ordinary change and only a
// collapse is a finding.
const wantMinimumShareableWriters = 15

// writesWithoutARowProbe ratifies a write of a shareable record that reaches no
// write-authority probe. Each entry says why a probe would be WRONG there, not
// merely inconvenient: where the probe is simply a no-op today it was added
// instead, because a no-op costs nothing and closes the drift.
var writesWithoutARowProbe = gatekit.Waive(map[string]string{
	// Rows chosen by a CLOCK, not by a caller. A row-scope probe would narrow
	// each sweep to whichever seat's rows the running principal could write and
	// leave every other subject's record untouched — for retention that is the
	// storage-limitation failure the pass exists to prevent, and for the rest it
	// would make a hygiene pass silently partial.
	"internal/modules/privacy:anonymizePerson":         "the retention engine anonymizes a person in place because their retention clock expired, under a policy that names the table. The subject is chosen by age; there is no caller asking for this row and no authority to test beyond the policy itself",
	"internal/modules/privacy:anonymizeLead":           "the lead half of the same retention pass, chosen the same way",
	"internal/modules/privacy:archiveDeal":             "the deal half of the same retention pass: a deal past its window is archived because of when it closed, not because somebody asked",
	"internal/modules/people:promoteIfWorkspaceScoped": "a row probe here would refuse the write it exists to make. The row is owner-scoped, which platform/auth reads as capture privacy — invisible to every principal but its owner, an admin included — so EnsureWritable answers no for exactly the rows this promotes. What decides the promotion is not the caller's authority over the row but the verdict that judged its sender a business counterparty, and the surrounding ensurePerson already takes auth.Require(person, create) for the caller who may mint one at all. The write is one-directional and idempotent: it moves owner to workspace and never back",
	"internal/modules/deals:SweepWorkspace":            "the close-date hygiene pass reads every open deal in the workspace and corrects or stages the ones whose expected close has gone stale. It runs as the workspace's own system principal with no human behind it, and narrowing it to one seat's rows would leave every other rep's forecast uncorrected while the sweep reported success",

	// The shared fill, probed by BOTH of its callers rather than by itself.
	// ApplySitePersonFields takes EnsureWritableLive and SKIPS an out-of-scope
	// match (staging a lead instead); the vCard import takes the same probe and
	// REPORTS the skip on that card's own row of the report. The two answers
	// are different because the surfaces are — a site read must not abort a
	// whole page over one employee, and an import must not silently omit a
	// card — and a probe inside the shared writer could only give one of them.
	"internal/modules/people:fillSitePersonFields": "the fill both the site read and the vCard import share, reached only past each caller's own auth.EnsureWritableLive on the matched person. The probe is in the callers because what a refusal MEANS differs between them: the site read skips to staging, the import reports the card as skipped. A probe here would have to pick one of those answers for both",

	// Capture. A colleague's mail must reach the record it is about, which is
	// the case the access model deliberately keeps open: Rep B emailing Rep A's
	// company still captures, links, and enriches what the message evidences.
	// Gating these on write authority over the incumbent would silently fork the
	// record — a second person or company for the same human — which is worse
	// for Rep A than the write it would have prevented.
	"internal/modules/people:EnsureCounterparty":    "capture resolving the person and company a captured message is about, and attaching it. The row it lands on is frequently a colleague's, by design: refusing would not protect that record, it would create a duplicate of it alongside. What this may write to an incumbent is bounded instead — a name fills only where the column is still empty, and a header carrying an impersonation tell teaches an existing record nothing",
	"internal/modules/people:writeOrgColumn":        "the cold-start writer's per-column half, reached only from applyColdStartTx and bounded by the same accepted proposal",
	"internal/modules/people:EnsureCounterpartyTx":  "capture's resolution running inside the caller's transaction, reached only from EnsureCounterparty",
	"internal/modules/people:ensurePerson":          "the person half of that same capture resolution",
	"internal/modules/people:fillMissingPersonName": "capture's name fill on an incumbent, reached only from ensurePerson. The write predicate is first_name IS NULL AND last_name IS NULL, so it only ever completes a record nobody has split into parts; full_name moves with them, and only where it still equals one of the two parts alone — a record displaying `Lars` while its columns say Lars Jankowfsky. A full_name a human typed that differs from both parts is left alone. It is a completion, not an overwrite, which is why the row it lands on being a colleague's is not the disclosure a probe would prevent",
	"internal/modules/people:touchPerson":           "stamps updated_at on a person a LinkedIn match just resolved, so the row's own change is visible to anything reading it. Reached only from the apply path, which is a system principal driving an accepted match",

	// Merges and sweeps whose authority is taken once, at the top.
	"internal/modules/projects:recordPhaseTransition":          "one project phase move, written for two callers whose authority is taken differently: AdvanceProject probes the project itself, and the won-deal delivery start took it on the DEAL — ensureProjectAttachable demands write authority over the project at the moment a deal is bound to one, which is the act that later authorizes advancing its phase. That is a real gate this walk cannot see, because it ran in a different function on a different record",
	"internal/modules/projects:StartDeliveryForWonDeal":        "moves a won deal's project into delivery, from the deal's own close. The authority is the DEAL's and was taken there; binding the deal to the project earlier demanded write authority over the project through ensureProjectAttachable, which is what makes this move authorized without a second probe on a record the closing caller may not own",
	"internal/modules/people:applyEvidenceFieldsWithOverwrite": "the company evidence writer, reached from three paths that each take the probe before calling it: the cold-start accept (gateResolvedColdStartTarget, on the organization the URL resolved to), Enrich, and the site-read confirmation",

	"internal/modules/people:recomputeUnderOverrideTx": "the sticky-override branch of recomputeLeadScoreTx, reached from nowhere else. Its four callers each take the row probe before they get here — RecomputeLeadScore, both manual-signal paths and UpdateLead — and the probe belongs at those entry points rather than in a branch, because an archived lead is a silent no-op to the workflow lane and a 404 to a human",

	"internal/modules/people:absorbOrgReferences":    "the organization merge's cascade: it re-points the deals, projects and child companies that NAMED the absorbed company at the survivor. The merge itself takes write authority on BOTH organizations through mergePair before anything moves, and the rows re-pointed here are not the ones being decided about — refusing to re-point a deal the caller cannot write would leave it pointing at a company that no longer exists",
	"internal/modules/people:RouteLead":              "routing assigns an ownerless lead to a chosen rep, and an ownerless row is nobody's to write by construction (the write arm renders no owner_id IS NULL branch) — auth.EnsureWritable here could only ever refuse. It self-guards instead: a lead that already has an owner returns already_owned before anything is written, so routing can never overwrite a human's assignment. The claim-then-write primitive this shape wants is storekit.ClaimOwnership, which takes the claimant as `me` and so does not fit an assignment to a third party",
	"internal/modules/deals:sweepWorkspace":          "the close-date sweep's inner pass, reached only from SweepWorkspace",
	"internal/modules/deals:correct":                 "one close-date correction inside that sweep",
	"internal/modules/deals:apply":                   "the correction's write, reached only from correct",
	"internal/modules/privacy:anonymizePersonRecord": "the retention engine's person write, reached only from the pass that selected the row by its expired clock",
	"internal/modules/privacy:anonymizeLeadTwins":    "the same pass's lead half",
})

// writeAuthorityVocabulary derives the spellings that answer "may this caller
// CHANGE this row" from platform/auth itself, rather than restating them.
//
// Every one of them funnels through ensureWriteAuthority or renders
// writeAuthorityPredicateAs, so that reachability IS the definition, and a
// ninth spelling added to that package is honoured by this gate the day it
// lands. A restated list would go stale silently, and staleness here looks
// exactly like a clean tree.
func writeAuthorityVocabulary(t *testing.T) map[string]bool {
	t.Helper()
	files := tierFiles(t, filepath.Join("internal", "platform", "auth"))
	bodies := map[string]*writeAuthorityFn{}
	for _, src := range files {
		for _, decl := range src.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			info := &writeAuthorityFn{calls: map[string]bool{}, requires: map[string]bool{}}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.Ident:
					info.calls[n.Name] = true
				case *ast.SelectorExpr:
					info.calls[n.Sel.Name] = true
				}
				return true
			})
			bodies[fn.Name.Name] = info
		}
	}
	vocab := map[string]bool{}
	for name := range bodies {
		if !ast.IsExported(name) {
			continue
		}
		if reachesWriteAuthorityCore(bodies, name, map[string]bool{}) {
			vocab[name] = true
		}
	}
	// Fail loudly rather than reporting every writer as unguarded: a collapsed
	// vocabulary would make this gate scream about the whole tree, which reads
	// as a broken gate and gets waived wholesale.
	if !vocab["EnsureWritable"] {
		t.Fatalf("the write-authority vocabulary derived from platform/auth is %v, which does not "+
			"contain EnsureWritable — the derivation has lost its source", vocab)
	}
	return vocab
}

// reachesWriteAuthorityCore walks platform/auth's own call graph to the two
// functions that ARE the write question.
func reachesWriteAuthorityCore(fns map[string]*writeAuthorityFn, name string, seen map[string]bool) bool {
	if seen[name] {
		return false
	}
	seen[name] = true
	info, ok := fns[name]
	if !ok {
		return false
	}
	if info.calls["ensureWriteAuthority"] || info.calls["writeAuthorityPredicateAs"] {
		return true
	}
	for callee := range info.calls {
		if reachesWriteAuthorityCore(fns, callee, seen) {
			return true
		}
	}
	return false
}

func TestEveryWriteToAShareableRecordReachesAWriteAuthorityProbe(t *testing.T) {
	t.Parallel()
	defer writesWithoutARowProbe.AssertAllMatched(t)

	tables := shareableTables(t)
	pkgs := writeAuthorityIndex(t, tables)
	vocab := writeAuthorityVocabulary(t)

	subjects := 0
	// One verdict per package and function name. The index holds a view per
	// receiver and merges the package-level bucket into each, so a name reachable
	// from several receivers is judged repeatedly; reporting each pass would bury
	// the real findings under copies of themselves. A function guarded under ANY
	// view is guarded — the probe is in its body either way.
	judged := map[string]bool{}
	guarded := map[string]bool{}
	type finding struct{ dir, name, written string }
	var findings []finding
	for dir, byReceiver := range pkgs {
		// Every writer is judged, not only the entry points. A function is
		// suppressed ONLY when a caller of it in the same package is itself a
		// judged writer that IS guarded — then the probe genuinely runs before
		// this write, and reporting the helper too would ask for a second
		// waiver where there is one decision. Skipping every called function
		// outright would be a hole: `UpdatePerson` on the store shares its name
		// with the HTTP handler that calls it, so a name-based roots filter
		// dropped the real store method out of the census entirely.
		guardedCallers := guardedCallersOf(byReceiver, tables, vocab)
		for recv := range byReceiver {
			visible := visibleWriteAuthorityFns(byReceiver, recv)
			for name := range visible {
				if guardedCallers[name] {
					continue
				}
				written := writtenTablesUnder(visible, name, tables, map[string]bool{})
				if len(written) == 0 {
					continue
				}
				key := dir + ":" + name
				if reachesWriteAuthorityProbe(visible, name, vocab, map[string]bool{}) {
					guarded[key] = true
				}
				if judged[key] {
					continue
				}
				judged[key] = true
				subjects++
				findings = append(findings, finding{dir, name, strings.Join(dedupe(written), ", ")})
			}
		}
	}
	for _, f := range findings {
		if guarded[f.dir+":"+f.name] || writesWithoutARowProbe.Waived(t, f.dir+":"+f.name) {
			continue
		}
		{
			dir, name, written := f.dir, f.name, f.written
			t.Errorf("%s: %s changes %s and reaches no write-authority probe — a mutation of a "+
				"shareable record takes auth.EnsureWritable/EnsureWritableLive/WritableBy on the row "+
				"before writing it, or the exception is ratified in writesWithoutARowProbe with the "+
				"reason a row probe would be wrong there",
				dir, name, written)
		}
	}
	if subjects < wantMinimumShareableWriters {
		t.Fatalf("only %d writers of a shareable record found in %s, want at least %d — the extractor "+
			"has lost its source and this gate is reporting a clean tree it never read",
			subjects, modulesDir, wantMinimumShareableWriters)
	}
}

// TestNoMutationOfAShareableRecordHidesItsTableFromThisGate is the tripwire
// under the census. Every mutation in the module tree names its table as a
// literal today, which is what lets the parsing above be exact; a table
// assembled at run time would fall out of the census silently, and silence is
// indistinguishable from safety.
func TestNoMutationOfAShareableRecordHidesItsTableFromThisGate(t *testing.T) {
	t.Parallel()
	for dir, byReceiver := range writeAuthorityIndex(t, shareableTables(t)) {
		for _, fns := range byReceiver {
			for name, info := range fns {
				for _, stmt := range info.dynamicWrite {
					t.Errorf("%s: %s mutates a table this gate cannot name (%q) — every mutation in the "+
						"module tree spells its table as a literal, which is what makes this census exact. "+
						"Teach mutatedTables to resolve the target rather than letting the write fall out "+
						"of the gate's sight", dir, name, stmt)
				}
			}
		}
	}
}

// guardedCallersOf names the functions whose write is already answered for: EVERY
// caller of them in the package writes a shareable record and reaches a
// write-authority probe. The probe then runs before this write on every path
// that reaches it, so the helper is covered rather than exempt.
//
// It has to be every caller, not any. Suppressing on one guarded caller is how a
// helper reached by a probed wrapper AND by an unprobed path reads as safe: the
// wrapper answers for the write, and the other path walks straight past it.
// PromoteOrgNameTx was exactly that shape — its wrapper took the probe while
// compose called the Tx form directly on both of its real paths.
//
// A helper with any caller that does NOT write, or does not reach a probe, stays
// in the census, which is the point.
func guardedCallersOf(byReceiver map[string]map[string]*writeAuthorityFn,
	tables map[string]bool, vocab map[string]bool,
) map[string]bool {
	known := map[string]bool{}
	for _, fns := range byReceiver {
		for name := range fns {
			known[name] = true
		}
	}
	callers, guardedBy := map[string]int{}, map[string]int{}
	for recv := range byReceiver {
		visible := visibleWriteAuthorityFns(byReceiver, recv)
		for name, info := range visible {
			writes := len(writtenTablesUnder(visible, name, tables, map[string]bool{})) > 0
			probed := writes && reachesWriteAuthorityProbe(visible, name, vocab, map[string]bool{})
			for callee := range info.calls {
				if !known[callee] || callee == name {
					continue
				}
				callers[callee]++
				if probed {
					guardedBy[callee]++
				}
			}
		}
	}
	covered := map[string]bool{}
	for name, total := range callers {
		if total > 0 && guardedBy[name] == total {
			covered[name] = true
		}
	}
	return covered
}

// writtenTablesUnder collects the shareable tables a function changes, its
// same-package callees included.
func writtenTablesUnder(fns map[string]*writeAuthorityFn, name string,
	tables map[string]bool, seen map[string]bool,
) []string {
	if seen[name] {
		return nil
	}
	seen[name] = true
	info, ok := fns[name]
	if !ok {
		return nil
	}
	var out []string
	for table := range info.writes {
		if tables[table] {
			out = append(out, table)
		}
	}
	for callee := range info.calls {
		out = append(out, writtenTablesUnder(fns, callee, tables, seen)...)
	}
	return out
}

// reachesWriteAuthorityProbe answers whether the function, or anything it calls
// in the same package, asks the write question.
func reachesWriteAuthorityProbe(fns map[string]*writeAuthorityFn, name string,
	vocab map[string]bool, seen map[string]bool,
) bool {
	if seen[name] {
		return false
	}
	seen[name] = true
	info, ok := fns[name]
	if !ok {
		return false
	}
	// A probe is recorded as a probeSite rather than as a plain call edge, so
	// this reads the site list; info.calls never names one.
	for _, site := range info.probes {
		if vocab[site.spelling] {
			return true
		}
	}
	for callee := range info.calls {
		if reachesWriteAuthorityProbe(fns, callee, vocab, seen) {
			return true
		}
	}
	return false
}

// dedupe collapses a table named by several statements in one function.
func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
