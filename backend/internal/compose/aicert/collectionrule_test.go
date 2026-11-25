// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"errors"
	"flag"
	"os"
	"strings"
	"testing"
)

var updateCollectionRule = flag.Bool("update-collection-rule", false,
	"rewrite testdata/collection_rule.golden after collectionRule was bumped for a collection-code change")

const collectionRuleGolden = "testdata/collection_rule.golden"

// collectionRuleOwners are the declarations that decide which runs a record
// holds and how a judge reply becomes a score. They are also grading-rule
// owners, so an edit here moves both digests. Only this one says the stored
// rows differ from what the build would record.
var collectionRuleOwners = []ruleOwner{
	{"run.go", []string{"runEntry"}},
	{"taskdriver.go", []string{"caseSeeds"}},
	{"judge.go", []string{"judgeScore", "wantsAnotherOpinion", "nearABand", "absDiff", "foldOpinions", "judgeVerdict"}},
	{"../certjudge.go", []string{"ParseJudgeVerdict"}},
	{"thresholds.go", []string{"reaskBandMargin", "reaskDisagreement", "maxJudgeOpinions"}},
}

// A restamp grades stored rows again, so it may not run across a change to the
// code that collected them. This holds the version to the code the way the
// grading rule's golden does.
func TestTheCollectionRuleVersionMovesWithItsCode(t *testing.T) {
	got, _ := closureOf(t, collectionRuleOwners)
	raw, err := os.ReadFile(collectionRuleGolden)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reading %s: %v", collectionRuleGolden, err)
	}
	goldenRule, goldenDigest, _ := strings.Cut(strings.TrimSpace(string(raw)), " ")
	moved := goldenRule == collectionRule && goldenDigest != got
	if *updateCollectionRule {
		if moved {
			t.Fatalf("the collection code changed but collectionRule is still %q; bump it in promptversion.go first", collectionRule)
		}
		if err := os.WriteFile(collectionRuleGolden, []byte(collectionRule+" "+got+"\n"), 0o600); err != nil {
			t.Fatalf("writing %s: %v", collectionRuleGolden, err)
		}
		return
	}
	switch {
	case goldenRule == "":
		t.Fatalf("%s is missing; run this test with -update-collection-rule", collectionRuleGolden)
	case moved:
		t.Fatalf("the collection code changed (%s) without a new version: bump collectionRule in promptversion.go, "+
			"then run this test with -update-collection-rule", got)
	case goldenRule != collectionRule:
		t.Fatalf("collectionRule is %q but %s records %q; run this test with -update-collection-rule",
			collectionRule, collectionRuleGolden, goldenRule)
	}
}

// The table is what a restamp reads, so it must say what is in force today.
func TestTheCurrentGradingRuleNamesTheCurrentCollectionRule(t *testing.T) {
	if got := collectionOfRule[gradingRule]; got != collectionRule {
		t.Errorf("collectionOfRule[%q] = %q, want %q: add the entry when gradingRule or collectionRule moves", gradingRule, got, collectionRule)
	}
}
