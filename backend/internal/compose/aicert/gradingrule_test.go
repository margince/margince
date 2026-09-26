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
		"Verdict", "caseOf", "passVetoed", "judgeUpper", "underFloor", "rowCase", "verdictOver", "mechanicalBand",
		"judgeBand", "marginsOver", "lowerVerdict", "verdictRank", "majorityOf", "judgeMedianAndMin", "medianOf",
	}},
	{"stats.go", []string{"wilsonBounds", "meanBounds", "meanAndSD", "tQuantile"}},
	{"adaptive.go", []string{"runRounds", "nextRunCounts", "caseBorderline", "poolUndecided"}},
	{"run.go", []string{"runEntry"}},
	{"taskdriver.go", []string{"caseSeeds"}},
	{"judge.go", []string{"judgeScore", "wantsAnotherOpinion", "nearABand", "absDiff", "foldOpinions", "judgeVerdict"}},
	{"../certjudge.go", []string{"ParseJudgeVerdict"}},
	{"thresholds.go", []string{
		"certifiedPassPercent", "certifiedPassBoundPercent", "casePassPercent", "vetoPassPercent",
		"majorityNumerator", "majorityDenominator", "confidenceZ", "tQuantile90", "adaptiveRound", "adaptiveMaxRuns",
		"reaskBandMargin", "reaskDisagreement", "maxJudgeOpinions", "judgeScoreSDFloor",
	}},
}

// gradingRuleDigest hashes each owner's gofmt'd source with comments and blank
// lines dropped, so rewording a doc comment does not demand a new rule version.
func gradingRuleDigest(t *testing.T) string {
	t.Helper()
	var material strings.Builder
	for _, owner := range gradingRuleOwners {
		fset := token.NewFileSet()
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
			var src bytes.Buffer
			if err := format.Node(&src, fset, node); err != nil {
				t.Fatalf("formatting %s.%s: %v", owner.file, name, err)
			}
			fmt.Fprintf(&material, "%s.%s\n%s\n", owner.file, name, dropBlankLines(src.String()))
		}
	}
	sum := sha256.Sum256([]byte(material.String()))
	return hex.EncodeToString(sum[:])
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
	for _, line := range strings.Split(src, "\n") {
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

// A record is stamped with gradingRule, so the code that grades it may not change
// under the same version: that would leave every record graded the old way current.
func TestTheGradingRuleVersionMovesWithItsCode(t *testing.T) {
	got := gradingRuleDigest(t)
	goldenRule, goldenDigest := readGradingRuleGolden(t)
	codeMovedUnderSameRule := goldenRule == gradingRule && goldenDigest != got
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
