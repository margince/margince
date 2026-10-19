// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestMeetingHeadersAndCommitmentsKeepRecordedFacts(t *testing.T) {
	langcopytest.Census(t, headerWords)
	langcopytest.Census(t, claimStatusWords)
	for _, language := range []string{"en", "de", "vi"} {
		said := briefCopyFor(language)
		for _, kind := range []string{kindCommitmentOurs, kindCommitmentTheirs, kindOpenQuestion} {
			for _, status := range []string{"open", "done", "dismissed", "future_status"} {
				claim := ClaimIn{Kind: kind, ContactName: "Original Name", Body: "Original quotation", SourceLabel: "Original source", DueAt: new(at(8)), Status: status}
				line := said.commitmentLine(claim)
				for _, fact := range []string{claim.ContactName, claim.Body, claim.SourceLabel, "2026-08-08", said.claimStatus(status)} {
					if !strings.Contains(line, fact) {
						t.Fatalf("%s %s omitted %s: %s", language, kind, fact, line)
					}
				}
				claim.DueAt, claim.SourceLabel = nil, ""
				line = said.commitmentLine(claim)
				if strings.Contains(line, "2026-08-08") || strings.Contains(line, "Original source") {
					t.Fatal("missing metadata was invented")
				}
			}
		}
		in := fullInput()
		in.Language = language
		in.StartsAt = time.Date(2026, 8, 12, 9, 30, 0, 0, time.FixedZone("ICT", 7*60*60))
		line := meetingLine(in)
		for _, fact := range []string{in.Subject, in.Company, "2026-08-12", "09:30 ICT"} {
			if !strings.Contains(line, fact) {
				t.Fatalf("meeting lost %s: %s", fact, line)
			}
		}
		if !strings.Contains(said.dealHeaderLine(*in.Deal), "2026-08-30") {
			t.Fatal("deal close date lost")
		}
		project := ProjectIn{Name: "Original Project", Key: "ABC", Phase: "Original Phase", TargetEndDate: new(at(20))}
		for _, line := range []string{said.projectHeaderLine(project), said.projectGoalLine(project)} {
			for _, fact := range []string{project.Name, project.Phase, "2026-08-20"} {
				if !strings.Contains(line, fact) {
					t.Fatalf("project lost %s: %s", fact, line)
				}
			}
		}
	}
}
