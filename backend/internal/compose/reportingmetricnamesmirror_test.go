// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Two copies of the catalog's metric names live where the catalog cannot reach.
// One is the frontend's test fixture, and one the weekly mail's platform copy.
// Each is held to reportingMetricNames, so a renamed metric fails here rather
// than leaving a test or a mail on the old name.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

const frontendReportingFixture = "../../../frontend/src/testing/reporting.fixtures.ts"

// fixtureMetricName reads one `id: "Name"` pair, the id bare or quoted.
var fixtureMetricName = regexp.MustCompile(`(?m)^\s*["']?([a-z_]+)["']?\s*:\s*"([^"]*)"\s*,?\s*$`)

func TestTheFrontendFixtureNamesEveryMetricAsTheCatalogDoes(t *testing.T) {
	source, err := os.ReadFile(frontendReportingFixture)
	if err != nil {
		t.Fatalf("reading the frontend reporting fixture: %v", err)
	}
	const opening = "export const reportingMetricNames"
	start := strings.Index(string(source), opening)
	if start < 0 {
		t.Fatalf("%s no longer declares %s", frontendReportingFixture, opening)
	}
	block, _, closed := strings.Cut(string(source)[start:], "\n};")
	if !closed {
		t.Fatalf("%s: %s has no closing brace", frontendReportingFixture, opening)
	}
	browser := map[string]string{}
	for _, pair := range fixtureMetricName.FindAllStringSubmatch(block, -1) {
		browser[pair[1]] = pair[2]
	}
	for id, phrase := range reportingMetricNames {
		got, named := browser[string(id)]
		if !named {
			t.Errorf("the fixture does not name %s; the catalog calls it %q", id, phrase.En)
		} else if got != phrase.En {
			t.Errorf("the fixture calls %s %q; the catalog calls it %q", id, got, phrase.En)
		}
	}
	for id := range browser {
		if _, served := reportingMetricNames[crmcontracts.ReportingMetricID(id)]; !served {
			t.Errorf("the fixture names %s, which the catalog does not serve", id)
		}
	}
}

func TestTheWeeklyMailNamesMeetingsHeldAsTheCatalogDoes(t *testing.T) {
	catalog := reportingMetricNames[reportingMeetingsHeld]
	for language, want := range map[mailcopy.Language]string{
		mailcopy.English: catalog.En, mailcopy.German: catalog.De, mailcopy.Vietnamese: catalog.Vi,
	} {
		if got := mailcopy.For(string(language)).WeeklyMeetings; got != want {
			t.Errorf("the %s weekly mail says %q; the catalog calls the metric %q", language, got, want)
		}
	}
}
