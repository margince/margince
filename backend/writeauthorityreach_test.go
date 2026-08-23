// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

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

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
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
	"internal/modules/privacy:anonymizePerson": "the retention engine anonymizes a person in place because their retention clock expired, under a policy that names the table. The subject is chosen by age; there is no caller asking for this row and no authority to test beyond the policy itself",
	"internal/modules/privacy:anonymizeLead":   "the lead half of the same retention pass, chosen the same way",
	"internal/modules/privacy:archiveDeal":     "the deal half of the same retention pass: a deal past its window is archived because of when it closed, not because somebody asked",
	"internal/modules/deals:SweepWorkspace":    "the close-date hygiene pass reads every open deal in the workspace and corrects or stages the ones whose expected close has gone stale. It runs as the workspace's own system principal with no human behind it, and narrowing it to one seat's rows would leave every other rep's forecast uncorrected while the sweep reported success",

	// Capture. A colleague's mail must reach the record it is about, which is
	// the case the access model deliberately keeps open: Rep B emailing Rep A's
	// company still captures, links, and enriches what the message evidences.
	// Gating these on write authority over the incumbent would silently fork the
	// record — a second person or company for the same human — which is worse
	// for Rep A than the write it would have prevented.
	"internal/modules/people:EnsureCounterparty":    "capture resolving the person and company a captured message is about, and attaching it. The row it lands on is frequently a colleague's, by design: refusing would not protect that record, it would create a duplicate of it alongside. What this may write to an incumbent is bounded instead — a name fills only where the column is still empty, and a header carrying an impersonation tell teaches an existing record nothing",
	"internal/modules/people:ApplySignatureFields":  "the signature enricher's fill-only-empty write, driven by the capture sweep off evidence the message itself carried. Every column is guarded by an IS NULL predicate, so a value a human entered is never replaced, and the same duplicate-forking argument as EnsureCounterparty applies to the record it lands on",
	"internal/modules/people:ApplyColdStartProfile": "the cold-start accept path, which runs as a system principal after a human approved the proposal. The authority question was asked at DECIDE time, where the approvals engine takes auth.EnsureWritableLive on the target row; asking it again here under a principal that has no human behind it would test nothing",
	"internal/modules/people:applyColdStartTx":      "the same accept path's transactional half, reached only from ApplyColdStartProfile",
	"internal/modules/people:writeOrgColumn":        "the cold-start writer's per-column half, reached only from applyColdStartTx and bounded by the same accepted proposal",
	"internal/modules/people:EnsureCounterpartyTx":  "capture's resolution running inside the caller's transaction, reached only from EnsureCounterparty",
	"internal/modules/people:ensurePerson":          "the person half of that same capture resolution",
	"internal/modules/people:fillMissingPersonName": "capture's name fill on an incumbent: every column carries its own IS NULL predicate, so a name a human entered is never replaced and a re-run converges instead of flapping. Reached only from ensurePerson",
	"internal/modules/people:applySignatureField":   "the signature enricher's per-field half, reached only from ApplySignatureFields and guarded by the same IS NULL predicates",
	"internal/modules/people:touchPerson":           "stamps updated_at on a person a LinkedIn match just resolved, so the row's own change is visible to anything reading it. Reached only from the apply path, which is a system principal driving an accepted match",

	// Merges and sweeps whose authority is taken once, at the top.
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

// guardedCallersOf names the functions whose write is already answered for by a
// caller: some function in the package that writes a shareable record, reaches a
// write-authority probe, and calls this one. The probe then runs before this
// write, so the helper is covered rather than exempt.
//
// A helper called only from UNGUARDED callers stays in the census, which is the
// point — otherwise a whole chain could go unprobed with every link pointing at
// the next as its excuse.
func guardedCallersOf(byReceiver map[string]map[string]*writeAuthorityFn,
	tables map[string]bool, vocab map[string]bool,
) map[string]bool {
	known := map[string]bool{}
	for _, fns := range byReceiver {
		for name := range fns {
			known[name] = true
		}
	}
	covered := map[string]bool{}
	for recv := range byReceiver {
		visible := visibleWriteAuthorityFns(byReceiver, recv)
		for name, info := range visible {
			if len(writtenTablesUnder(visible, name, tables, map[string]bool{})) == 0 {
				continue
			}
			if !reachesWriteAuthorityProbe(visible, name, vocab, map[string]bool{}) {
				continue
			}
			for callee := range info.calls {
				if known[callee] && callee != name {
					covered[callee] = true
				}
			}
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
