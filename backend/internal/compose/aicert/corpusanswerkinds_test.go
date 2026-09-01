// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package-internal rather than aicert_test, unlike its three sibling corpus
// gates: the vocabulary is read off firstBuiltRequest, which is unexported. Do
// not "fix" that inconsistency by moving the file — the alternative is
// re-creating the request build here, and a gate that builds its own request
// certifies a copy of one.
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
// The unit is the ENUM, not the site, because a schema can be shared: three
// onboarding conversation sites send companyReadMessageSchema
// (onboardingacts.go, onboardingcompanyanswer.go, onboardingsitereadanswer.go)
// and each narrows it in prose the model is given and this gate cannot read.
// Demanding every kind of every site would demand scenarios the prompt forbids —
// the `acts` prompt permits five of that enum's seven and tells the model
// outright not to emit `correction` or `confirmation`. Per enum, a kind is
// covered when SOME site sharing it scores it, which is the obligation that is
// actually true: the enum is a property of the schema, so the coverage of it is
// too.
//
// Self-scoping, so it needs no maintained list of "the classification tasks": an
// enum is under this obligation exactly when some scenario expects an answer
// naming one of its members. An open-vocabulary site — prose, a per-call citation
// enum whose members are that call's passage ids — never matches and is never
// asked to cover anything. A new closed-vocabulary task comes under the gate the
// moment its first scenario lands, with nothing to remember here.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aitasks"
)

// recognisedOwners is every closed ANSWER vocabulary the corpus reaches, named
// by the sites that send it.
//
// The SET rather than a count, because a count can be masked: a vocabulary that
// stops being recognised is hidden by any unrelated enum that splits in two, and
// the number this gate reports is the number whose falling is the only evidence
// of under-recognition. Naming the owners means a vocabulary cannot go missing
// without the diff saying which one.
//
// Update deliberately when a task's vocabulary genuinely arrives or leaves;
// never edit it to make a run green.
var recognisedOwners = []string{
	"capture_classify/classify",
	"capture_confidentiality_verdict/thread",
	"capture_counterparty_verdict/verdict",
	"cold_start/acts, cold_start/company_message, cold_start/sitereadmessage",
	"propose_roles/committee",
	"signal_extract/thread_events",
	"site_triage/triage",
}

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

	// Keyed by the enum's own members, so two sites sending one schema land on
	// one entry and two sites that merely happen to share a task do not.
	recognised := map[string]*vocabulary{}
	for _, site := range sortedMapKeys(bySite) {
		group := bySite[site]
		named, err := namedKinds(group)
		if err != nil {
			t.Fatalf("%s: %v", site, err)
		}
		if len(named) == 0 {
			continue // nothing in this site's corpus names a kind
		}
		enums, err := siteEnums(group, census)
		if err != nil {
			t.Fatalf("%s: %v", site, err)
		}
		for _, key := range sortedMapKeys(enums) {
			members := enums[key]
			if !namesAny(members, named) {
				continue // not this site's answer vocabulary
			}
			entry, seen := recognised[key]
			if !seen {
				entry = &vocabulary{members: members, sites: map[string]bool{}, covered: map[string]bool{}}
				recognised[key] = entry
			}
			entry.sites[site] = true
			for _, member := range members {
				if named[member] {
					entry.covered[member] = true
				}
			}
		}
	}

	reportUncoveredKinds(t, recognised)

	owners := make([]string, 0, len(recognised))
	for _, entry := range recognised {
		owners = append(owners, strings.Join(sortedMapKeys(entry.sites), ", "))
	}
	sort.Strings(owners)
	if !slices.Equal(owners, recognisedOwners) {
		t.Errorf("closed answer vocabularies are owned by\n  %s\nwant\n  %s\n"+
			"A vocabulary missing here means the derivation stopped reaching it, which is how this gate "+
			"could report PASS having checked less than it did yesterday. A new one wants a decision: "+
			"is it an answer vocabulary whose every kind wants a scenario, or a set of FIELD identifiers a\n"+
			"site merely names? Decide before making this pass.",
			strings.Join(owners, "\n  "), strings.Join(recognisedOwners, "\n  "))
	}
}

// reportUncoveredKinds fails once per vocabulary that admits a kind no scenario
// names, listing the sites that send it so the author knows where a scenario may
// live — the prompts narrow the shared enum differently per site.
func reportUncoveredKinds(t *testing.T, recognised map[string]*vocabulary) {
	t.Helper()
	for _, key := range sortedMapKeys(recognised) {
		entry := recognised[key]
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
			strings.Join(sortedMapKeys(entry.sites), ", "), len(entry.members), len(missing),
			strings.Join(missing, ", "))
	}
}

// siteEnums unions the enums of the request EVERY scenario in the group builds,
// keyed by members.
//
// Every scenario rather than the first: a response schema may be built per call
// from its own fixture — site_extract's citation enum is that call's passage ids
// — so one scenario's schema does not stand for the site's. Reading only the
// first would also make the answer depend on which filename sorts first, a
// dependency the corpus never agreed to.
func siteEnums(group []Scenario, census *aitasks.Registry) (map[string][]string, error) {
	enums := map[string][]string{}
	for _, sc := range group {
		request, err := firstBuiltRequest(context.Background(), sc, census)
		if err != nil {
			return nil, fmt.Errorf("scenario %q: building the request its own case sends: %w", sc.Name, err)
		}
		members, err := collectEnums(request.ResponseSchema)
		if err != nil {
			return nil, fmt.Errorf("scenario %q: %w", sc.Name, err)
		}
		for _, enum := range members {
			enums[strings.Join(enum, "\x00")] = enum
		}
	}
	return enums, nil
}

// namedKinds is every kind this site's ACCEPTED scenarios name, read as the
// string leaves of each expectation.
//
// Leaves rather than the whole value, because an expectation is written in its
// own site's vocabulary and only some sites spell a kind as the entire answer:
// the capture verdicts expect a bare `spam`, while the onboarding sites expect
// {kind: correction, changes: {...}}. Reading only the scalar form counted
// `correction` as unscored while two scenarios asserted it.
//
// Two limits of reading leaves, both worth knowing before trusting a green run.
//
// Coverage means "this string appears somewhere in expect.answer", not "the
// answer's kind field equals it". A schema-path-exact match is not available —
// an expectation is written in the site's own vocabulary and is not
// schema-shaped, which is the whole reason the leaf set is needed — so a future
// scenario with a free-text field whose value happened to equal an enum member
// would mark that kind covered without scoring it. None does today, and a rubric
// quoting a kind name is safe because Rubric is not part of Answer.
//
// A site whose expectations name FIELD identifiers rather than answers is
// therefore not recognised at all, which is the honest outcome rather than an
// exemption: site_extract/profile's 19-value enum is the set of fields the
// extractor may populate, and its accepted scenarios spell those as KEYS
// (`display_name: Kestrel Fold`) whose leaves are values. Only its ABSTENTION
// case lists field names as leaves, and an abstention credits nothing. Should a
// future accepted scenario there name a field as a leaf, the enum becomes
// recognised and recognisedOwners fails, putting the question to a human instead
// of resolving it silently either way.
func namedKinds(group []Scenario) (map[string]bool, error) {
	named := map[string]bool{}
	for _, sc := range group {
		if len(sc.Expect.Answer) == 0 {
			continue
		}
		// Only an ACCEPTED scenario credits coverage. A scenario whose right
		// answer is a refusal or a silence names a kind without ever asking a
		// model to produce it, so crediting it would leave the kind's own branch
		// of the prompt ungraded — and adding one is the cheapest way to silence a
		// genuine miss.
		if sc.Expect.Outcome != aitasks.OutcomeAccepted {
			continue
		}
		var decoded any
		// Not skipped on error: Expect.Answer is a JSONValue this package rendered
		// with json.Marshal, so a value that will not decode means the renderer
		// changed under us. Treating that as "no kinds named" would drop the
		// scenario's coverage silently and could take a whole vocabulary with it.
		if err := json.Unmarshal(sc.Expect.Answer, &decoded); err != nil {
			return nil, fmt.Errorf("scenario %q: expect.answer is not the JSON its JSONValue rendered: %w", sc.Name, err)
		}
		walkExpectation(decoded, named)
	}
	return named, nil
}

// walkExpectation is the ONE place this gate reads a decoded expectation, which
// is why the `any` is here and nowhere else.
//
//craft:ignore naked-any expect.answer is free-form per site by contract — a bare string, a person-to-role map, a list of labels, a nested {kind, changes} object — so there is no shape to name, and naming one would be this gate deciding what a site may assert
func walkExpectation(node any, named map[string]bool) {
	switch typed := node.(type) {
	case string:
		named[typed] = true
	case map[string]any:
		// Map KEYS are deliberately not read. A key names a FIELD, never an answer,
		// and reading them would let a field called `personal` count as covering the
		// kind `personal`.
		for _, value := range typed {
			walkExpectation(value, named)
		}
	case []any:
		for _, item := range typed {
			walkExpectation(item, named)
		}
	}
}

// schemaNode is as much of a JSON Schema as this gate reads: the enums, and the
// two constructs the shipped schemas nest them under. Declared rather than
// walked as decoded JSON so the gate STATES which schema constructs it depends
// on — a schema hiding an enum behind a third construct would be
// under-recognised, which is what the recognisedVocabularies assertion catches.
type schemaNode struct {
	Enum       []json.RawMessage     `json:"enum"`
	Properties map[string]schemaNode `json:"properties"`
	Items      *schemaNode           `json:"items"`
}

// collectEnums returns every all-string enum a built request's response schema
// carries, at any depth: the capture sites nest theirs under
// properties.results.items.properties.
//
// A schema that will not parse is an error rather than zero enums. Zero would
// mean "this site answers from no closed vocabulary", which is the same thing
// this gate reports for an open-vocabulary site — so an unreadable schema would
// disable the obligation and look exactly like the site not having one.
func collectEnums(responseSchema json.RawMessage) ([][]string, error) {
	if len(responseSchema) == 0 {
		return nil, nil // no schema at all: the site constrains nothing
	}
	var root schemaNode
	if err := json.Unmarshal(responseSchema, &root); err != nil {
		return nil, fmt.Errorf("its response schema is not readable as a JSON Schema: %w", err)
	}
	return root.enums(), nil
}

func (n schemaNode) enums() [][]string {
	var found [][]string
	if members, allStrings := stringMembers(n.Enum); allStrings {
		found = append(found, members)
	}
	for _, name := range sortedMapKeys(n.Properties) {
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

// sortedMapKeys keeps every failure naming the same enum, sites and schema
// properties on every run.
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
