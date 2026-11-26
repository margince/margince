// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Margince asks "Which deal is this about?" in one place: the link suggestion
// queue, through activities.AskWhichDealTx, where only the colleague asked sees
// it. Any producer of the question spells one of its marks: the question's
// words (the systemtext key) or the source stored on old question tasks. The
// census finds every function that does or that calls AskWhichDealTx. The
// one allowed producer must ask through the queue, and any other fails here.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// dealQuestionProducer is the one function allowed to raise the question.
const dealQuestionProducer = "compose/capturewhichdeal.go:Ask"

// dealQuestionMarks are the spellings only a producer of the question uses.
var dealQuestionMarks = []string{"capture.which_deal_subject", provenance.WhichDealSource}

// scanDealQuestionProducers answers each function under root that spells a
// mark or calls AskWhichDealTx, and whether it calls it. planted adds files a
// test writes.
func scanDealQuestionProducers(t *testing.T, root string, planted map[string]string) map[string]bool {
	t.Helper()
	found := map[string]bool{}
	fset := token.NewFileSet()
	add := func(rel string, src any) {
		file, err := parser.ParseFile(fset, rel, src, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", rel, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			asks := firstCall(fn, callsNamed("AskWhichDealTx")) >= 0
			if asks || spellsDealQuestion(fn.Body) {
				found[filepath.ToSlash(rel)+":"+fn.Name.Name] = asks
			}
		}
	}
	err := gatekit.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		body, err := gatekit.ReadFile(path) // #nosec G304 -- a *.go file from walking the trusted backend tree
		if err != nil {
			return err
		}
		add(rel, body)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	for rel, src := range planted {
		add(rel, src)
	}
	return found
}

// spellsDealQuestion reports whether body names a mark, as a string literal or
// as provenance.WhichDealSource.
func spellsDealQuestion(body *ast.BlockStmt) bool {
	spells := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BasicLit:
			if value, err := strconv.Unquote(node.Value); node.Kind == token.STRING && err == nil && slices.Contains(dealQuestionMarks, value) {
				spells = true
			}
		case *ast.SelectorExpr:
			spells = spells || node.Sel.Name == "WhichDealSource"
		}
		return !spells
	})
	return spells
}

func dealQuestionFaults(found map[string]bool) []string {
	var faults []string
	for key, viaQueue := range found {
		switch {
		case key != dealQuestionProducer:
			faults = append(faults, key+" raises the deal question outside the link suggestion queue")
		case !viaQueue:
			faults = append(faults, key+" raises the deal question without activities.AskWhichDealTx")
		}
	}
	if _, ok := found[dealQuestionProducer]; !ok {
		faults = append(faults, dealQuestionProducer+" no longer raises the deal question; name its new producer here")
	}
	slices.Sort(faults)
	return faults
}

func TestTheDealQuestionHasOneProducerAndItIsTheQueue(t *testing.T) {
	t.Parallel()
	found := scanDealQuestionProducers(t, filepath.Join(gatekit.ModuleRoot(t), "internal"), nil)
	if faults := dealQuestionFaults(found); len(faults) > 0 {
		t.Fatalf("the deal question census found:\n%s\n\nAsk it through activities.AskWhichDealTx from %s, "+
			"so it reaches only the colleague asked and one answer settles it.", strings.Join(faults, "\n"), dealQuestionProducer)
	}
}

// A task writer that raises the question, a second caller of the queue's
// door, and a producer that stops asking through the queue all fail.
func TestTheDealQuestionCensusCatchesASecondProducer(t *testing.T) {
	t.Parallel()
	planted := map[string]string{
		"modules/planted/question.go": `package planted
func raise() { write(provenance.WhichDealSource) }`,
		"modules/planted/copy.go": `package planted
func words() string { return systemtext.Copy("capture.which_deal_subject", "en") }`,
		"compose/refileasker.go": `package compose
func ask(tx pgx.Tx) error { return activities.AskWhichDealTx(ctx, tx, q) }`,
		"compose/capturewhichdeal.go": `package compose
func (a whichDealAsker) Ask() error { return tasks.LogActivityTx("which_deal") }`,
	}
	faults := dealQuestionFaults(scanDealQuestionProducers(t, t.TempDir(), planted))
	want := []string{
		dealQuestionProducer + " raises the deal question without activities.AskWhichDealTx",
		"compose/refileasker.go:ask raises the deal question outside the link suggestion queue",
		"modules/planted/copy.go:words raises the deal question outside the link suggestion queue",
		"modules/planted/question.go:raise raises the deal question outside the link suggestion queue",
	}
	if !slices.Equal(faults, want) {
		t.Fatalf("the census found %q, want %q", faults, want)
	}
}
