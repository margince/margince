// Package rubric is the craftsmanship standard the review agent reviews against,
// and rubric.json is the standard rather than a copy of one: it is what the gate
// consumes, it carries the severity model and the block-eligibility the gate acts
// on, and it is versioned. AGENTS.md's Craftsmanship section is the prose form
// for a human or an agent reading the rulebook, plus whatever per-directory
// deltas a nested AGENTS.md adds — the gate is handed that section alongside this
// file, never instead of it.
//
// TestTheProseFormNamesTheSameRules holds two things about the prose, and only
// two: every T- or P-id it names is a real rule here, and it mentions the
// positive rules at all. It does NOT establish full parity — the prose may still
// describe a rule thinly or leave one out.
//
// That is the pair of failures it was written for. The prose advertised an
// anti-tell catalogue of "T1-P3" against a rubric of T1-T10 plus P1-P5: it named
// one rule that does not exist, and said nothing about five that the gate applies.
// The positive rules are part of the standard the model is given; they carry no
// severity, so unlike a BLOCKER or MAJOR anti-tell they do not by themselves stop
// a push.
//
// See architecture/15 (ADR-0045/A60).
package rubric

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed rubric.json
var raw []byte

// Kind distinguishes a forbidden anti-tell from a positive-rubric guideline.
type Kind string

const (
	// KindAntiTell is a forbidden craftsmanship anti-pattern.
	KindAntiTell Kind = "anti_tell"
	// KindPositive is a positive-rubric guideline to uphold.
	KindPositive Kind = "positive"
)

// Rule is one craftsmanship rule. Every rule carries an id, a category, and a
// BLOCK-eligibility flag — only block_eligible categories can produce a merge-blocking verdict.
type Rule struct {
	ID            string `json:"id"`
	Kind          Kind   `json:"kind"`
	Category      string `json:"category"`
	BlockEligible bool   `json:"block_eligible"`
	Title         string `json:"title"`
	Rule          string `json:"rule"`
}

// Rubric is the versioned standard. Version pins one third of the gate's identity
// tuple (prompt, rubric, exemplars, model).
type Rubric struct {
	Version       string            `json:"version"`
	Source        string            `json:"source"`
	Binds         []string          `json:"binds"`
	MetaRule      string            `json:"meta_rule"`
	SeverityModel map[string]string `json:"severity_model"`
	Rules         []Rule            `json:"rules"`
}

// Load returns the embedded rubric.
func Load() (*Rubric, error) {
	var r Rubric
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("parse rubric.json: %w", err)
	}
	return &r, nil
}

// BlockEligible reports whether a finding in this category may produce a BLOCK
// verdict. An unknown category is never block-eligible — the gate fails safe toward PASS.
func (r *Rubric) BlockEligible(category string) bool {
	for _, rule := range r.Rules {
		if rule.Category == category {
			return rule.BlockEligible
		}
	}
	return false
}

// CraftsmanshipHeading is the H2 a rulebook carries its craft deltas under.
const CraftsmanshipHeading = "## Craftsmanship"

// CraftsmanshipSection returns the body of that heading, from the heading itself
// up to the next H2 or end of file, trimmed.
//
// It lives here, and not beside its caller in gate/, because there are two
// readers of this one definition — the gate that feeds the section to the model,
// and the parity test that checks the prose against these rules — and they had
// already drifted apart: one trimmed the result and the other did not. gate/
// imports this package, so this is the side the shared spelling can live on.
func CraftsmanshipSection(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		// The heading EXACTLY, not a prefix: `## Craftsmanship notes` is a
		// different section, and taking it as this one hands the gate somebody's
		// aside as its rubric layer.
		if strings.TrimSpace(line) == CraftsmanshipHeading {
			start = i
			break
		}
	}
	if start < 0 {
		return "", false
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines[start:end], "\n")), true
}
