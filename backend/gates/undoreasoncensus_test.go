// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// One refusal set, three spellings.
//
// A person reads a reason twice: once as the sentence under a disabled Undo
// button, and once as the 409 that comes back if they press it anyway and the
// binding evaluation disagrees. Those must be the same words. Three artefacts
// hold the set — the Go constants the evaluator returns, the crm.yaml enum the
// contract publishes, and the frontend copy that renders it — and a reason
// present in one and absent from another is a refusal that renders as blank
// space or as a raw identifier at a user.
//
// The corpus is derived from the Go constants, which are where a reason is
// BORN: a branch cannot refuse with a word it has not declared. Restating the
// list here would make this gate a fourth copy of the thing it exists to keep
// singular.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

// contractReasonEnum reads the enum crm.yaml publishes on Undoability.reason.
// `null` is a member there and not a reason: the wire type is nullable because
// reason is absent exactly when the entry IS undoable.
func contractReasonEnum(t *testing.T) []string {
	t.Helper()
	contract, err := os.ReadFile(filepath.Join("api", "crm.yaml"))
	if err != nil {
		t.Fatalf("read the contract: %v", err)
	}
	schema := regexp.MustCompile(`(?s)    Undoability:.*?\n    [A-Z]`).FindString(string(contract))
	if schema == "" {
		t.Fatal("no Undoability schema in crm.yaml; the census reads the enum from there, " +
			"and a schema it cannot find reports an empty set rather than a disagreement")
	}
	block := regexp.MustCompile(`(?s)enum: \[(.*?)\]`).FindStringSubmatch(schema)
	if block == nil {
		t.Fatal("Undoability.reason declares no enum; every reason the evaluator can return " +
			"must be a value the contract admits")
	}
	var members []string
	for _, member := range strings.Split(block[1], ",") {
		member = strings.TrimSpace(strings.Trim(strings.TrimSpace(member), "\n"))
		member = strings.Join(strings.Fields(member), "")
		if member == "" || member == "null" {
			continue
		}
		members = append(members, member)
	}
	sort.Strings(members)
	return members
}

// declaredReasons is compose.Reasons, spelled as strings. It is read from the
// exported slice rather than from the constants' source, because the slice is
// what the evaluator's branch order walks — a constant declared and never put
// in it is a reason no branch can return, and this gate should not bless it.
func declaredReasons() []string {
	out := make([]string, 0, len(compose.Reasons))
	for _, reason := range compose.Reasons {
		out = append(out, string(reason))
	}
	sort.Strings(out)
	return out
}

// The contract admits exactly the reasons the evaluator can return. A Go
// constant with no enum member is a 409 whose code the client cannot type; an
// enum member with no constant is a reason the product promises and no branch
// produces.
// Every reason a branch actually returns is in compose.Reasons. Without this
// the list is a hand-kept claim: a branch refusing with a constant nobody
// listed would publish no enum member, get no frontend copy, and reach a person
// as a raw identifier under a disabled button — and no gate here would notice,
// because all three of the others read the list rather than the branches.
func TestEveryReasonABranchReturnsIsListed(t *testing.T) {
	t.Parallel()
	listed := map[string]bool{}
	for _, reason := range declaredReasons() {
		listed["Reason"+reasonConstantName(reason)] = true
	}
	returned := reasonsReturnedUnderAModeGuard(theEvaluatorsSources(t))
	if len(returned.anywhere) == 0 {
		t.Fatal("no refuse() call found in the evaluator; the walk is broken, not the tree")
	}
	for name := range returned.anywhere {
		if !listed[name] {
			t.Errorf("%s is returned by a branch and is not in compose.Reasons; "+
				"it would publish no enum member and get no copy, and reach a person "+
				"as a raw identifier under a disabled button", name)
		}
	}
}

func TestTheContractAdmitsExactlyTheReasonsTheEvaluatorCanReturn(t *testing.T) {
	t.Parallel()
	declared, published := declaredReasons(), contractReasonEnum(t)
	if len(declared) == 0 {
		t.Fatal("compose.Reasons is empty; the census would report agreement between two " +
			"empty sets, which is the shape of a gate that has already failed")
	}
	if strings.Join(declared, ",") != strings.Join(published, ",") {
		t.Errorf("the refusal set disagrees with the contract.\n"+
			"\tcompose.Reasons: %v\n\tcrm.yaml enum:   %v\n"+
			"\tA reason in one and not the other renders as blank space or as a raw "+
			"identifier at a user.", declared, published)
	}
}

// Every reason the frontend can be handed has copy. A reason with no sentence
// is the greyed button with no explanation this feature exists to remove —
// and it fails silently, because the button still renders.
func TestTheFrontendHasWordsForEveryReason(t *testing.T) {
	t.Parallel()
	copyText := readFrontendReasonCopy(t)
	for _, reason := range declaredReasons() {
		if !strings.Contains(copyText, reason) {
			t.Errorf("no frontend copy names %q; a refusal with no sentence renders as a "+
				"disabled button that will not say why", reason)
		}
	}
}

// readFrontendReasonCopy concatenates the frontend sources that could carry the
// copy. It reads the whole screens tree rather than one named file: a gate that
// hard-codes part of its subject has become a second copy of it, and the file
// this copy lives in is the frontend's decision, not this gate's.
func readFrontendReasonCopy(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "frontend", "src")
	var found strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
			return nil
		}
		// A reason named only in a test satisfies nobody: the gate exists to
		// prove a PERSON is given words for the refusal, and a test file is not
		// a sentence anyone reads.
		if strings.HasSuffix(path, ".test.ts") || strings.HasSuffix(path, ".test.tsx") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		found.Write(body)
		return nil
	})
	if err != nil {
		t.Fatalf("read the frontend sources: %v", err)
	}
	if found.Len() == 0 {
		t.Fatal("no frontend sources read; the gate would report every reason as having copy")
	}
	return found.String()
}

// The advisory read and the binding write may differ on TIMING and never on the
// set of reasons. Three of them depend on state the write path owns and are
// best-effort on a read — but "best-effort" means the read may not yet know,
// not that the read can return a word the write cannot, or the reverse. Both
// modes run the same branches, and this is the assertion that they do.
func TestBothEvaluationModesCanReturnTheSameReasons(t *testing.T) {
	t.Parallel()
	returned := reasonsReturnedUnderAModeGuard(theEvaluatorsSources(t))
	for _, reason := range declaredReasons() {
		name := "Reason" + reasonConstantName(reason)
		if !returned.anywhere[name] {
			t.Errorf("no branch returns %q; a reason the contract publishes and no branch "+
				"produces is a promise the product does not keep", reason)
		}
		if returned.underModeGuard[name] {
			t.Errorf("%q is returned only from inside a branch testing the evaluation mode; "+
				"the two modes may differ on timing, never on the SET of reasons", reason)
		}
	}
}

// reasonsReturned records which reason constants reach a refuse() call, and
// which of them do so ONLY from inside a condition that reads the mode.
//
// The distinction is why this walks the syntax rather than the text: the
// evaluator does test the mode, for one thing — whether a failed live-state
// lookup falls back to undoable — and a proximity heuristic cannot tell that
// error fallback apart from a branch that chooses a reason by mode. A gate
// that cannot tell them apart fires on every refactor and teaches its readers
// to ignore it.
type reasonsReturned struct {
	anywhere       map[string]bool
	underModeGuard map[string]bool
	// Collected separately so the difference can be taken once at the end,
	// rather than clearing a flag mid-walk and depending on which branch the
	// walk happens to reach last.
	unguarded map[string]bool
}

// theEvaluatorsSources parses every hand-written file of package compose.
//
// The corpus is the PACKAGE and not one file, because the evaluator's branches
// are not one file's: the edge arm lives beside the edge reversal it judges, and
// a census reading undoability.go alone reported PASS while a whole branch set
// was invisible to it — under-recognition, which is the one way a gate must not
// break, because a smaller tree still produces a green run and no assertion to
// notice. Any file may hold a refuse() call and every one of them is read.
func theEvaluatorsSources(t *testing.T) []*ast.File {
	t.Helper()
	dir := filepath.Join("internal", "compose")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read the evaluator's package: %v", err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		t.Fatal("no sources parsed for package compose; the corpus is empty, not the tree")
	}
	return files
}

func reasonsReturnedUnderAModeGuard(files []*ast.File) reasonsReturned {
	found := reasonsReturned{
		anywhere:       map[string]bool{},
		underModeGuard: map[string]bool{},
		unguarded:      map[string]bool{},
	}
	var guards int
	var walk func(node ast.Node) bool
	walk = func(node ast.Node) bool {
		branch, isBranch := node.(*ast.IfStmt)
		if isBranch && conditionReadsTheMode(branch.Cond) {
			guards++
			ast.Inspect(branch.Body, walk)
			if branch.Else != nil {
				ast.Inspect(branch.Else, walk)
			}
			guards--
			return false
		}
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		name, isRefusal := call.Fun.(*ast.Ident)
		if !isRefusal || name.Name != "refuse" || len(call.Args) == 0 {
			return true
		}
		reason, named := call.Args[0].(*ast.Ident)
		if !named {
			return true
		}
		if guards > 0 {
			found.underModeGuard[reason.Name] = true
		} else {
			found.unguarded[reason.Name] = true
		}
		return true
	}
	for _, file := range files {
		ast.Inspect(file, walk)
	}
	// Both sets are collected in full and the difference taken here, never
	// cleared as the walk goes: a reason returned unguarded EARLY and again
	// inside a mode-guarded branch later would otherwise keep the flag its
	// second visit set, and the census would report a mode-only reason because
	// of the order the branches happen to appear in.
	for name := range found.unguarded {
		found.anywhere[name] = true
		delete(found.underModeGuard, name)
	}
	for name := range found.underModeGuard {
		found.anywhere[name] = true
	}
	return found
}

// conditionReadsTheMode reports whether an if-condition tests the evaluation
// mode. It matches the identifier rather than a spelling of the comparison, so
// `mode == Advisory`, `mode != Binding` and a helper taking mode all count.
func conditionReadsTheMode(cond ast.Expr) bool {
	reads := false
	ast.Inspect(cond, func(node ast.Node) bool {
		if ident, isIdent := node.(*ast.Ident); isIdent && ident.Name == "mode" {
			reads = true
		}
		return true
	})
	return reads
}

// reasonConstantName turns a wire reason into the Go constant's suffix.
func reasonConstantName(reason string) string {
	var name strings.Builder
	for _, word := range strings.Split(reason, "_") {
		if word == "" {
			continue
		}
		name.WriteString(strings.ToUpper(word[:1]) + word[1:])
	}
	return name.String()
}
