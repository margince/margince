// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

package gates

// A record's source channel is spelled three times: provenance.RecordSources(),
// the table's CHECK and the contract's enum. A value in Go and not in the CHECK
// fails the write at run time; one in the contract and not in Go is refused 422.

import (
	"bufio"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// recordSourceHeld maps a record table whose source is held to the list onto
// the create requests that carry it. A partner has no source on the wire.
var recordSourceHeld = map[string][]string{
	"deal":    {"CreateDealRequest"},
	"offer":   {"CreateOfferRequest"},
	"partner": nil,
	"project": {"CreateProjectRequest"},
}

// recordSourceFree lists the record tables whose writers still store other
// words. A table that gains the check moves to recordSourceHeld.
var recordSourceFree = []string{"activity", "company", "contact", "lead", "signal"}

var (
	recordSourceCheckRE = regexp.MustCompile(
		`^public\.([a-z_]+)\.([a-z_]+)_source_check CHECK \(\(source = ANY \(ARRAY\[([^\]]*)\]\)\)\)`)
	contractSchemaRE     = regexp.MustCompile(`^    ([A-Za-z0-9]+):$`)
	contractSourceEnumRE = regexp.MustCompile(`^        source: \{ type: string, enum: \[([^\]]*)\]`)
)

// recordSourceChecks reads each table's source check out of the head catalog.
func recordSourceChecks(t *testing.T, catalog string) map[string][]string {
	t.Helper()
	checks := map[string][]string{}
	for line := range strings.SplitSeq(catalog, "\n") {
		match := recordSourceCheckRE.FindStringSubmatch(line)
		if match == nil || match[1] != match[2] {
			continue
		}
		checks[match[1]] = splitCatalogTokens(match[3])
	}
	return checks
}

// contractSourceEnums reads the enum of each schema's source property.
func contractSourceEnums(contract string) map[string][]string {
	enums := map[string][]string{}
	schema := ""
	scanner := bufio.NewScanner(strings.NewReader(contract))
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for scanner.Scan() {
		line := scanner.Text()
		if match := contractSchemaRE.FindStringSubmatch(line); match != nil {
			schema = match[1]
			continue
		}
		if match := contractSourceEnumRE.FindStringSubmatch(line); match != nil {
			var values []string
			for value := range strings.SplitSeq(match[1], ",") {
				values = append(values, strings.TrimSpace(value))
			}
			slices.Sort(values)
			enums[schema] = values
		}
	}
	return enums
}

// recordSourceFindings names each place the list is spelled differently, and
// each record table that is neither held nor declared free.
func recordSourceFindings(vocabulary []string, checks, enums map[string][]string) []string {
	var findings []string
	for table, schemas := range recordSourceHeld {
		check, ok := checks[table]
		switch {
		case !ok:
			findings = append(findings, table+": no "+table+"_source_check in the head catalog")
		case !equalSets(vocabulary, check):
			findings = append(findings, table+": the CHECK allows "+strings.Join(check, ", "))
		}
		for _, schema := range schemas {
			if !equalSets(vocabulary, enums[schema]) {
				findings = append(findings, schema+": the contract's source enum is ["+strings.Join(enums[schema], ", ")+"]")
			}
		}
	}
	for _, table := range recordSourceFree {
		if _, ok := checks[table]; ok {
			findings = append(findings, table+": has a source check, so move it to recordSourceHeld with its schemas")
		}
	}
	slices.Sort(findings)
	return findings
}

func TestTheRecordSourceVocabularyIsTheCatalogsCheck(t *testing.T) {
	t.Parallel()
	catalog, err := gatekit.ReadFile(replyVerdictCatalog)
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	contract, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	if len(recordSourceHeld)+len(recordSourceFree) != 9 {
		t.Fatalf("the gate names %d record tables, and there are nine", len(recordSourceHeld)+len(recordSourceFree))
	}
	vocabulary := provenance.RecordSources()
	for _, finding := range recordSourceFindings(vocabulary, recordSourceChecks(t, string(catalog)), contractSourceEnums(string(contract))) {
		t.Errorf("%s; provenance.RecordSources() spells %v", finding, vocabulary)
	}
}

func TestTheRecordSourceGateSeesEachWayTheListCanDrift(t *testing.T) {
	t.Parallel()
	vocabulary := []string{"api", "manual"}
	held := func() (checks, enums map[string][]string) {
		checks, enums = map[string][]string{}, map[string][]string{}
		for table, schemas := range recordSourceHeld {
			checks[table] = vocabulary
			for _, schema := range schemas {
				enums[schema] = vocabulary
			}
		}
		return checks, enums
	}
	checks, enums := held()
	if findings := recordSourceFindings(vocabulary, checks, enums); len(findings) != 0 {
		t.Fatalf("an agreeing tree was reported: %v", findings)
	}
	for name, drift := range map[string]func(checks, enums map[string][]string){
		"a check with one more value": func(checks, _ map[string][]string) { checks["deal"] = []string{"api", "manual", "seed"} },
		"a held table with no check":  func(checks, _ map[string][]string) { delete(checks, "partner") },
		"a contract enum that is shorter": func(_, enums map[string][]string) {
			enums["CreateOfferRequest"] = []string{"manual"}
		},
		"a contract schema with no enum":   func(_, enums map[string][]string) { delete(enums, "CreateProjectRequest") },
		"a free table that gained a check": func(checks, _ map[string][]string) { checks["lead"] = vocabulary },
	} {
		checks, enums := held()
		drift(checks, enums)
		if findings := recordSourceFindings(vocabulary, checks, enums); len(findings) != 1 {
			t.Errorf("%s: got findings %v, want one", name, findings)
		}
	}
}

func TestTheRecordSourceReadersParseTheCommittedSpellings(t *testing.T) {
	t.Parallel()
	checks := recordSourceChecks(t, strings.Join([]string{
		`public.deal.deal_source_check CHECK ((source = ANY (ARRAY['api'::text, 'manual'::text]))) NOT VALID`,
		`public.partner.partner_source_check CHECK ((source = ANY (ARRAY['manual'::text])))`,
		`public.deal.offer_source_check CHECK ((source = ANY (ARRAY['manual'::text])))`,
		`public.signal.signal_source_channel_check CHECK ((source_channel = ANY (ARRAY['derived'::text])))`,
	}, "\n"))
	if len(checks) != 2 || !equalSets(checks["deal"], []string{"api", "manual"}) || !equalSets(checks["partner"], []string{"manual"}) {
		t.Errorf("checks = %v, want deal and partner only", checks)
	}
	enums := contractSourceEnums(strings.Join([]string{
		"    Deal:",
		"        source: { type: string, enum: [manual, api], x-go-type: string }",
		"    Lead:",
		"        source: { type: string }",
	}, "\n"))
	if len(enums) != 1 || !equalSets(enums["Deal"], []string{"api", "manual"}) {
		t.Errorf("enums = %v, want Deal only", enums)
	}
}
