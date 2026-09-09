// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The AI-activity contract must name exactly the work that can reach it, and
// cap exactly what the read caps.
//
// Both halves were held by a gate inside the package the read used to live in.
// That package is gone, and the obligations are not: a spec whose name the
// contract does not carry announces a kind the wire cannot express and the rail
// renders NOTHING for it — silently, with the agent really running. Restored
// here because the root is the only place that can see the catalog, the
// contract and the read's own bounds at once.

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/compose/orgscan"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/modules/people"
)

// The two carrier kinds that are not scheduled specs: a human asking for an
// attached document to be read, and a company's website being read end to end.
//
// Read from each EMITTER's own exported constant rather than restated, so a
// carrier that renames its kind fails here instead of leaving this gate
// vouching for a name nothing writes. The document reading's ai_task and its
// display kind are one string — the reading IS the task — which is why one
// constant answers both there; the website read runs several tasks and names
// none as its own.
const (
	documentReadingKind = activities.ExtractionAITask
	websiteReadingKind  = people.SiteReadActivityKind
	transcriptReadKind  = activities.TranscriptAITask
)

func TestEveryKindSomethingProducesIsOneTheContractCanExpress(t *testing.T) {
	t.Parallel()
	declared := crmYAMLNamedEnum(t, "AiActivityKind")
	if len(declared) == 0 {
		t.Fatal("AiActivityKind declares no enum; this gate would pass vacuously")
	}
	var missing []string
	for _, kind := range producedKinds() {
		if !slices.Contains(declared, kind) {
			missing = append(missing, kind)
			t.Errorf("something announces kind %q and the contract's enum does not carry it — the wire "+
				"cannot express it, and the rail would render nothing for AI work that really happened. "+
				"Add it to the enum and ship its copy in en/de/vi", kind)
		}
	}
	if len(missing) > 0 {
		t.Log(alignEnum(missing))
	}
}

// alignEnum names the file, the schema and exactly what to add.
//
// It deliberately does NOT render a replacement enum, and the reason is worth
// keeping: crmYAMLNamedEnum SORTS what it reads, because its callers do set
// comparisons. So no helper built on it can reproduce the contract's own order
// — and that order is deliberate, opening with the four carrier kinds the
// schema's own description explains. A "paste this" block built from a sorted
// list would move every name in the enum in order to add one: a diff nobody can
// review, and a grouping silently destroyed.
//
// Naming the file and the missing names is the part that was actually missing.
// The per-kind errors above already say why each one matters.
func alignEnum(missing []string) string {
	out := append([]string{}, missing...)
	sort.Strings(out)
	return fmt.Sprintf(
		"align: backend/api/crm.yaml — add %s to the AiActivityKind enum, keeping its existing order "+
			"(the carrier kinds lead it on purpose), then ship each one's copy in en/de/vi",
		strings.Join(out, ", "))
}

// producedKinds is every kind an emitter can announce.
//
// Four producers, and the fourth is why this function is derived rather than
// listed: the ROUTER announces on behalf of every task the rail registry leaves
// to it, under the task's own name. That set grows the moment somebody declares
// a task in api/ai-tasks.yaml, so a list here would be one edit behind the
// contract forever — which is the shape of the defect that left seventeen
// shipped tasks reporting nothing at all.
//
// Both directions of this parity are checked against THIS list, which is why it
// is one function rather than two inline slices — a producer named in only one
// direction is a producer half-gated, and the half that is missing is whichever
// one nobody thought about.
func producedKinds() []string {
	out := []string{documentReadingKind, websiteReadingKind, transcriptReadKind, orgscan.ActivityKind}
	for _, spec := range runner.Catalog() {
		out = append(out, spec.Name)
	}
	for task, source := range ai.RailOwners() {
		if source == ai.SourceRouter {
			out = append(out, task)
		}
	}
	return out
}

// The reverse: a kind nothing can produce is copy three locales carry, a line
// no reader will see, and a promise the server cannot keep.
func TestEveryContractKindHasSomethingThatProducesIt(t *testing.T) {
	t.Parallel()
	produced := producedKinds()
	for _, kind := range crmYAMLNamedEnum(t, "AiActivityKind") {
		if !slices.Contains(produced, kind) {
			t.Errorf("the contract declares kind %q and nothing announces it — either an emitter was "+
				"removed and the enum kept its name, or the name is aspirational. Drop it, or point this "+
				"gate at what produces it", kind)
		}
	}
}

// The read caps its free-text columns on the way to the wire, and the contract
// publishes those caps as maxLength. A cap larger than the published one ships
// a string a strict client rejects; a smaller one truncates below what the
// contract promised a reader would get.
//
// The corpus comes from the contract — each property that publishes a
// maxLength — and the read has to name a bound for each: a list of the
// properties somebody remembered would pass while a newly capped one shipped
// unheld, which is how subject_label shipped, cap published and nothing
// holding it.
func TestTheReadsTextCapsAreTheOnesTheContractPublishes(t *testing.T) {
	t.Parallel()
	held := map[string]int{
		"summary":        aiactivity.SummaryBound,
		"degrade_reason": aiactivity.DegradeReasonBound,
		"subject_label":  aiactivity.SubjectLabelBound,
		"subject_type":   aiactivity.SubjectTypeBound,
	}
	published := crmYAMLMaxLengths(t, "AiActivityItem")
	if len(published) == 0 {
		t.Fatal("AiActivityItem publishes no maxLength at all, so this gate would hold nothing")
	}
	for property, capped := range published {
		bound, ok := held[property]
		if !ok {
			t.Errorf("the contract caps AiActivityItem.%s at %d and the read declares no bound for it — cap it in the read and name the bound here", property, capped)
			continue
		}
		if bound != capped {
			t.Errorf("the read caps %s at %d but the contract publishes maxLength %d", property, bound, capped)
		}
	}
	for property := range held {
		if _, ok := published[property]; !ok {
			t.Errorf("the read caps %s but the contract publishes no maxLength for it", property)
		}
	}
}

// crmYAMLMaxLengths reads every property of one schema that publishes a
// maxLength, keyed by property name.
func crmYAMLMaxLengths(t *testing.T, schema string) map[string]int {
	t.Helper()
	// `maxLength` is OpenAPI's own spelling, so the tag cannot be snake_case:
	// the repo's tag rule is about the shapes WE publish, and this decodes a
	// document whose key names are not ours to choose. Typed rather than a bare
	// map so an absent field stays a distinguishable nil.
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					MaxLength *int `yaml:"maxLength"` //nolint:tagliatelle // OpenAPI's key, not ours to rename
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	shape, ok := doc.Components.Schemas[schema]
	if !ok {
		t.Fatalf("the contract has no schema %s", schema)
	}
	out := map[string]int{}
	for property, prop := range shape.Properties {
		if prop.MaxLength != nil {
			out[property] = *prop.MaxLength
		}
	}
	return out
}

// A spec name that is not a legal message-key segment cannot have copy keyed on
// it, which is the other half of "the rail renders nothing".
func TestEverySpecNameCanBeAMessageKeySegment(t *testing.T) {
	t.Parallel()
	for _, spec := range runner.Catalog() {
		if strings.ContainsAny(spec.Name, " .") || spec.Name == "" {
			t.Errorf("spec name %q cannot key a locale message", spec.Name)
		}
	}
}
