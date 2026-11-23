// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// One shared rule answers "which day is it for this reader". A production file
// that works out a local day or zone must go through the rule's own code. Or it
// sits in the allow-list below, with the reason it is a combined surface.
// The files are read from the tree. A new reader that computes a day its own way
// fails here until someone decides which kind it is.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// localDaySpellings are the ways code resolves a calendar day or zone.
var localDaySpellings = regexp.MustCompile(
	`installationZone\(|TaskDue(Before|From|Order)SQL\(|InstallationZoneSQL|CURRENT_DATE|now\(\)::date|AT TIME ZONE|` +
		`date_trunc\('day'|Truncate\(24 ?\* ?time\.Hour\)|identity\.TimezoneOf|TimezoneAppliedTx|ZoneAppliedTx|TodaySQL\(`)

// ruleOwned is the code that implements the rule or owns the stored zone.
var ruleOwned = []string{
	"internal/compose/dayrule/",
	"internal/shared/kernel/occurredday/",
	"internal/platform/database/storekit/taskdueday.go",
	"internal/modules/identity/",
}

const (
	combined = "a screen or job that combines several colleagues reads the installation's day"
	record   = "a record-centred view reads the installation's day, one answer for every viewer"
	utcOnly  = "UTC arithmetic or one installation-wide rate day, not a reader's calendar day"
	override = "reads the installation zone as the default and takes the surface's zone from its caller"
)

// allowed maps a path prefix to why it may read the installation's day. A file
// reaches the rule through dayrule instead when its surface is a member's own.
var allowed = gatekit.Waive(map[string]string{
	"internal/compose/accountcoverage.go":                     combined,
	"internal/compose/attention/dayboundary.go":               override,
	"internal/compose/attentionlanesseam.go":                  "relationship quiet days count in the installation zone, as the contact record shows them",
	"internal/compose/attentionseam.go":                       override,
	"internal/compose/automaticwork.go":                       combined,
	"internal/compose/automaticworksql.go":                    combined,
	"internal/compose/commitmentadmission.go":                 "a promise's due day is admitted on the seat's own working-hours zone",
	"internal/compose/company360/":                            record,
	"internal/compose/companyrollupread.go":                   record,
	"internal/compose/contact360/":                            record,
	"internal/compose/controllermailwiring.go":                "binds the installation zone for consent proof dates",
	"internal/compose/datatask_requestproof.go":               "a data task over the installation's proofs",
	"internal/compose/dealstatus/":                            record,
	"internal/compose/draftenvelope.go":                       "the installation zone for a draft's send floor",
	"internal/compose/dueday/":                                "turns a stated due day into its end; an empty zone means the installation's",
	"internal/compose/forecastevaluation.go":                  combined,
	"internal/compose/forecastseam.go":                        combined,
	"internal/compose/handoffseam.go":                         record,
	"internal/compose/installseam/":                           "installation set-up reads the installation zone",
	"internal/compose/integration/harness.go":                 "the test harness seeds the installation zone",
	"internal/compose/jobs_pastactivityclock.go":              combined,
	"internal/compose/jobs_riskverdict.go":                    combined,
	"internal/compose/lifecyclereview/":                       record,
	"internal/compose/network/":                               record,
	"internal/compose/owedclassify.go":                        combined,
	"internal/compose/reportingcontext.go":                    combined,
	"internal/compose/reportprojects.go":                      combined,
	"internal/compose/reportsql.go":                           combined,
	"internal/compose/salesprogress/":                         record,
	"internal/compose/sdrmeetings.go":                         combined,
	"internal/compose/serverassembly.go":                      "binds the installation zone into the stores",
	"internal/compose/settingswiring.go":                      "the settings page reads and writes the installation zone",
	"internal/compose/stageevidencetrigger.go":                combined,
	"internal/compose/winchance/":                             combined,
	"internal/compose/weekly/":                                "the weekly review job is one pass over all reps, on the installation's week",
	"internal/compose/weeklyplanseam.go":                      "a plan's week is the installation's week",
	"internal/modules/activities/activityread.go":             override,
	"internal/modules/activities/meetingnoonrepair.go":        "a data task over the installation's meetings",
	"internal/modules/activities/overdueload.go":              "the team board counts every colleague's day on the installation zone",
	"internal/modules/ai/":                                    utcOnly,
	"internal/modules/automation/handlers_clock.go":           combined,
	"internal/modules/capture/":                               utcOnly,
	"internal/modules/consent/":                               "consent proof dates count in the installation's zone",
	"internal/modules/contacts/":                              record,
	"internal/modules/deals/":                                 record,
	"internal/modules/integrations/":                          utcOnly,
	"internal/modules/search/queryjoins.go":                   record,
	"internal/platform/database/storekit/predicateoperand.go": "a saved filter's \"days ago\" is one answer for every viewer",
	"internal/shared/kernel/dealseat/":                        record,
	"internal/shared/kernel/dormancy/":                        record,
	"internal/shared/kernel/employment/":                      record,
	"internal/shared/kernel/relstrength/":                     utcOnly,
})

// minimumLocalDayFiles fails the census when the walk finds fewer files than
// the tree holds. A walk that misses subjects reads a smaller tree and passes.
const minimumLocalDayFiles = 80

// allowedPrefix is the allow-list entry that covers path, if any.
func allowedPrefix(path string) (string, bool) {
	for _, prefix := range allowed.Subjects() {
		if path == prefix || (strings.HasSuffix(prefix, "/") && strings.HasPrefix(path, prefix)) {
			return prefix, true
		}
	}
	return "", false
}

func TestEveryLocalDayComputationGoesThroughTheRuleOrIsDeclared(t *testing.T) {
	t.Parallel()
	found := 0
	walkHandWrittenGoFiles(t, "internal", func(path, text string) {
		if strings.HasSuffix(path, "_test.go") || strings.HasPrefix(path, "internal/contracts/") {
			return
		}
		if !localDaySpellings.MatchString(text) {
			return
		}
		found++
		for _, owned := range ruleOwned {
			if strings.HasPrefix(path, owned) {
				return
			}
		}
		prefix, ok := allowedPrefix(path)
		if !ok {
			t.Errorf("%s works out a local day or zone (%s) without the day rule. Go through compose/dayrule or "+
				"occurredday, or add it to the allow-list with the reason it is a combined surface",
				path, localDaySpellings.FindString(text))
			return
		}
		allowed.Waived(t, prefix)
	})
	if found < minimumLocalDayFiles {
		t.Fatalf("found %d files that compute a local day, want at least %d: the census is too short", found, minimumLocalDayFiles)
	}
	allowed.AssertAllMatched(t)
}

// Files that serve a member's own list and a combined screen from one path must
// pick the surface through the rule.
var mustPickSurface = []string{
	"internal/compose/slipping.go", "internal/compose/attentionseam.go",
	"internal/compose/briefjobs.go", "internal/compose/briefs/briefday.go",
	"internal/compose/commercialseams.go", "internal/compose/attentionplanseam.go",
	"internal/compose/attentionriskseam.go",
}

func TestTheReadersThatServeBothSurfacesPickOneThroughTheRule(t *testing.T) {
	t.Parallel()
	picked := map[string]bool{}
	walkHandWrittenGoFiles(t, "internal", func(path, text string) {
		if !strings.HasSuffix(path, "_test.go") && !strings.HasPrefix(path, "internal/compose/dayrule/") &&
			(strings.Contains(text, "compose/dayrule\"") || strings.Contains(text, "occurredday.Personal")) {
			picked[path] = true
		}
	})
	for _, path := range mustPickSurface {
		if !picked[path] {
			t.Errorf("%s no longer picks a surface through the day rule", path)
		}
	}
	if len(picked) < len(mustPickSurface) {
		t.Fatalf("found %d files that pick a surface, want at least %d: the census is too short", len(picked), len(mustPickSurface))
	}
}
