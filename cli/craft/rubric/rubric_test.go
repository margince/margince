package rubric

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestLoad_everyRuleCarriesIDCategoryAndBlockFlag(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if r.Version == "" {
		t.Error("rubric version is empty; the version pins the gate identity tuple")
	}
	if len(r.Rules) == 0 {
		t.Fatal("rubric has no rules")
	}
	seen := map[string]bool{}
	for _, rule := range r.Rules {
		if rule.ID == "" {
			t.Errorf("rule with title %q has no id", rule.Title)
		}
		if rule.Category == "" {
			t.Errorf("rule %s has no category", rule.ID)
		}
		if rule.Kind != KindAntiTell && rule.Kind != KindPositive {
			t.Errorf("rule %s has unknown kind %q", rule.ID, rule.Kind)
		}
		// block_eligible is the field downstream BLOCK-mapping reads; positive
		// guidelines must never be block-eligible.
		if rule.Kind == KindPositive && rule.BlockEligible {
			t.Errorf("positive rule %s is block_eligible; only anti-tells block", rule.ID)
		}
		if seen[rule.ID] {
			t.Errorf("duplicate rule id %s", rule.ID)
		}
		seen[rule.ID] = true
	}
}

func TestBlockEligible(t *testing.T) {
	r, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tests := []struct {
		category string
		want     bool
	}{
		{"over-commenting", true},
		{"type-escape-hatch", true},
		{"idiomatic", false},
		{"restraint", false},
		{"category-that-does-not-exist", false},
	}
	for _, tt := range tests {
		if got := r.BlockEligible(tt.category); got != tt.want {
			t.Errorf("BlockEligible(%q) = %v, want %v", tt.category, got, tt.want)
		}
	}
}

// The prose form of the standard lives in AGENTS.md's Craftsmanship section and
// the machine form lives in rubric.json. Two statements of one thing drift, and
// these two had: the prose advertised "T1-P3" while the rubric carried T1-T10
// plus P1-P5, so it promised a rule that does not exist and said nothing about
// five that block a push.
//
// The expectation is derived from the rubric, which is the standard — a rule id
// added there is enrolled here the moment it exists, and there is no list in this
// test to go short.
func TestTheProseFormNamesTheSameRules(t *testing.T) {
	const rulebook = "../../../AGENTS.md"

	raw, err := os.ReadFile(rulebook)
	if err != nil {
		t.Fatalf("read %s: %v", rulebook, err)
	}
	section, found := CraftsmanshipSection(string(raw))
	if !found {
		t.Fatalf("%s carries no ## Craftsmanship section — the gate is handed that section as its delta layer, "+
			"and `make check-craft-doc` asserts it exists", rulebook)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("load rubric: %v", err)
	}

	// Any T-or-P id the prose mentions must be a real rule.
	idPattern := regexp.MustCompile(`\b([TP]\d{1,2})\b`)
	real := map[string]bool{}
	for _, r := range loaded.Rules {
		real[r.ID] = true
	}
	for _, m := range idPattern.FindAllStringSubmatch(section, -1) {
		if !real[m[1]] {
			t.Errorf("%s's Craftsmanship section names rule %s, which rubric.json does not carry. An agent reading "+
				"the rulebook is told about a rule the gate will never apply; worse, a range like T1-P3 written "+
				"over a rubric of T1-T10 hides that the ids past it are P-rules.", rulebook, m[1])
		}
	}

	// And the prose must not silently drop a whole class the gate blocks on.
	var hasPositive bool
	for _, r := range loaded.Rules {
		if r.Kind == KindPositive {
			hasPositive = true
			break
		}
	}
	if hasPositive && !strings.Contains(section, "P1") {
		t.Errorf("rubric.json carries positive rules (P-ids) and %s's Craftsmanship section never mentions one. "+
			"They are part of the standard the gate applies, so a reader of the rulebook alone does not know "+
			"what blocks a push.", rulebook)
	}
}
