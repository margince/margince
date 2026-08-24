// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind census H3

package backendarch

// The gate inventory: every gate in this package declares its own shape, and
// the reference page listing them is rendered from those declarations.
//
// docs/reference/gate-patterns.md teaches the eight shapes. A page that ALSO
// listed which gate is which would be a hand-maintained catalog of a set the
// tree already knows — the list that goes quietly short, which is the defect
// this repository's own principle names. So the list is generated and the
// prose is not.
//
// The declaration lives in the gate's own file, one line under the SPDX
// header, because that is where the person editing the gate will see it. A
// classification kept here instead would be a second file to remember.

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// gateKinds is the closed set of shapes, in the order the page renders them.
// A kind is added here and in gate-patterns.md together: a shape with no
// section to read is a label rather than a classification.
var gateKinds = []string{
	"parity", "census", "reachability", "shape",
	"prohibition", "claim", "budget", "falsification",
}

// gateHardness is the closed set of soundness levels, weakest last so the
// rendered page reads down from what cannot miss to what can.
var gateHardness = []string{"H3", "H2", "H1"}

// gateTag matches the declaration line. Anchored to the line start so a tag
// quoted inside a doc comment's prose — as gate-patterns.md's own examples
// are — cannot register as a second declaration.
var gateTag = regexp.MustCompile(`(?m)^//gate:kind ([a-z]+) (H[123])\s*$`)

const (
	gateInventoryPage = "../docs/reference/gate-inventory.md"
	// gateFloor sits below the real count so it catches a broken walk rather
	// than a shrinking tree. A census that discovered nothing would otherwise
	// render an empty page and call it current.
	gateFloor = 100
)

var updateGateInventory = flag.Bool("update-gate-inventory", false,
	"rewrite docs/reference/gate-inventory.md from the //gate:kind declarations")

// gate is one declared gate: its file, its shape, and the first sentence of
// its own doc comment.
type gate struct {
	file, kind, hardness, holds string
}

func TestEveryGateDeclaresItsShape(t *testing.T) {
	gates, untagged := readGateDeclarations(t)
	if len(untagged) > 0 {
		t.Errorf("%d gate file(s) carry no //gate:kind declaration:\n\t%s\n\n"+
			"Add one line under the SPDX header, e.g. `//gate:kind census H3`. The kinds are %s "+
			"and the hardness levels are %s; docs/reference/gate-patterns.md explains each. "+
			"Without it the gate is missing from docs/reference/gate-inventory.md, which is the "+
			"list the next author reads before writing a ninth shape.",
			len(untagged), strings.Join(untagged, "\n\t"),
			strings.Join(gateKinds, ", "), strings.Join(gateHardness, ", "))
	}
	if len(gates) < gateFloor {
		t.Fatalf("this census found %d gate file(s) and expects at least %d, so the walk over "+
			"backend/*_test.go has stopped finding them rather than the tree having shrunk",
			len(gates), gateFloor)
	}
}

func TestTheGateInventoryPageIsCurrent(t *testing.T) {
	gates, _ := readGateDeclarations(t)
	if len(gates) < gateFloor {
		t.Fatalf("refusing to judge the page against %d gate(s); see TestEveryGateDeclaresItsShape",
			len(gates))
	}
	want := renderGateInventory(gates)
	if *updateGateInventory {
		if err := os.WriteFile(gateInventoryPage, []byte(want), 0o644); err != nil {
			t.Fatalf("rewriting %s: %v", gateInventoryPage, err)
		}
		return
	}
	got, err := os.ReadFile(gateInventoryPage)
	if err != nil {
		t.Fatalf("reading %s: %v\nRegenerate it from the backend directory with:\n"+
			"  go test . -run GateInventory -update-gate-inventory", gateInventoryPage, err)
	}
	if string(got) == want {
		return
	}
	t.Errorf("%s no longer matches the //gate:kind declarations in the tree.\n"+
		"Regenerate it from the backend directory with:\n"+
		"  go test . -run GateInventory -update-gate-inventory\n"+
		"and commit the page together with the change that moved it.", gateInventoryPage)
}

// readGateDeclarations walks this package's test files and returns the declared
// gates plus the names of any that declare nothing.
//
// A gate file is one that declares a Test function. The helper files beside them
// — the shared call graph, the shared SQL reader — hold no test and are not
// gates, so the population is derived from the tree rather than from a list.
func readGateDeclarations(t *testing.T) (gates []gate, untagged []string) {
	t.Helper()
	paths, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatalf("listing this package's test files: %v", err)
	}
	for _, path := range paths {
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		parsed := parseGateFile(t, path, source)
		if !declaresATest(parsed) {
			continue
		}
		match := gateTag.FindSubmatch(source)
		if match == nil {
			untagged = append(untagged, path)
			continue
		}
		kind, hardness := string(match[1]), string(match[2])
		requireDeclaredValue(t, path, "kind", kind, gateKinds)
		requireDeclaredValue(t, path, "hardness", hardness, gateHardness)
		gates = append(gates, gate{path, kind, hardness, firstSentenceOf(parsed)})
	}
	sort.Slice(gates, func(i, j int) bool { return gates[i].file < gates[j].file })
	return gates, untagged
}

// parseGateFile reads one test file with its comments kept, which is what makes
// the prose below an import block reachable.
func parseGateFile(t *testing.T, path string, source []byte) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return file
}

// declaresATest reports whether a file holds a Test function, read from its
// syntax rather than by matching `func Test` in text — a commented-out example
// of one is not a gate.
func declaresATest(file *ast.File) bool {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil &&
			strings.HasPrefix(fn.Name.Name, "Test") {
			return true
		}
	}
	return false
}

func requireDeclaredValue(t *testing.T, path, field, value string, allowed []string) {
	t.Helper()
	for _, ok := range allowed {
		if value == ok {
			return
		}
	}
	t.Errorf("%s declares %s %q, which is not one of %s. Adding a value here means adding its "+
		"section to docs/reference/gate-patterns.md too — a shape with nothing to read is a label.",
		path, field, value, strings.Join(allowed, ", "))
}

// firstSentenceOf returns the opening sentence of the file's own prose, which
// is what the page prints for each gate.
//
// Derived rather than declared: a gate whose doc comment already says what it
// holds would otherwise say it twice, and the two would drift.
//
// Read from the parsed comment groups rather than by scanning lines from the
// top. Three gates in this tree put their prose BELOW the import block, and a
// line scan that stopped at the first non-comment line reported them as having
// no doc comment at all — an under-recognition that renders as a plausible
// empty cell rather than as a failure.
func firstSentenceOf(file *ast.File) string {
	for _, group := range file.Comments {
		text := strings.TrimSpace(group.Text())
		if text == "" || isMachineReadable(group) {
			continue
		}
		return firstSentence(text)
	}
	return ""
}

// isMachineReadable reports whether a comment group is one of the directive
// blocks at the top of a file rather than prose a reader is meant to read.
func isMachineReadable(group *ast.CommentGroup) bool {
	for _, line := range group.List {
		if !strings.HasPrefix(line.Text, "// SPDX") &&
			!strings.HasPrefix(line.Text, "//gate:") &&
			!strings.HasPrefix(line.Text, "//go:") {
			return false
		}
	}
	return true
}

// sentenceEnd is a full stop that ends a sentence rather than one inside a
// file name or an abbreviation: `crm.yaml` and `§4.2` keep their dots because
// what follows is not a space.
var sentenceEnd = regexp.MustCompile(`\.\s`)

func firstSentence(prose string) string {
	prose = strings.Join(strings.Fields(prose), " ")
	if at := sentenceEnd.FindStringIndex(prose); at != nil {
		return prose[:at[0]+1]
	}
	return prose
}

func renderGateInventory(gates []gate) string {
	var page strings.Builder
	page.WriteString(gateInventoryPreamble)
	byKind := map[string][]gate{}
	for _, g := range gates {
		byKind[g.kind] = append(byKind[g.kind], g)
	}
	for _, kind := range gateKinds {
		fmt.Fprintf(&page, "\n## %s (%d)\n\n", capitalized(kind), len(byKind[kind]))
		page.WriteString("| Gate | Hardness | What it holds |\n|---|---|---|\n")
		for _, g := range byKind[kind] {
			fmt.Fprintf(&page, "| `%s` | %s | %s |\n", g.file, g.hardness, cellText(g.holds))
		}
	}
	return page.String()
}

// capitalized renders a kind as a heading. The kinds are single ASCII words,
// which is why this is two lines rather than a call into text/cases.
func capitalized(kind string) string {
	return strings.ToUpper(kind[:1]) + kind[1:]
}

// cellText makes a sentence safe inside a markdown table cell: a pipe would
// end the column early, and a newline the row.
func cellText(prose string) string {
	prose = strings.ReplaceAll(prose, "|", `\|`)
	if prose == "" {
		return "_(no doc comment)_"
	}
	return prose
}

const gateInventoryPreamble = `# Gate inventory

<!-- Generated by backend/gateinventory_test.go. Do not edit by hand. -->

**This page is generated, and an edit made here is lost.** It is rendered from
the ` + "`//gate:kind`" + ` line each gate declares in its own file, so it states what the
tree holds rather than what somebody wrote down once.

Each row's sentence is the opening line of that gate's own doc comment — the
file itself is the authority on what it holds. To move a gate between shapes,
edit its ` + "`//gate:kind`" + ` line, then regenerate from the backend directory:

    go test . -run GateInventory -update-gate-inventory

The eight shapes, what each is for, and how each one silently passes:
[gate-patterns.md](gate-patterns.md).
`
