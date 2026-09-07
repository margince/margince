// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

// The defect the reach census exists for, planted and run.
//
// TestEveryWriteToAShareableRecordReachesAWriteAuthorityProbe is green over
// this tree, and a green census proves nothing on its own: an extractor that
// stopped recognising a write, a call edge or a probe would be green in exactly
// the same way. So the judgement is run over sources whose verdict is known,
// #1881's own shape among them.
//
// #1881 was a mutating entry point with ZERO row probes — DisposeDedupeCandidate
// wrote a dedupe pair on the object grant alone — and the two gates that stood
// in front of it both said PASS: the auth-gating census admits any mention of
// the auth package, and the spelling census iterates the probes a function
// took, so a function holding none produced no sites at all. Silence read as
// safety. These cases pin that it no longer can.
//
// The real judgement is called, never a copy of it. A planted test that
// reimplemented the walk would prove the copy right and say nothing about the
// gate.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// plantedDir is where the synthetic package pretends to live. It only has to
// look like the module tier; nothing reads the path off disk.
const plantedDir = "internal/modules/planted"

// judgePlantedWrite runs the reach census over one synthetic source and answers,
// per function name, whether the gate saw a write of a shareable record and
// whether it found that write guarded.
//
// It is the gate's own pipeline — the same index, the same caller suppression,
// the same reachability walk — over a file this test wrote instead of over the
// tree. The vocabulary and the shareable table set still come from
// platform/auth, because a planted case that invented its own would stop being
// a test of what ships.
func judgePlantedWrite(t *testing.T, source string) (written, guarded map[string]bool) {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, plantedDir+"/planted.go", source, 0)
	if err != nil {
		t.Fatalf("parsing the planted source: %v", err)
	}
	src := tierFile{Path: plantedDir + "/planted.go", File: parsed, fset: fset}

	tables := shareableTables(t)
	vocab := writeAuthorityVocabulary(t)
	consts := packageStringConsts(src)

	byReceiver := map[string]map[string]*writeAuthorityFn{}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		recv := receiverName(fn)
		if byReceiver[recv] == nil {
			byReceiver[recv] = map[string]*writeAuthorityFn{}
		}
		info := &writeAuthorityFn{requires: map[string]bool{}, calls: map[string]bool{}}
		byReceiver[recv][fn.Name.Name] = info
		indexWriteAuthorityBody(fn, info, tables, consts,
			probeSite{dir: plantedDir, recv: recv, fn: fn.Name.Name, file: src.Path}, src)
	}

	written, guarded = map[string]bool{}, map[string]bool{}
	guardedCallers := guardedCallersOf(byReceiver, tables, vocab)
	for recv := range byReceiver {
		visible := visibleWriteAuthorityFns(byReceiver, recv)
		for name := range visible {
			if guardedCallers[name] {
				continue
			}
			if len(writtenTablesUnder(visible, name, tables, map[string]bool{})) == 0 {
				continue
			}
			written[name] = true
			if reachesWriteAuthorityProbe(visible, name, vocab, map[string]bool{}) {
				guarded[name] = true
			}
		}
	}
	return written, guarded
}

// plantedSource wraps one or more function bodies in a package the parser will
// accept. The imports are named rather than elided because the index resolves a
// probe by its SELECTOR — `auth.EnsureWritable` — and a file that never imported
// auth would be judging a different spelling than the tree does.
func plantedSource(bodies string) string {
	return `package planted

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type Store struct{}
` + bodies
}

// #1881, as it stood: a mutating entry point that takes the OBJECT grant and no
// row probe at all. It is the case both earlier gates reported clean.
func TestAMutationWithNoRowProbeAtAllIsAFinding(t *testing.T) {
	t.Parallel()
	written, guarded := judgePlantedWrite(t, plantedSource(`
func (s *Store) DisposePair(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `+"`UPDATE person SET display_name = $1 WHERE id = $2`"+`, "x", id)
	return err
}
`))
	if !written["DisposePair"] {
		t.Fatal("the census did not see a write of `person` at all, so it is judging nothing here — " +
			"the extractor has stopped recognising this tree's UPDATE shape")
	}
	if guarded["DisposePair"] {
		t.Error("a mutation holding auth.Require and no row probe was judged guarded, which is #1881 " +
			"exactly: object admission answers whether the caller may change people, never whether " +
			"they may change THIS person, and a gate that accepts it reads green over the defect it " +
			"was written for")
	}
}

// The mirror, so the case above cannot pass against a census that finds
// everything unguarded.
func TestAMutationThatTakesTheRowProbeIsNotAFinding(t *testing.T) {
	t.Parallel()
	_, guarded := judgePlantedWrite(t, plantedSource(`
func (s *Store) DisposePair(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if err := auth.EnsureWritable(ctx, tx, "person", id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `+"`UPDATE person SET display_name = $1 WHERE id = $2`"+`, "x", id)
	return err
}
`))
	if !guarded["DisposePair"] {
		t.Error("a mutation taking auth.EnsureWritable on the row was judged unguarded — the " +
			"vocabulary derived from platform/auth no longer recognises its own spelling, and this " +
			"gate would now report the whole tier as a finding")
	}
}

// The probe an ANCESTOR takes still answers for the write, or every correct
// helper in the tier is a finding.
func TestAWriteUnderAProbedCallerIsNotAFinding(t *testing.T) {
	t.Parallel()
	written, guarded := judgePlantedWrite(t, plantedSource(`
func (s *Store) Dispose(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if err := auth.EnsureWritable(ctx, tx, "person", id); err != nil {
		return err
	}
	return writePair(ctx, tx, id)
}

func writePair(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx, `+"`UPDATE person SET display_name = $1 WHERE id = $2`"+`, "x", id)
	return err
}
`))
	if !guarded["Dispose"] {
		t.Error("the entry point's own probe did not answer for the write it makes two frames down")
	}
	if written["writePair"] {
		t.Error("the helper was reported separately although its only caller is a judged writer that " +
			"IS guarded — the probe genuinely runs before this write, and asking for a second waiver " +
			"where there is one decision is how a gate teaches its reader to waive")
	}
}

// The PromoteOrgNameTx shape: a helper reached by a probed wrapper AND by an
// unprobed path. Suppressing on ANY guarded caller is what let that one through.
func TestAHelperWithOneUnprobedCallerStaysAFinding(t *testing.T) {
	t.Parallel()
	written, guarded := judgePlantedWrite(t, plantedSource(`
func (s *Store) Dispose(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if err := auth.EnsureWritable(ctx, tx, "person", id); err != nil {
		return err
	}
	return writePair(ctx, tx, id)
}

func (s *Store) DisposeFromTheOtherDoor(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return writePair(ctx, tx, id)
}

func writePair(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx, `+"`UPDATE person SET display_name = $1 WHERE id = $2`"+`, "x", id)
	return err
}
`))
	if !written["DisposeFromTheOtherDoor"] || guarded["DisposeFromTheOtherDoor"] {
		t.Error("the second door was not reported: it reaches the same write with no probe on its " +
			"own path, and one guarded caller must not vouch for the other")
	}
	if !written["writePair"] {
		t.Error("the shared writer was suppressed although one of its callers takes no probe — that " +
			"is the shape where the wrapper answers for the write and the other path walks past it")
	}
}

// A DELETE is a mutation, and a census that only reads UPDATE would report the
// destructive half of the tier as clean.
func TestADeleteWithNoRowProbeIsAFinding(t *testing.T) {
	t.Parallel()
	written, guarded := judgePlantedWrite(t, plantedSource(`
func (s *Store) DropPair(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx, `+"`DELETE FROM person WHERE id = $1`"+`, id)
	return err
}
`))
	if !written["DropPair"] || guarded["DropPair"] {
		t.Errorf("an unprobed DELETE of `person` was judged written=%v guarded=%v, want seen and "+
			"unguarded", written["DropPair"], guarded["DropPair"])
	}
}

// A table outside the shareable set is outside the question: its visibility IS
// its write authority, so demanding a probe there would fail the tier for rows
// where the two cannot differ.
func TestAWriteToATableNoGrantCanNameIsNotJudged(t *testing.T) {
	t.Parallel()
	tables := shareableTables(t)
	if tables["audit_log"] {
		t.Fatal("audit_log is now a shareable table, so this case no longer plants what it describes " +
			"— pick a table a record grant still cannot name")
	}
	written, _ := judgePlantedWrite(t, plantedSource(`
func (s *Store) Stamp(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx, `+"`UPDATE audit_log SET evidence = $1 WHERE id = $2`"+`, "x", id)
	return err
}
`))
	if written["Stamp"] {
		t.Error("a write to a table no manual grant can name was pulled into the census, which would " +
			"make every list and saved-view writer in the tier a finding")
	}
}

// The vocabulary is derived, so a probe spelling that is NOT a write question
// must not satisfy the obligation — which is the whole difference between this
// pair of gates and the visibility one they were split out of.
func TestAVisibilityProbeDoesNotAnswerTheWriteQuestion(t *testing.T) {
	t.Parallel()
	vocab := writeAuthorityVocabulary(t)
	if vocab["EnsureVisible"] {
		t.Fatal("EnsureVisible now reaches the write-authority core in platform/auth, so it is no " +
			"longer a visibility-only spelling and this case plants nothing")
	}
	_, guarded := judgePlantedWrite(t, plantedSource(`
func (s *Store) Dispose(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if err := auth.EnsureVisible(ctx, tx, "person", id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `+"`UPDATE person SET display_name = $1 WHERE id = $2`"+`, "x", id)
	return err
}
`))
	if guarded["Dispose"] {
		t.Error("a visibility probe was accepted as write authority — that is the original defect, " +
			"where a `read` share was a licence to write while the sharing screen said it was not")
	}
}

// The floor under the planted cases themselves: every one of them names a table
// the shareable vocabulary still holds. A table renamed out from under this file
// would leave each case planting nothing and passing.
func TestThePlantedCasesNameATableThisGateStillJudges(t *testing.T) {
	t.Parallel()
	tables := shareableTables(t)
	if !tables["person"] {
		t.Fatalf("`person` is no longer a shareable table (the set is %v), so every case in this file "+
			"plants a write the census correctly ignores and passes for the wrong reason",
			strings.Join(sortedKeys(tables), ", "))
	}
}
