// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// The technical lookup reads the domain the RECORD holds, and nothing else.
//
// That is the guardrail the whole feature's legal position rests on: the
// lookup touches one domain a workspace already recorded, so this path cannot
// discover companies nobody asked about. A second source of the domain — a
// request body, a job argument, a caller-supplied override — would not fail
// any test in the tree, and would quietly turn a company-record enrichment
// into a way to research arbitrary domains.
//
// So the two gates below fail when one appears.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// technicalDomainReaders matches a SQL statement that SELECTS the domain
// value out of organization_domain.
//
// The value is the point, not the table: the sweep's due-list asks whether a
// company has any domain at all (an EXISTS with no column read), which decides
// nothing about which domain is looked up and is deliberately not matched.
// What must stay singular is the place that answers "this string is the domain
// we may ask about".
//
// Derived from the statement rather than from a list of files, because the
// defect this catches is a NEW reader in a file nobody thought to list.
var technicalDomainReaders = regexp.MustCompile(`(?s)SELECT\s+domain\s`)

// TestTheTechnicalLookupReadsTheDomainFromTheRecordAlone holds the claim in
// people.Store.TechnicalDomain's doc comment.
//
// It asserts the narrow thing that is actually true: within the technical
// lookup's own files, exactly one function reads the domain. Other paths in
// the module read organization_domain for their own reasons — the list filter,
// the dedupe — and this says nothing about those.
func TestTheTechnicalLookupReadsTheDomainFromTheRecordAlone(t *testing.T) {
	t.Parallel()
	const technicalFiles = "internal/modules/people"
	entries, err := os.ReadDir(technicalFiles)
	if err != nil {
		t.Fatalf("reading the people module: %v", err)
	}
	readers := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "technical") && !strings.HasPrefix(name, "organizationtechnical") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(technicalFiles, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		readers += len(technicalDomainReaders.FindAll(body, -1))
	}
	if readers != 1 {
		t.Errorf("the technical lookup's files select a domain value %d times, want exactly 1 "+
			"(Store.TechnicalDomain). A second reader is a second answer to 'which domain may we ask "+
			"about', and the guardrail is that only the record decides", readers)
	}
}

// TestTheTechnicalLookupTakesNoDomainFromACaller is the other half, and the
// one that matters more: the domain must not be reachable from OUTSIDE.
//
// A request body field or a job argument would let a caller name the domain,
// which is exactly the company-discovery path this feature promises not to be.
func TestTheTechnicalLookupTakesNoDomainFromACaller(t *testing.T) {
	t.Parallel()
	for _, subject := range []struct{ path, what string }{
		{"internal/compose/jobs_techenrich.go", "the queued job's arguments"},
		{"internal/compose/techenrichtransport.go", "the HTTP request"},
	} {
		body, err := os.ReadFile(subject.path)
		if err != nil {
			t.Fatalf("reading %s: %v", subject.path, err)
		}
		// A Domain field on an args struct or a decoded request is the shape
		// this refuses. The worker and the handler both receive an
		// organization id and read the domain from the record.
		if regexp.MustCompile(`(?m)^\s*Domain\s+string`).Match(body) {
			t.Errorf("%s carries a domain in %s — the lookup reads the one the record holds, "+
				"and a caller-supplied domain is how this becomes company discovery",
				subject.path, subject.what)
		}
	}
}

// TestEveryTechnicalLaneIsDerivedFromItsFields holds the claim on
// people.technicalLanes: it is every lane, derived rather than written twice.
//
// A hand-written second list would fall out of step with laneFields, and the
// sweep would then treat a company as fully looked at while one lane had never
// run — which reads on the record exactly like a company that publishes
// nothing of what that lane reads.
func TestEveryTechnicalLaneIsDerivedFromItsFields(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("internal/modules/people/organizationtechnical.go")
	if err != nil {
		t.Fatalf("reading the technical apply: %v", err)
	}
	if !regexp.MustCompile(`technicalLanes\s*=\s*func\(\)\s*\[\]TechnicalLane\s*\{[\s\S]*?range laneFields`).Match(body) {
		t.Error("technicalLanes no longer derives from laneFields — a second list of the lanes " +
			"falls out of step with the fields each one owns, and the sweep then calls a company " +
			"fully looked at while a lane has never run")
	}
}
