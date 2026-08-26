package rubric

import (
	"os"
	"regexp"
	"strconv"
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

// TestTheSectionScanSurvivesAFencedExample plants the two fence shapes that broke
// it: an inner ```go, which cannot close a block, and an opener behind a list
// marker, which a backtick-anchored scan walks past.
func TestTheSectionScanSurvivesAFencedExample(t *testing.T) {
	const doc = "# Doc\n\n" +
		"Here is what a rulebook looks like:\n\n" +
		"````markdown\n" +
		"## Craftsmanship\n" +
		"THIS IS THE EXAMPLE, NOT THE SECTION\n" +
		"```go\n" +
		"x := 1\n" +
		"```\n" +
		"````\n\n" +
		"- an opener behind a list marker:\n" +
		"- ```text\n" +
		"  ## Craftsmanship also not it\n" +
		"  ```\n\n" +
		"## Craftsmanship\n" +
		"THE REAL SECTION\n\n" +
		"## Next\n"

	section, found := CraftsmanshipSection(doc)
	if !found {
		t.Fatal("no ## Craftsmanship section found at all — the fenced examples swallowed the real one")
	}
	if !strings.Contains(section, "THE REAL SECTION") {
		t.Errorf("selected a fenced example instead of the real section:\n%s", section)
	}
	if strings.Contains(section, "NOT THE SECTION") || strings.Contains(section, "also not it") {
		t.Errorf("the section ran into a fenced example:\n%s", section)
	}
	if strings.Contains(section, "## Next") {
		t.Errorf("the section did not stop at the next H2:\n%s", section)
	}
}

// TestTheProseRangeReachesTheHighestRule holds the rubric→prose direction: the
// range the rulebook advertises must reach the highest rule the gate applies.
//
// Its sibling above holds prose→rubric — every id the prose names must exist —
// and that is the direction a reader survives. A range that overshoots names a
// rule nobody can find. A range that falls SHORT tells a reader the standard is
// smaller than it is, and they have no reason to look further, so nothing else
// in the tree will correct them.
//
// The bound is derived from the rubric, so a T12 is enrolled the day it lands
// and there is no list here to go short.
func TestTheProseRangeReachesTheHighestRule(t *testing.T) {
	const rulebook = "../../../AGENTS.md"

	raw, err := os.ReadFile(rulebook)
	if err != nil {
		t.Fatalf("read %s: %v", rulebook, err)
	}
	section, found := CraftsmanshipSection(string(raw))
	if !found {
		t.Fatalf("%s carries no ## Craftsmanship section", rulebook)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("load rubric: %v", err)
	}

	// The prose wraps, so the range's two ends can sit on different lines;
	// whitespace is normalised before matching rather than the pattern being
	// taught about line breaks.
	flat := strings.Join(strings.Fields(section), " ")
	// Every dash the rulebook might reasonably be written with. A pattern that
	// knew only the ASCII hyphen would find no range in prose using the
	// typographic one — reported below as "states no range at all", which is a
	// true failure but names the wrong cause and sends the reader to edit prose
	// that is already correct.
	//
	// The digit bound is two, which is the whole of any rule set this gate has
	// had. A hundredth rule fails here rather than passing short.
	rangePattern := regexp.MustCompile(`\b([TP])(\d{1,2})\s*[-–—]\s*([TP])(\d{1,2})\b`)

	highest := map[string]int{}
	for _, r := range loaded.Rules {
		prefix := "T"
		if r.Kind == KindPositive {
			prefix = "P"
		}
		n, err := strconv.Atoi(strings.TrimPrefix(r.ID, prefix))
		if err != nil {
			t.Errorf("rule id %q is not <prefix><number>, so the prose range cannot be checked against it", r.ID)
			continue
		}
		if n > highest[prefix] {
			highest[prefix] = n
		}
	}

	covered := map[string]int{}
	for _, m := range rangePattern.FindAllStringSubmatch(flat, -1) {
		if m[1] != m[3] {
			// A range that changes prefix mid-way (T1–P3) states no upper bound
			// for either class; the sibling test already reports it.
			continue
		}
		if n, err := strconv.Atoi(m[4]); err == nil && n > covered[m[1]] {
			covered[m[1]] = n
		}
	}

	for prefix, top := range highest {
		if covered[prefix] == 0 {
			t.Errorf("%s's Craftsmanship section states no %s1–%s%d range at all, so a reader cannot tell how many "+
				"%s-rules the gate applies", rulebook, prefix, prefix, top, prefix)
			continue
		}
		if covered[prefix] < top {
			t.Errorf("%s's Craftsmanship section advertises %s-rules only up to %s%d, but rubric.json carries %s%d. "+
				"A rule added to the standard is one the rulebook never mentions: the reader is not told it exists, "+
				"and unlike a range that overshoots, nothing else will tell them either",
				rulebook, prefix, prefix, covered[prefix], prefix, top)
		}
	}
}
