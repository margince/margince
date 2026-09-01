// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The corpus census next door (corpus_test.go) is per SITE. A site whose model
// answers from a CLOSED vocabulary satisfies that with ONE scenario and can
// leave most of the vocabulary never once scored — which is how role_mailbox,
// organization_sender and transactional shipped uncertified while the report
// called capture_counterparty_verdict covered, and how the confidentiality
// verdict shipped with four of its seven kinds unmeasured.
//
// A kind nothing scores is a branch of the prompt no model's answer has ever
// been read. For the two capture verdicts that is not a coverage statistic: an
// unscored `personal` is a founder's family published to the workspace as a
// contact, and an unscored `legal` is a dispute opened to everyone.
//
// The vocabulary is read off the REQUEST each site's own code builds, never a
// list kept in this file. The enum in the shipped response schema is the only
// statement of what the model may answer, and a copy of it here would be a
// second answer to that question — the failure captureverdictkinds.go already
// records, where two kinds reached the prompt while the schema still refused
// them and every test stayed green.
//
// The unit is the ENUM, not the site, because a schema can be shared: the four
// onboarding conversation sites all send companyReadMessageSchema and each
// narrows it in prose the model is given and this gate cannot read. Demanding
// every kind of every site would demand scenarios the prompt forbids — the
// `acts` prompt permits five of that enum's seven and tells the model outright
// not to emit `correction` or `confirmation`. Per enum, a kind is covered when
// SOME site sharing it scores it, which is the obligation that is actually true:
// the enum is a property of the schema, so the coverage of it is too.
//
// Self-scoping, so it needs no maintained list of "the classification tasks": an
// enum is under this obligation exactly when some scenario expects an answer
// naming one of its members. An open-vocabulary site — prose, field extraction,
// a per-call citation enum whose members are that call's passage ids — never
// matches and is never asked to cover anything. A new closed-vocabulary task
// comes under the gate the moment its first scenario lands, with nothing to
// remember here.

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

// vocabulary is one closed answer enum, the sites that send it, and the members
// the corpus has scored somewhere among them.
type vocabulary struct {
	members []string
	sites   map[string]bool
	covered map[string]bool
}

func TestEveryClosedAnswerKindCarriesAScenario(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	scenarios, err := LoadCorpus("corpus", census)
	if err != nil {
		t.Fatalf("LoadCorpus(corpus): %v", err)
	}
	if len(scenarios) == 0 {
		t.Fatal("the shipped corpus loaded zero scenarios")
	}

	bySite := map[string][]Scenario{}
	for _, sc := range scenarios {
		bySite[sc.Task+"/"+sc.Site] = append(bySite[sc.Task+"/"+sc.Site], sc)
	}
	sites := make([]string, 0, len(bySite))
	for site := range bySite {
		sites = append(sites, site)
	}
	sort.Strings(sites)

	// Keyed by the enum's own members, so two sites sending one schema land on
	// one entry and two sites that merely happen to share a task do not.
	vocabularies := map[string]*vocabulary{}
	for _, site := range sites {
		group := bySite[site]
		named, keyed := namedAndKeyed(group)
		if len(named) == 0 {
			continue // nothing in this site's corpus names a scalar kind
		}
		request, err := firstBuiltRequest(context.Background(), group[0], census)
		if err != nil {
			t.Errorf("%s: building the request its own case sends: %v", site, err)
			continue
		}
		for _, members := range collectEnums(request.ResponseSchema) {
			if !namesAny(members, named) {
				continue // not this site's answer vocabulary
			}
			// An enum whose members this site uses as expectation KEYS identifies
			// FIELDS, not answers: site_extract/profile's 19-value enum is the
			// set of fields the extractor may populate, and its expectations
			// spell them `display_name: Kestrel Fold` (a key) or list them under
			// `not_grounded` (a value). Coverage there would mean "named in some
			// abstention list", which is satisfied by typing a field name into
			// one — it says nothing about whether any scenario exercises that
			// field. No answer vocabulary is ever keyed: an answer is what a
			// scenario asserts, never what it asserts ABOUT.
			if namesAny(members, keyed) {
				continue
			}
			key := strings.Join(members, "\x00")
			entry, seen := vocabularies[key]
			if !seen {
				entry = &vocabulary{members: members, sites: map[string]bool{}, covered: map[string]bool{}}
				vocabularies[key] = entry
			}
			entry.sites[site] = true
			for _, member := range members {
				if named[member] {
					entry.covered[member] = true
				}
			}
		}
	}

	keys := make([]string, 0, len(vocabularies))
	for key := range vocabularies {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		entry := vocabularies[key]
		var missing []string
		for _, member := range entry.members {
			if !entry.covered[member] {
				missing = append(missing, member)
			}
		}
		if len(missing) == 0 {
			continue
		}
		t.Errorf("the closed vocabulary sent by %s admits %d kinds and %d carry no scenario: %s\n"+
			"Every kind the shipped schema admits must be named by some scenario's expect.answer on one of "+
			"those sites — an unscored kind is a branch of the prompt no model has been graded on. Author it "+
			"on whichever of those sites its own prompt permits.",
			strings.Join(sortedKeys(entry.sites), ", "), len(entry.members), len(missing), strings.Join(missing, ", "))
	}

	// Under-recognition is the one way this gate must not break: it would read a
	// corpus whose answers stopped matching any enum, find nothing to check, and
	// report PASS with no failing assertion to notice. Six closed answer
	// vocabularies are reachable today — the two capture verdicts, capture_classify,
	// propose_roles, site_triage, and the onboarding conversation enum four
	// cold_start sites share — so a smaller count means the derivation stopped
	// reaching them rather than that the tree got better. Raise this with the
	// seventh; never lower it to make a run green.
	const reachableVocabularies = 6
	if len(vocabularies) < reachableVocabularies {
		t.Errorf("recognised %d closed answer vocabular(ies), expected at least %d.\n"+
			"Each is derived from an enum in a site's built request, so this means the derivation stopped "+
			"finding them — not that the corpus improved.", len(vocabularies), reachableVocabularies)
	}
}

// namedAndKeyed reads every kind this site's scenarios NAME and every map key
// they USE, from one walk of each expectation: they are two readings of the same
// tree, and a second walk would be a second chance for the two to disagree about
// what the tree contains.
//
// Leaves rather than the whole value, because an expectation is written in its
// own site's vocabulary and only some sites spell a kind as the entire answer:
// the capture verdicts expect a bare `spam`, while the onboarding sites expect
// {kind: correction, changes: {...}}. Reading only the scalar form counted
// `correction` as unscored while two scenarios asserted it.
//
// Keys are kept APART from leaves rather than pooled, because a key names a
// FIELD and never an answer. Pooling them would let a field called `personal`
// count as covering the kind `personal`, and it is the key set that tells a
// field vocabulary from an answer vocabulary at the call site.
func namedAndKeyed(group []Scenario) (named, keyed map[string]bool) {
	named, keyed = map[string]bool{}, map[string]bool{}
	for _, sc := range group {
		if len(sc.Expect.Answer) == 0 {
			continue
		}
		var decoded any
		if err := json.Unmarshal(sc.Expect.Answer, &decoded); err != nil {
			continue
		}
		walkExpectation(decoded, named, keyed)
	}
	return named, keyed
}

// walkExpectation is the ONE place this gate reads a decoded expectation, which
// is why the `any` is here and nowhere else.
//
//craft:ignore naked-any expect.answer is free-form per site by contract — a bare string, a person-to-role map, a list of labels, a nested {kind, changes} object — so there is no shape to name, and naming one would be this gate deciding what a site may assert
func walkExpectation(node any, named, keyed map[string]bool) {
	switch typed := node.(type) {
	case string:
		named[typed] = true
	case map[string]any:
		for key, value := range typed {
			keyed[key] = true
			walkExpectation(value, named, keyed)
		}
	case []any:
		for _, item := range typed {
			walkExpectation(item, named, keyed)
		}
	}
}

// schemaNode is as much of a JSON Schema as this gate reads: the enums, and the
// two constructs the shipped schemas nest them under. Declared rather than
// walked as decoded JSON so the gate STATES which parts of a schema it depends
// on — a schema hiding an enum somewhere else would be under-recognised, which
// is what the reachableVocabularies floor exists to catch.
type schemaNode struct {
	Enum       []json.RawMessage     `json:"enum"`
	Properties map[string]schemaNode `json:"properties"`
	Items      *schemaNode           `json:"items"`
}

// collectEnums returns every all-string enum a built request's response schema
// carries, at any depth: the capture sites nest theirs under
// properties.results.items.properties.
func collectEnums(responseSchema json.RawMessage) [][]string {
	if len(responseSchema) == 0 {
		return nil
	}
	var root schemaNode
	if err := json.Unmarshal(responseSchema, &root); err != nil {
		return nil
	}
	return root.enums()
}

func (n schemaNode) enums() [][]string {
	var found [][]string
	if members, allStrings := stringMembers(n.Enum); allStrings {
		found = append(found, members)
	}
	for _, name := range sortedNames(n.Properties) {
		found = append(found, n.Properties[name].enums()...)
	}
	if n.Items != nil {
		found = append(found, n.Items.enums()...)
	}
	return found
}

// stringMembers reports an enum's members when every one is a string. A mixed or
// numeric enum is not an answer vocabulary and must not be read as one.
func stringMembers(raw []json.RawMessage) ([]string, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	members := make([]string, 0, len(raw))
	for _, encoded := range raw {
		var text string
		if err := json.Unmarshal(encoded, &text); err != nil {
			return nil, false
		}
		members = append(members, text)
	}
	return members, true
}

// namesAny reports whether the corpus named at least one of this enum's members,
// which is what marks the enum as an answer vocabulary rather than some other
// closed set the same schema happens to carry.
func namesAny(members []string, names map[string]bool) bool {
	for _, member := range members {
		if names[member] {
			return true
		}
	}
	return false
}

// sortedNames and sortedKeys keep a failure naming the same enum, and the same
// sites, on every run.
func sortedNames(m map[string]schemaNode) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
