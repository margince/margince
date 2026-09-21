// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// The tool catalog calls the record a COMPANY, and this is what stops the other
// word coming back to it.
//
// companyvocabulary_test.go holds the whole tree against the record type's
// RETIRED name, and says in its own doc why it does not also hold it against
// `account`: that word names a LinkedIn account, a channel Official Account,
// and the ordinary words accountable and accountability, so telling them apart
// is judgement per occurrence, and a gate that covers most of a rule reads as
// covering all of it.
//
// The CATALOG is where that judgement is affordable. It is one file, every
// string in it is written to be read by a model choosing a verb, and the whole
// point of #2025 and #4995 was that a model reading `company` as the record type
// and `account` on the tools operating on it lost a turn to
// `list_records {record_type: "account"}`. So the rule here is the strict one —
// no `account` at all — with each surviving occurrence ratified by what it
// actually means.
//
// The corpus is the PUBLISHED catalog rather than the Go sources that build it:
// docs/reference/mcp-info.json is what a client is served, held to the wiring by
// TestPublishedMCPSurfaceMatchesWhatAClientIsServed, so a word reaching a model
// reaches this gate whichever file spelled it.

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const publishedCatalog = "../docs/reference/mcp-info.json"

// catalogAccountFloor guards against a vacuous pass. The catalog is a few
// hundred kilobytes of description; a walk that found fewer strings than this
// read something other than the catalog, and would report a clean vocabulary
// over a file it could not parse.
const catalogStringFloor = 500

// accountWord matches the retired noun as a whole word, in prose or in a
// snake_case identifier. `_` is a word character to a regexp engine, so the
// ordinary `\b` never fires on `account_notice`; the class below makes a
// letter the only thing that continues the word, which is also what keeps
// `accountable` and `accountability` out.
var accountWord = regexp.MustCompile(`(?i)(^|[^a-z])(accounts?)([^a-z]|$)`)

// otherSenses ratifies each catalog wording that says `account` and does not
// mean this record type.
//
// Keyed by a QUOTE the wording still has to contain, rather than by the tool it
// sits on: a description rewritten around the word loses its ratification and is
// asked again, which is the direction that matters. A quote no catalog string
// carries any more is reported below rather than left standing.
var otherSenses = gatekit.Waive(map[string]string{
	"account_notice": "a communication_context: a notice about the customer's ACCOUNT WITH US — their subscription, their billing " +
		"relationship — which is a consent category in the published enum and not the CRM record type. Renaming it would " +
		"change what a sender is claiming about a message",
	`a workspace with "Key Account" does not want "key accounts" beside it`: "an example of a workspace's OWN tag vocabulary, " +
		"quoted to show that two spellings of one tag split the records that belong together. The words are the reader's, not the product's",
	// One entry per STRING, not per occurrence: forecast_movement's description
	// says it three times — what the buckets account FOR, that they are a
	// complete account OF the change, and that subtracting two readings gives a
	// number with no account of where it went. They are one sense between them.
	"named causes that account for the whole difference": "the ordinary verb, and the ordinary noun twice beside it",
})

func TestTheToolCatalogCallsTheRecordACompany(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile(publishedCatalog)
	if err != nil {
		t.Fatalf("reading the published catalog: %v", err)
	}
	var catalog any
	if err := json.Unmarshal(source, &catalog); err != nil {
		t.Fatalf("parsing the published catalog: %v", err)
	}

	said := catalogStrings(catalog)
	if len(said) < catalogStringFloor {
		t.Fatalf("read %d string(s) out of %s, expected at least %d — a walk this short reports a clean vocabulary over a catalog it did not read",
			len(said), publishedCatalog, catalogStringFloor)
	}

	defer otherSenses.AssertAllMatched(t)
	for _, where := range sortedCatalogKeys(said) {
		word := accountWord.FindStringSubmatch(where)
		if word == nil {
			continue
		}
		if quote, ratified := ratifyingQuote(where); ratified {
			otherSenses.Waived(t, quote)
			continue
		}
		t.Errorf("the published catalog says %q in %q.\n"+
			"\tThe record type is called company, and a model reading one word on the record and another on the tools "+
			"operating on it loses the turn. Say company, or ratify this occurrence in otherSenses with what it means instead.",
			word[2], trimForFinding(where))
	}
}

// ratifyingQuote is the ratified wording this string carries, if any.
func ratifyingQuote(where string) (string, bool) {
	for _, quote := range otherSenses.Subjects() {
		if strings.Contains(where, quote) {
			return quote, true
		}
	}
	return "", false
}

// trimForFinding keeps a finding readable when the string is a whole tool
// description.
func trimForFinding(where string) string {
	if len(where) <= 160 {
		return where
	}
	return where[:157] + "…"
}

// catalogStrings collects the string values anywhere in the document, keyed by
// themselves so one wording is asked about once however many tools carry it.
//
//craft:ignore naked-any a decoded JSON document IS map[string]any, []any and string, and the walk's subject is every string anywhere in it — a narrower parameter would name a shape the catalog does not have
func catalogStrings(node any) map[string]struct{} {
	found := map[string]struct{}{}
	var walk func(any)
	walk = func(n any) {
		switch value := n.(type) {
		case map[string]any:
			for key, child := range value {
				found[key] = struct{}{}
				walk(child)
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		case string:
			found[value] = struct{}{}
		}
	}
	walk(node)
	return found
}

// sortedCatalogKeys orders the corpus, so one run's findings read like the last.
func sortedCatalogKeys(in map[string]struct{}) []string {
	out := make([]string, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
