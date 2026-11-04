// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestAProjectsFindingMapsOntoOneStateAndOneReason(t *testing.T) {
	t.Parallel()
	words := projectHealthCopyFor(textlang.English)
	days := fmt.Sprint(projects.DefaultProjectQuietDays)
	for _, tc := range []struct {
		finding    projectHealthFinding
		state      crmcontracts.ProjectHealthState
		reasonSays []string
	}{
		{projectHealthFinding{Quiet: true, Overdue: true}, crmcontracts.ProjectHealthStateOffTrack, []string{days + " days", "past its due date"}},
		{projectHealthFinding{Quiet: true}, crmcontracts.ProjectHealthStateAtRisk, []string{days + " days"}},
		{projectHealthFinding{Overdue: true}, crmcontracts.ProjectHealthStateAtRisk, []string{"past its due date"}},
		{projectHealthFinding{}, crmcontracts.ProjectHealthStateOnTrack, []string{"no open task is past its due date"}},
	} {
		t.Run(fmt.Sprintf("quiet=%t overdue=%t", tc.finding.Quiet, tc.finding.Overdue), func(t *testing.T) {
			t.Parallel()
			if got := tc.finding.state(); got != tc.state {
				t.Errorf("state = %s, want %s", got, tc.state)
			}
			reason := tc.finding.reason(words)
			for _, part := range tc.reasonSays {
				if !strings.Contains(reason, part) {
					t.Errorf("reason %q does not say %q", reason, part)
				}
			}
			if strings.Count(reason, ".") != 1 || !strings.HasSuffix(reason, ".") {
				t.Errorf("reason %q is not one sentence", reason)
			}
		})
	}
}

// Every shipped language words the reading in its own language, with the
// formatting holes each sentence needs.
func TestEveryShippedLanguageWordsTheHealthReadingItself(t *testing.T) {
	t.Parallel()
	english := projectHealthByLang[textlang.English]
	for _, lang := range textlang.Shipped {
		said, ok := projectHealthByLang[lang]
		if !ok {
			t.Errorf("the product ships %s and the health job's copy has not learned it", lang)
			continue
		}
		for _, field := range []struct {
			name, got, en string
			holes         map[string]int
		}{
			{"quiet", said.quiet, english.quiet, map[string]int{"%d": 1}},
			{"overdue", said.overdue, english.overdue, map[string]int{"%": 0}},
			{"quietAndOverdue", said.quietAndOverdue, english.quietAndOverdue, map[string]int{"%d": 1}},
			{"active", said.active, english.active, map[string]int{"%": 0}},
			{"noticeSubject", said.noticeSubject, english.noticeSubject, map[string]int{"%s": 2}},
			{"noticeUndo", said.noticeUndo, english.noticeUndo, map[string]int{"%": 0}},
			{"onTrack", said.onTrack, english.onTrack, nil},
			{"atRisk", said.atRisk, english.atRisk, nil},
			{"offTrack", said.offTrack, english.offTrack, nil},
		} {
			if field.got == "" {
				t.Errorf("%s: %s is empty", lang, field.name)
				continue
			}
			for verb, want := range field.holes {
				if got := strings.Count(field.got, verb); got != want {
					t.Errorf("%s: %s carries %d %s, want %d: %q", lang, field.name, got, verb, want, field.got)
				}
			}
			if lang != textlang.English && field.got == field.en {
				t.Errorf("%s: %s is the English text verbatim", lang, field.name)
			}
		}
	}
}
