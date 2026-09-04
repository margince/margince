// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// A task's display name is what a person is shown when it breaks, and the
// contract is where it cannot be forgotten.
//
// The AI-work incident row folds several failures of one task into one line,
// so the name on it is the only thing saying which task. Its only alternative
// is the task KEY, which is generated enum vocabulary — so a task declared
// without a name would rejoin the generic fallback silently, and this refuses
// it where the author can still pick words.

import (
	"strings"
	"testing"
)

// displayNameContract is a one-task contract with the name substituted in, so
// each case below differs in exactly the field under test.
func displayNameContract(display string) string {
	return `tiers: [alpha]
degrade_to: {alpha: alpha}
tasks:
  site_triage:
    display_name: ` + display + `
    ladder: [alpha]
    execution_mode: background
    on_budget_exhausted: queue
    status: planned
`
}

func TestATaskWithoutADisplayNameIsRefused(t *testing.T) {
	_, err := parseContract([]byte(displayNameContract(`""`)))
	if err == nil || !strings.Contains(err.Error(), "display_name is required") {
		t.Fatalf("an unnamed task parsed with %v — it would reach the incident row "+
			"with nothing to be called, which is the state this field exists to end", err)
	}
}

// The key restyled is the vocabulary wearing a hat: it reads as words and says
// exactly what the key said. Refused here rather than downstream, where the
// reader has already been shown it.
//
// Case and separator are both disguises, and each was a hole: the lowercase-only
// pattern let `SITE_TRIAGE` through, and `Site_Triage` cleared that AND a
// comparison that had already swapped underscores for spaces. The cases below
// span the family rather than the two spellings that were reported, because the
// check now compares word shape and a per-spelling list would go stale the next
// time somebody invents one.
func TestADisplayNameThatIsJustTheKeyIsRefused(t *testing.T) {
	for name, display := range map[string]string{
		"the key itself":        `"site_triage"`,
		"the key with spaces":   `"site triage"`,
		"the key, capitalised":  `"Site Triage"`,
		"the key SHOUTED":       `"SITE_TRIAGE"`,
		"the key, title-cased":  `"Site_Triage"`,
		"the key, hyphenated":   `"site-triage"`,
		"the key, run together": `"SiteTriage"`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseContract([]byte(displayNameContract(display)))
			if err == nil || !strings.Contains(err.Error(), "the task key in words") {
				t.Fatalf("display_name %s parsed with %v — a reader shown it learns "+
					"exactly what the key would have told them", display, err)
			}
		})
	}
}

func TestARealDisplayNameIsAccepted(t *testing.T) {
	c, err := parseContract([]byte(displayNameContract(`"Website triage"`)))
	if err != nil {
		t.Fatalf("a named task was refused: %v", err)
	}
	if got := c.Tasks["site_triage"].DisplayName; got != "Website triage" {
		t.Errorf("the parsed name is %q, want %q", got, "Website triage")
	}
}

// The emitted lookup is what the incident row reads, so a name that parsed and
// then did not reach the generated table would leave the row unlabelled with
// nothing failing.
func TestTheGeneratedLookupCarriesEveryDisplayName(t *testing.T) {
	c, err := parseContract([]byte(displayNameContract(`"Website triage"`)))
	if err != nil {
		t.Fatal(err)
	}
	out, err := emitGo(c, "deadbeef")
	if err != nil {
		t.Fatalf("emitting: %v", err)
	}
	for _, want := range []string{
		"var taskDisplayNames = map[Task]string{",
		`TaskSiteTriage: "Website triage",`,
		"func DisplayName(t Task) string",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the generated file does not carry %q", want)
		}
	}
}
