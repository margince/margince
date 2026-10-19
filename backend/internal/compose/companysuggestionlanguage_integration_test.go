// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCompanySuggestionUsesBaseLanguageWithoutChangingEvidence(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Example Company", &e.Rep1)
	subject, body, direction := "Original quotation", "Original customer words.", "outbound"
	at := now.AddDate(0, 0, -10)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: company}},
	}); err != nil {
		t.Fatal(err)
	}
	var baseline crmcontracts.Company360Suggestion
	for _, language := range []string{"en", "de", "vi"} {
		integration.SetBaseLanguage(e.Admin(), t, e.Pool, language)
		page := pageOf(t, e, quietAccount{company: ids.From[ids.CompanyKind](company), now: now})
		if page.Suggestions == nil {
			t.Fatal("suggestions were omitted")
		}
		found := false
		for _, suggestion := range *page.Suggestions {
			if suggestion.Kind != crmcontracts.Company360SuggestionKindNoReply {
				continue
			}
			found = true
			if language == "en" {
				baseline = suggestion
				continue
			}
			if suggestion.Reason == baseline.Reason || !strings.Contains(suggestion.Reason, "10") {
				t.Fatalf("%s did not localize the measured silence: %s", language, suggestion.Reason)
			}
			if suggestion.Fingerprint != baseline.Fingerprint {
				t.Fatal("language changed the suggestion identity")
			}
			if len(suggestion.Evidence) != 1 || len(baseline.Evidence) != 1 {
				t.Fatal("the unanswered message must remain the sole source")
			}
			cited, original := suggestion.Evidence[0], baseline.Evidence[0]
			if cited.Origin == nil || original.Origin == nil || *cited.Origin == *original.Origin {
				t.Fatalf("%s did not localize the source caption", language)
			}
			cited.Origin = original.Origin
			if !reflect.DeepEqual(cited, original) {
				t.Fatal("language changed the source identity, timestamp or original words")
			}
		}
		if !found {
			t.Fatalf("%s omitted the unanswered message", language)
		}
	}
}
