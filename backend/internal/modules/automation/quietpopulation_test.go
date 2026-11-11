// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestQuietReminderSetupRequiresAnExplicitPopulationAndPeriod(t *testing.T) {
	for _, tc := range []struct{ key, days, population string }{
		{noActivityReminderName, "no_activity_days", "pipeline_id"},
		{checkInCadenceName, "check_in_days", "customer_list_id"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			entry, ok := CatalogEntryByKey(tc.key)
			if !ok {
				t.Fatal("missing quiet reminder")
			}
			if err := entry.Validate(map[string]any{tc.days: float64(7)}); err == nil {
				t.Fatal("saved a reminder with no selected population")
			}
			if err := entry.Validate(map[string]any{tc.population: ids.NewV7().String()}); err == nil {
				t.Fatal("saved a reminder with no quiet period")
			}
			if err := entry.Validate(map[string]any{tc.days: float64(7), tc.population: ids.NewV7().String()}); err != nil {
				t.Fatal(err)
			}
			if err := entry.Validate(map[string]any{tc.days: float64(7), tc.population: "not-a-record"}); err == nil {
				t.Fatal("saved a malformed population identity")
			}
		})
	}
}

func TestRenamingALegacyQuietRuleDoesNotConvertItsPopulation(t *testing.T) {
	before := Automation{Key: noActivityReminderName, Enabled: true, Params: json.RawMessage(`{"no_activity_days":7}`)}
	err := NewAutomationStore(nil).revalidate(context.Background(), before, UpdateAutomationInput{
		Name: new("Renamed reminder"), Params: map[string]any{"no_activity_days": float64(7)},
	})
	if err != nil {
		t.Fatalf("a rename forced a legacy scope conversion: %v", err)
	}
}

type quietPreviewReader struct {
	result     QuietCandidates
	population QuietPopulation
	cutoff     time.Time
}

func (r *quietPreviewReader) QuietCandidates(_ context.Context, population QuietPopulation, cutoff time.Time, _ int) (QuietCandidates, error) {
	r.population, r.cutoff = population, cutoff
	return r.result, nil
}

func (*quietPreviewReader) ValidateQuietPopulation(context.Context, QuietPopulation) error {
	return nil
}

func TestQuietPreviewUsesTheSameScopedCandidatesWithoutAHistoricalEstimate(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	pipeline, record := ids.NewV7(), ids.NewV7()
	reader := &quietPreviewReader{result: QuietCandidates{Total: 21, Records: []EntityAnchor{{Ref: datasource.EntityRef{Type: datasource.EntityDeal, ID: record}}}}}
	store := NewAutomationStore(nil).SetClock(func() time.Time { return now }).WithQuietPopulationScan(reader)
	params, err := json.Marshal(map[string]any{"pipeline_id": pipeline, "no_activity_days": 7})
	if err != nil {
		t.Fatal(err)
	}
	result, handled, err := store.quietPreview(context.Background(), Automation{Key: noActivityReminderName, Params: params}, AutomationPreviewInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !handled || !result.MatchesNowAvailable || result.MatchesNow != 21 || len(result.Sample) != 1 || result.Sample[0] != record {
		t.Fatalf("quiet preview=%+v handled=%v", result, handled)
	}
	if result.HistoricalPreviewAvailable || result.WouldHaveFired != nil {
		t.Fatalf("invented a historical preview: %+v", result)
	}
	if reader.population.PipelineID != pipeline || !reader.cutoff.Equal(now.AddDate(0, 0, -7)) {
		t.Fatalf("wrong scope/cutoff: %+v %s", reader.population, reader.cutoff)
	}
}

func TestReviewedCustomerRuleKeepsAnExistingQuietTaskIdentity(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	anchor := now.AddDate(0, 0, -40)
	payload, err := json.Marshal(touchAnchorPayload{LastActivityAt: anchor})
	if err != nil {
		t.Fatal(err)
	}
	ev := workflow.Event{OccurredAt: now, Payload: payload, Entity: datasource.EntityRef{Type: datasource.EntityContact, ID: ids.NewV7()}}
	var previous struct {
		System string `json:"source_system"`
		ID     string `json:"source_id"`
	}
	for _, h := range []workflow.Handler{noActivityReminder{}, checkInCadence{}} {
		ev.Params = json.RawMessage(`{}`)
		if h.Spec().Name == checkInCadenceName {
			ev.Params = json.RawMessage(`{"customer_list_id":"00000000-0000-4000-8000-000000000001"}`)
		}
		effect, err := anchorReminderTaskEffect(context.Background(), Executors{}, ev, "Keep in touch", h)
		if err != nil {
			t.Fatal(err)
		}
		var key struct {
			System string `json:"source_system"`
			ID     string `json:"source_id"`
		}
		if err := json.Unmarshal(effect.Actions[0].Args, &key); err != nil {
			t.Fatal(err)
		}
		if key.System != provenance.NoActivityReminderSource || key.ID == "" {
			t.Fatalf("missing canonical quiet identity: %+v", key)
		}
		if previous.ID != "" && previous != key {
			t.Fatalf("quiet handlers planned separate tasks: %+v and %+v", previous, key)
		}
		previous = key
	}
}

func TestRenewalSetupChecksTheCurrentDateFieldBeforeSaving(t *testing.T) {
	store := NewAutomationStore(nil).WithFieldCatalog(fakeFieldCatalog{columns: map[string][]fieldcatalog.Column{
		"contact": {{Name: "cf_renewal", Type: fieldcatalog.TypeDate}, {Name: "cf_words", Type: fieldcatalog.TypeText}},
	}})
	entry, ok := CatalogEntryByKey(renewalReminderName)
	if !ok {
		t.Fatal("missing renewal reminder")
	}
	for _, field := range []string{"cf_renewal", "cf_words", "cf_retired", ""} {
		err := store.validateRefs(context.Background(), entry, map[string]any{"object": "contact", "date_field": field})
		if (err == nil) != (field == "cf_renewal") {
			t.Fatalf("field %q: %v", field, err)
		}
	}
}

func TestScheduledPopulationChangesNeedAnInitialRunConfirmation(t *testing.T) {
	on, off := true, false
	for _, key := range []string{noActivityReminderName, checkInCadenceName, renewalReminderName, stageChangeCreateTaskName} {
		for _, tc := range []struct {
			name              string
			wasEnabled        bool
			in                UpdateAutomationInput
			needsConfirmation bool
		}{
			{"enable", false, UpdateAutomationInput{Enabled: &on}, true},
			{"change active population", true, UpdateAutomationInput{Params: map[string]any{"days": 9}}, true},
			{"save paused population", false, UpdateAutomationInput{Params: map[string]any{"days": 9}}, false},
			{"pause and edit", true, UpdateAutomationInput{Enabled: &off, Params: map[string]any{"days": 9}}, false},
			{"rename", true, UpdateAutomationInput{Name: new("Renamed")}, false},
			{"unchanged params", true, UpdateAutomationInput{Params: map[string]any{"days": float64(7)}}, false},
		} {
			t.Run(key+"/"+tc.name, func(t *testing.T) {
				before := Automation{Key: key, Enabled: tc.wasEnabled, Params: json.RawMessage(`{"days":7}`)}
				err := confirmScheduledPopulation(before, tc.in)
				wantRefusal := tc.needsConfirmation && key != stageChangeCreateTaskName
				if (err != nil) != wantRefusal {
					t.Fatalf("confirmation refusal=%v, want %v", err, wantRefusal)
				}
				tc.in.InitialRunConfirmed = true
				if err := confirmScheduledPopulation(before, tc.in); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
