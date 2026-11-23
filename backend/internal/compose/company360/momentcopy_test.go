// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"reflect"
	"strings"
	"testing"
	"time"

	contracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestAccountMomentCatalogIsComplete(t *testing.T) {
	langcopytest.Census(t, momentWords)
}

func TestAccountMomentLanguageKeepsEvidenceAndActions(t *testing.T) {
	claim := commitment("Original promise", -2, "Carol")
	step := task("Original task", 3)
	undated := step
	undated.DueAt = nil
	today := task("Today", 0)
	contactTask := step
	contactTask.LinkedContactId = new(step.ActivityId)
	dealTask := step
	dealTask.LinkedDealId = new(step.ActivityId)
	hoursLate := step
	hours := momentNow.Add(-2 * time.Hour)
	hoursLate.DueAt = &hours
	cases := []struct {
		name   string
		tasks  []contracts.Company360NextStep
		claims []contacts.CompanyCommitment
	}{
		{name: "quiet"},
		{name: "today", tasks: []contracts.Company360NextStep{today}},
		{name: "contact", tasks: []contracts.Company360NextStep{contactTask}},
		{name: "deal", tasks: []contracts.Company360NextStep{dealTask}},
		{name: "hours late", tasks: []contracts.Company360NextStep{hoursLate}},
		{name: "claim", claims: []contacts.CompanyCommitment{claim}},
		{name: "task", tasks: []contracts.Company360NextStep{step}},
		{name: "undated", tasks: []contracts.Company360NextStep{undated}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			render := func(lang string) contracts.ContactMoment {
				return momentCopyFor(lang).accountMoment(momentNow, tc.tasks, []time.Time{momentNow}, tc.claims)
			}
			english := render("en")
			for _, lang := range textlang.Shipped {
				got := render(string(lang))
				if got.ClaimKey != english.ClaimKey || got.EvidenceFingerprint != english.EvidenceFingerprint || !reflect.DeepEqual(got.Evidence, english.Evidence) {
					t.Fatalf("%s changed evidence or identity", lang)
				}
				if got.RecommendedAction.Kind != english.RecommendedAction.Kind || got.RecommendedAction.State != english.RecommendedAction.State || !reflect.DeepEqual(got.RecommendedAction.Destination, english.RecommendedAction.Destination) {
					t.Fatalf("%s changed action effects", lang)
				}
				if lang != textlang.English && (got.Headline == english.Headline || got.WhyNow == english.WhyNow || got.RecommendedAction.Label == english.RecommendedAction.Label) {
					t.Fatalf("%s retained English copy: %+v", lang, got)
				}
				if strings.Contains(got.Headline+got.WhyNow, "%!") {
					t.Fatal("unbound format placeholder")
				}
			}
		})
	}
}

// 23:00 and 01:00 the next morning in Bangkok share a UTC date, so the card
// says "tomorrow" only if it counts in the zone it was given.
func TestAccountDeadlineCountsCalendarDaysInTheInstallationZone(t *testing.T) {
	bangkok := time.FixedZone("UTC+7", 7*60*60)
	lateEvening := time.Date(2026, 9, 2, 23, 0, 0, 0, bangkok)
	tonight := time.Date(2026, 9, 2, 23, 30, 0, 0, bangkok)
	tomorrowMorning := time.Date(2026, 9, 3, 1, 0, 0, 0, bangkok)
	earlyMorning := time.Date(2026, 9, 2, 1, 0, 0, 0, bangkok)
	yesterdayEvening := time.Date(2026, 9, 1, 23, 0, 0, 0, bangkok)
	for _, tc := range []struct{ lang, today, tomorrow, late string }{
		{"en", "Due today.", "Due tomorrow.", "Due one day ago and still open."},
		{"de", "Heute fällig.", "Morgen fällig.", "Seit einem Tag fällig und noch offen."},
		{"vi", "Đến hạn hôm nay.", "Đến hạn ngày mai.", "Quá hạn một ngày và vẫn đang mở."},
	} {
		said := momentCopyFor(tc.lang)
		said.zone = bangkok
		if got := said.owedWhyNow(lateEvening, &tonight); got != tc.today {
			t.Errorf("%s, due 23:30 tonight: %q, want %q", tc.lang, got, tc.today)
		}
		if got := said.owedWhyNow(lateEvening, &tomorrowMorning); got != tc.tomorrow {
			t.Errorf("%s, due 01:00 tomorrow: %q, want %q", tc.lang, got, tc.tomorrow)
		}
		if got := said.owedWhyNow(earlyMorning, &yesterdayEvening); got != tc.late {
			t.Errorf("%s, due 23:00 yesterday, read at 01:00: %q, want %q", tc.lang, got, tc.late)
		}
	}
}
