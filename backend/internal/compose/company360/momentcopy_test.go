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

func TestAccountDeadlineOneDayUsesSingularCopy(t *testing.T) {
	for _, tc := range []struct{ lang, want string }{
		{"en", "Due in one day."}, {"de", "In einem Tag fällig."}, {"vi", "Đến hạn sau một ngày."},
	} {
		if got := momentCopyFor(tc.lang).owedWhyNow(momentNow, due(1)); got != tc.want {
			t.Errorf("%s: %q", tc.lang, got)
		}
	}
}
