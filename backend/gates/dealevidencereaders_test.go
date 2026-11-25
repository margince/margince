// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A Go reader that judges whether a deal is alive asks
// relstrength.DealEvidenceSQL with the deal it reads. A personal thread, or a
// link marked not about the deal, then keeps no deal warm. The census is every
// hand-written file whose SQL names a deal and an activity recency rule. Each
// must call the check or say here why it need not.

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// dealEvidenceReaders are the deal-recency readers that carry the check. The
// last four name no recency rule, so the census cannot find them.
var dealEvidenceReaders = []string{
	"internal/compose/assuranceseam.go",
	"internal/modules/deals/championcover.go",
	"internal/modules/deals/formulas.go",
	"internal/modules/deals/health.go",
	"internal/modules/deals/quietfacts.go",
	"internal/compose/dealverdictinput.go",
	"internal/compose/dealprivatemail/dealprivatemail.go",
	"internal/modules/deals/stagemoveverdict.go",
	"internal/compose/weekly/weeklyscorecard.go",
}

// gatekit:fixture the census hits that need not call the check, each with why.
var dealEvidenceExempt = map[string]string{
	"internal/compose/dealclosefacts.go":             "keeps a deal open on relstrength.DealKeptOpenSQL, so doubt closes nothing; its refusal reads customerRefusalAnyAudienceSQL, which carries the check",
	"internal/compose/jobs_dealbackfill.go":          "asks whether a close for silence was premature through customerProgressAnyAudienceSQL, the check dealclosefacts.go makes of the same close, and keeps a deal open on relstrength.DealKeptOpenSQL",
	"internal/compose/company360/pipelineread.go":    "reads deals.MeetingAheadSQL, which carries it",
	"internal/modules/deals/closedaterun.go":         "reads deals.MeetingAheadSQL, which carries it",
	"internal/modules/deals/deal_singleread.go":      "reads deals.MeetingAheadSQL, which carries it",
	"internal/modules/deals/lastemail.go":            "shows the newest email on the card; a personal thread is already participants-only",
	"internal/compose/meetingbrief/meeting.go":       "briefs a meeting about its attendees, not about a deal being alive",
	"internal/compose/meetingbrief/history.go":       "briefs a meeting about its attendees, not about a deal being alive",
	"internal/compose/meetingbrief/whatchanged.go":   "briefs a meeting about its attendees, not about a deal being alive",
	"internal/modules/contacts/lead_read.go":         "a lead's own touch, before any deal",
	"internal/modules/activities/activityfilter.go":  "a list filter on any record, not a deal judgement",
	"internal/modules/activities/lasttouch.go":       "the no-activity reminder of any record, the relationship's clock",
	"internal/modules/activities/projectcoverage.go": "a project's last update",
	"internal/modules/activities/awaitingreply.go":   "who owes a reply on a thread, not whether a deal is alive",
	"internal/modules/activities/meetingfollowup.go": "follow-up for a meeting that took place, not a deal judgement",
}

var (
	dealRecencyRuleRe = regexp.MustCompile(`relstrength\.(HadContactSQL|MeetingAheadSQL|LastActivityCountsSQL|ExternalActivityCountsSQL|InteractionCountsSQL|InteractionCountsAtSQL|MeetingInTouchSQL)\(|\bMeetingAheadSQL\(|healthInteraction\(|customerProgress\w*SQL\(`)
	dealNameRe        = regexp.MustCompile(`deal_id|DealID|deal\.id`)
)

const dealEvidenceCall = "DealEvidenceSQL("

// dealRecencyCensusFloor is below the readers this census finds today.
const dealRecencyCensusFloor = 15

func TestEveryDealRecencyReaderAsksTheDealEvidenceCheck(t *testing.T) {
	t.Parallel()
	found := map[string]bool{}
	err := gatekit.WalkDir("internal", func(rel string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(rel, ".go") ||
			strings.HasSuffix(rel, "_test.go") || strings.HasSuffix(rel, "_gen.go") {
			return err
		}
		body, err := gatekit.ReadFile(rel) // #nosec G304 -- a *.go file from walking the trusted backend tree
		if err != nil {
			return err
		}
		src := string(body)
		if !dealRecencyRuleRe.MatchString(src) || !dealNameRe.MatchString(src) {
			return nil
		}
		found[rel] = true
		if strings.Contains(src, dealEvidenceCall) || dealEvidenceExempt[rel] != "" {
			return nil
		}
		if !slices.Contains(dealEvidenceReaders, rel) {
			t.Errorf("%s reads an activity recency rule about a deal and never calls relstrength.DealEvidenceSQL, "+
				"so a personal thread or a link marked not about the deal can keep it warm. Call it, or name why not in dealEvidenceExempt", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal: %v", err)
	}
	if len(found) < dealRecencyCensusFloor {
		t.Fatalf("the census found %d deal-recency readers, under its floor of %d: the walk read the wrong place", len(found), dealRecencyCensusFloor)
	}
	for rel := range dealEvidenceExempt {
		if !found[rel] {
			t.Errorf("%s is exempt but the census no longer finds it. Remove the stale entry", rel)
		}
	}
	for _, rel := range dealEvidenceReaders {
		src, err := gatekit.ReadFile(rel)
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			continue
		}
		if !strings.Contains(string(src), dealEvidenceCall) {
			t.Errorf("%s no longer calls relstrength.DealEvidenceSQL, so a personal thread or a link marked not about the deal can keep it warm", rel)
		}
	}
}
