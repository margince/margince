// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var updateGradingRule = flag.Bool("update-grading-rule", false,
	"rewrite testdata/grading_rule.golden after gradingRule was bumped for a grading-code change")

const gradingRuleGolden = "testdata/grading_rule.golden"

// gradingRuleOwners are the declarations that decide which runs enter the tally
// and turn a judge's replies into a verdict, so an edit to any of them changes
// what a committed record would score today. defaultRepeats is not one: a record
// stores its own run counts and the verdict is recomputed from them. The
// adaptive rounds are: they decide which runs a record holds at all.
var gradingRuleOwners = []struct {
	file  string
	names []string
}{
	{"score.go", []string{
		"Verdict", "caseOf", "passVetoed", "runLeaked", "leaked", "judgeUpper", "underFloor", "rowCase", "verdictOver", "mechanicalBand",
		"judgeBand", "marginsOver", "lowerVerdict", "verdictRank", "majorityOf", "judgeMedianAndMin", "medianOf",
		"caseVerdict", "caseMechanicalBand", "caseJudgeBand",
	}},
	{"stats.go", []string{"wilsonBounds", "meanBounds", "meanAndSD", "tQuantile"}},
	{"adaptive.go", []string{"runRounds", "roundsOf", "firstRunCounts", "nextRunCounts", "caseBorderline", "poolUndecided", "extendsCase"}},
	{"run.go", []string{"runEntry"}},
	{"taskdriver.go", []string{"caseSeeds"}},
	{"judge.go", []string{"judgeScore", "wantsAnotherOpinion", "nearABand", "absDiff", "foldOpinions", "judgeVerdict"}},
	{"../certjudge.go", []string{"ParseJudgeVerdict"}},
	{"thresholds.go", []string{
		"certifiedPassPercent", "certifiedPassBoundPercent", "casePassPercent", "vetoPassPercent",
		"majorityNumerator", "majorityDenominator", "confidenceZ", "tQuantile90", "adaptiveRound", "adaptiveMaxRuns",
		"reaskBandMargin", "reaskDisagreement", "maxJudgeOpinions", "judgeScoreSDFloor", "privacyMinRuns",
		"certifiedMinRuns", "minJudgeScores",
	}},
}

// gradingRuleDigest hashes each owner's gofmt'd source with comments and blank
// lines dropped, so rewording a doc comment does not demand a new rule version.
// Beyond the named owners it hashes what they call, however deep. That means a
// function called by name, a method called through a selector, and each
// constant they name. Variables are not walked. The site checks live in one,
// and they are stamped beside the rule.
func gradingRuleDigest(t *testing.T) string {
	t.Helper()
	digest, _ := gradingRuleClosure(t)
	return digest
}

// gradingRuleClosure is the digest and the sorted names it covers.
func gradingRuleClosure(t *testing.T) (digest string, names []string) {
	t.Helper()
	fset := token.NewFileSet()
	index := indexPackage(t, fset)
	var material strings.Builder
	hashed := map[string]bool{}
	var pending []*ast.FuncDecl
	write := func(label string, node ast.Node) {
		var src bytes.Buffer
		if err := format.Node(&src, fset, node); err != nil {
			t.Fatalf("formatting %s: %v", label, err)
		}
		fmt.Fprintf(&material, "%s\n%s\n", label, dropBlankLines(src.String()))
	}
	for _, owner := range gradingRuleOwners {
		file, err := parser.ParseFile(fset, owner.file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", owner.file, err)
		}
		for _, name := range owner.names {
			node := findTopLevelDecl(file, name)
			if node == nil {
				t.Fatalf("%s no longer declares %s; move it in gradingRuleOwners to where it lives now, "+
					"or bump gradingRule if the grading rule lost it", owner.file, name)
			}
			write(owner.file+"."+name, node)
			hashed[name] = true
			if fn, ok := node.(*ast.FuncDecl); ok && !strings.HasPrefix(owner.file, "..") {
				pending = append(pending, fn)
			}
		}
	}
	take := func(label, name, key string, decls []*ast.FuncDecl) {
		hashed[key] = true
		for i, decl := range decls {
			write(fmt.Sprintf("%s.%s.%d", label, name, i), decl)
			pending = append(pending, decl)
		}
	}
	for len(pending) > 0 {
		fn := pending[0]
		pending = pending[1:]
		ast.Inspect(fn, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				switch callee := node.Fun.(type) {
				case *ast.Ident:
					if !hashed[callee.Name] && index.funcs[callee.Name] != nil {
						take("callee", callee.Name, callee.Name, index.funcs[callee.Name])
					}
				case *ast.SelectorExpr:
					if !hashed["."+callee.Sel.Name] && index.methods[callee.Sel.Name] != nil {
						take("method", callee.Sel.Name, "."+callee.Sel.Name, index.methods[callee.Sel.Name])
					}
				}
			case *ast.Ident:
				if spec := index.consts[node.Name]; spec != nil && !hashed[node.Name] {
					hashed[node.Name] = true
					write("const."+node.Name, spec)
				}
			}
			return true
		})
	}
	for name := range hashed {
		names = append(names, name)
	}
	slices.Sort(names)
	sum := sha256.Sum256([]byte(material.String()))
	return hex.EncodeToString(sum[:]), names
}

// packageIndex is this package's non-test declarations by name, for the walk
// from the owners to what they use.
type packageIndex struct {
	funcs, methods map[string][]*ast.FuncDecl
	consts         map[string]ast.Node
}

func indexPackage(t *testing.T, fset *token.FileSet) packageIndex {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing the package: %v", err)
	}
	index := packageIndex{funcs: map[string][]*ast.FuncDecl{}, methods: map[string][]*ast.FuncDecl{}, consts: map[string]ast.Node{}}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					index.funcs[d.Name.Name] = append(index.funcs[d.Name.Name], d)
				} else {
					index.methods[d.Name.Name] = append(index.methods[d.Name.Name], d)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					if value, ok := spec.(*ast.ValueSpec); ok && d.Tok == token.CONST {
						for _, ident := range value.Names {
							index.consts[ident.Name] = value
						}
					}
				}
			}
		}
	}
	return index
}

// The walk follows calls, not mentions. A built-in that shares a method's name
// then cannot drag the runner, the journal or the report into the grading rule.
// A change to any of those would otherwise ask for a bump, and a bump stales
// every record.
func TestTheGradingRuleDigestCoversWhatGradingCallsAndNothingMore(t *testing.T) {
	_, names := gradingRuleClosure(t)
	for _, want := range []string{"shortOnlyOfScores", "asFloats", "VerdictCertified"} {
		if !slices.Contains(names, want) {
			t.Errorf("the digest does not cover %s, which grading uses", want)
		}
	}
	for _, unrelated := range []string{"Run", "LoadRecords", "WriteRecord", "Readiness", "scenarioStampsUnder", "main"} {
		if slices.Contains(names, unrelated) {
			t.Errorf("the digest covers %s, so editing it would stale every record", unrelated)
		}
	}
	if len(names) > 150 {
		t.Errorf("the digest covers %d declarations; the walk has grown past the grading code", len(names))
	}
	t.Logf("the digest covers %d declarations", len(names))
}

// findTopLevelDecl answers the function or the single const/var/type spec named
// name, or nil.
func findTopLevelDecl(file *ast.File, name string) ast.Node {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.Name == name {
				return d
			}
		case *ast.GenDecl:
			if spec := findSpec(d, name); spec != nil {
				return spec
			}
		}
	}
	return nil
}

func findSpec(decl *ast.GenDecl, name string) ast.Node {
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.ValueSpec:
			for _, ident := range s.Names {
				if ident.Name == name {
					return s
				}
			}
		case *ast.TypeSpec:
			if s.Name.Name == name {
				return s
			}
		}
	}
	return nil
}

// dropBlankLines removes the blank line a stripped comment leaves behind, which
// the printer keeps because it lays out by the original line positions.
func dropBlankLines(src string) string {
	var kept []string
	for line := range strings.SplitSeq(src, "\n") {
		if strings.TrimSpace(line) != "" {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func readGradingRuleGolden(t *testing.T) (rule, digest string) {
	t.Helper()
	raw, err := os.ReadFile(gradingRuleGolden)
	if errors.Is(err, os.ErrNotExist) {
		return "", ""
	}
	if err != nil {
		t.Fatalf("reading %s: %v", gradingRuleGolden, err)
	}
	rule, digest, ok := strings.Cut(strings.TrimSpace(string(raw)), " ")
	if !ok {
		t.Fatalf("%s is not \"<gradingRule> <sha256>\": %q", gradingRuleGolden, raw)
	}
	return rule, digest
}

// ruleExtension lets the grading code move from one digest to the next under
// the same gradingRule. It is only for a change that extends the rule to runs
// no committed record can contain, so no record scores differently under it —
// a change that could re-score one bumps gradingRule instead. The reason says
// why no record can: this list is the gate's one exception, and it is read.
type ruleExtension struct {
	rule, from, to, reason string
}

var gradingRuleExtensions = []ruleExtension{}

// extendedDigest is the digest the golden's version accepts once every
// extension recorded for that version is applied in order.
func extendedDigest(t *testing.T, goldenDigest string) string {
	t.Helper()
	digest := goldenDigest
	for _, ext := range gradingRuleExtensions {
		if ext.rule != gradingRule {
			t.Fatalf("an extension for %q outlived its rule; gradingRule is now %q, so delete it", ext.rule, gradingRule)
		}
		if strings.TrimSpace(ext.reason) == "" {
			t.Fatalf("the extension %s→%s gives no reason; say why no committed record can score differently", ext.from, ext.to)
		}
		if ext.from == digest {
			digest = ext.to
		}
	}
	return digest
}

// A record is stamped with gradingRule, so the code that grades it may not change
// under the same version: that would leave every record graded the old way current.
func TestTheGradingRuleVersionMovesWithItsCode(t *testing.T) {
	got := gradingRuleDigest(t)
	goldenRule, goldenDigest := readGradingRuleGolden(t)
	codeMovedUnderSameRule := goldenRule == gradingRule && extendedDigest(t, goldenDigest) != got
	if *updateGradingRule {
		if codeMovedUnderSameRule {
			t.Fatalf("the grading code changed but gradingRule is still %q; bump it in promptversion.go first", gradingRule)
		}
		if err := os.WriteFile(gradingRuleGolden, []byte(gradingRule+" "+got+"\n"), 0o600); err != nil {
			t.Fatalf("writing %s: %v", gradingRuleGolden, err)
		}
		return
	}
	switch {
	case goldenRule == "":
		t.Fatalf("%s is missing; run this test with -update-grading-rule", gradingRuleGolden)
	case codeMovedUnderSameRule:
		t.Fatalf("the grading rule's code changed (%s) without a new version: bump gradingRule in promptversion.go, "+
			"then run this test with -update-grading-rule", got)
	case goldenRule != gradingRule:
		t.Fatalf("gradingRule is %q but %s records %q; run this test with -update-grading-rule",
			gradingRule, gradingRuleGolden, goldenRule)
	}
}
