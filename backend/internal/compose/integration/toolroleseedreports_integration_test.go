// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reportSeeders make what the reporting, forecast and import reads find. That
// is a saved run, a report, a share, an input check, an import and a document.
func reportSeeders(s sweepSeed) map[string]func() string {
	selection := func() AnyMap {
		return AnyMap{"period": "this_month", "target_basis": "month", "close_window": "all_open", "metrics": []string{"bookings_won"}}
	}
	return map[string]func() string{
		"report_run": func() string {
			var run struct {
				RunID string `json:"run_id"`
			}
			s.call("POST", "/v1/analytics/query", AnyMap{
				"entity": "activities-by-kind", "group_by": []string{"kind"},
				"measures": []AnyMap{{"fn": "count", "as": "n"}}, "save": true,
			}, nil, &run)
			return run.RunID
		},
		"report": func() string {
			return s.post("/v1/analytics/reports", AnyMap{"name": "Sweep " + ids.NewV7().String(), "audience": "private", "selection": selection()})
		},
		"question_report": func() string {
			chosen := selection()
			chosen["layout"] = []AnyMap{{"cards": []AnyMap{{"id": "q1", "kind": "question", "width": "full", "question": AnyMap{
				"entity": "open-deals-per-company", "group_by": []string{"company_id"}, "measures": []AnyMap{{"fn": "count", "as": "deals"}},
			}}}}}
			return s.post("/v1/analytics/reports", AnyMap{"name": "Sweep " + ids.NewV7().String(), "audience": "private", "selection": chosen})
		},
		"forecast_share": func() string {
			var share struct {
				Token string `json:"token"`
			}
			s.call("POST", "/v1/forecast/shares", AnyMap{
				"kind": "snapshot", "target": "someone@example.test", "snapshot_id": freezeAt(s.t, s.e, clocktest.Now(s.t)),
			}, nil, &share)
			return share.Token
		},
		"input_check": func() string {
			finishInputCheck(s.t, s.e)
			return ""
		},
		"forecast_snapshot": func() string { return ownForecastSnapshot(s) },
		"import_run": func() string {
			profile, status := uploadCSV(s.t, s.e, "company", "display_name\nSweep "+ids.NewV7().String()+"\n")
			if status != http.StatusOK {
				s.t.Fatalf("uploading the import's file → %d", status)
			}
			run, status := createRunWithMapping(s.t, s.e, "company", profile.SourceRef, map[string]string{"display_name": "display_name"})
			if status != http.StatusAccepted && status != http.StatusCreated {
				s.t.Fatalf("starting the import → %d", status)
			}
			return run.ID
		},
		"corpus_document": func() string {
			corpus := s.create["corpus"]()
			form, contentType := multipartDocument(s.t, "sweep.md", "text/markdown", []byte("# Role sweep\n"))
			status, raw := postUpload(s.t, s.e, "/v1/knowledge/corpora/"+corpus+"/documents", nil, form, contentType)
			if status != http.StatusAccepted {
				s.t.Fatalf("uploading a corpus document → %d %s", status, raw)
			}
			var document struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(raw), &document); err != nil || document.ID == "" {
				s.t.Fatalf("the corpus document answered %s: %v", raw, err)
			}
			return document.ID
		},
		"automation": func() string {
			var automations struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			s.call("GET", "/v1/automations", nil, nil, &automations)
			if len(automations.Data) == 0 {
				s.t.Fatal("the workspace seeds no automation")
			}
			return automations.Data[0].ID
		},
	}
}

// ownForecastSnapshot freezes the forecast of the admin's own deals, the one
// every role's user may read, as the nightly freeze writes it.
func ownForecastSnapshot(s sweepSeed) string {
	store := forecasting.NewStore(compose.InstallationDB(s.e.Pool))
	owner, at := ids.MustParse(sessionUserID(s.t, s.e)), clocktest.Now(s.t)
	var id ids.UUID
	ctx := adminContext(s.t, s.e)
	if err := apptest.InWorkspace(s.e, s.t, func(tx pgx.Tx) error {
		period, base, err := compose.ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, at)
		if err != nil {
			return err
		}
		id, err = store.TakeSnapshot(ctx, tx, forecasting.NewSnapshot{
			Period: period, Scope: forecasting.Scope{Kind: forecasting.ScopeOwner, ID: &owner},
			Trigger: forecasting.TriggerRecheck, BaseCurrency: base, TakenAt: at,
		})
		return err
	}); err != nil {
		s.t.Fatalf("freezing the admin's forecast: %v", err)
	}
	return id.String()
}

// upload sends a logo image as the multipart file a logo route takes.
func upload(s sweepSeed, path string) {
	form, contentType := multipartDocument(s.t, "logo.png", "image/png", logoPNG(s.t))
	if status, raw := postUpload(s.t, s.e, path, nil, form, contentType); status < 200 || status >= 300 {
		s.t.Fatalf("uploading a logo to %s → %d %s", path, status, raw)
	}
}

// batchSeed makes the records of several kinds together, n of each, because
// all of them must exist before the first is made.
type batchSeed struct {
	kinds []string
	seed  func(wanted map[string]int) map[string][]string
}

// toolRoleBatchSeeders are the kinds seeded in a batch.
func toolRoleBatchSeeders(t *testing.T, e *apptest.AppEnv) []batchSeed {
	t.Helper()
	// One brief runs per user a day and ranks at most a handful of open deals.
	// So a role's items are ranked after its run is cleared and the deals an
	// earlier role's calls left open are closed.
	brief := func(wanted map[string]int) map[string][]string {
		if _, err := e.Owner.Exec(t.Context(), `DELETE FROM brief_run WHERE user_id = $1`, sessionUserID(t, e)); err != nil {
			t.Fatalf("clearing the day's brief: %v", err)
		}
		if _, err := e.Owner.Exec(t.Context(), `UPDATE deal SET archived_at = now() WHERE archived_at IS NULL`); err != nil {
			t.Fatalf("closing an earlier role's deals: %v", err)
		}
		stages := apptest.DiscoverSeededPipeline(t, e)
		deals := map[string][]string{}
		for _, kind := range []string{"brief_item", "snoozed_brief_item"} {
			for range wanted[kind] {
				deals[kind] = append(deals[kind], createDealClosingThisWeek(t, e, stages, "Sweep brief "+ids.NewV7().String()))
			}
		}
		var assembled struct {
			Items []briefItemResponse `json:"items"`
		}
		if status := e.Call(t, "POST", "/v1/brief", AnyMap{}, nil, &assembled); status != http.StatusCreated {
			t.Fatalf("assembling the brief → %d", status)
		}
		items := map[string][]string{}
		for kind, ofKind := range deals {
			for _, deal := range ofKind {
				i := slices.IndexFunc(assembled.Items, func(item briefItemResponse) bool { return item.DealId == deal })
				if i < 0 {
					t.Fatalf("the brief ranks %d items and none for deal %s: %+v", len(assembled.Items), deal, assembled.Items)
				}
				items[kind] = append(items[kind], assembled.Items[i].Id)
			}
		}
		for _, item := range items["snoozed_brief_item"] {
			until := clocktest.Now(t).Add(72 * time.Hour).UTC().Format(time.RFC3339)
			if status := e.Call(t, "POST", "/v1/brief/items/"+item+"/snooze", AnyMap{"snoozed_until": until}, nil, nil); status != http.StatusOK {
				t.Fatalf("snoozing a brief item → %d", status)
			}
		}
		return items
	}
	return []batchSeed{{kinds: []string{"brief_item", "snoozed_brief_item"}, seed: brief}}
}

// companySeeders make what the company and settings reads find: the logos,
// a voice build, a site read, a capture trace and the settings rows.
func companySeeders(s sweepSeed) map[string]func() string {
	return map[string]func() string{
		"loss_reason_key": func() string {
			var reasons struct {
				Data []struct {
					Key string `json:"key"`
				} `json:"data"`
			}
			s.call("GET", "/v1/loss-reasons", nil, nil, &reasons)
			if len(reasons.Data) == 0 {
				s.t.Fatal("the workspace seeds no loss reason")
			}
			return reasons.Data[0].Key
		},
		// Seeded as a call takes it, so as whichever role is being swept; only
		// the admin uploads the company's own logo.
		"anchor_logo": func() string {
			defer s.asRoles("admin")()
			upload(s, "/v1/company/logo")
			upload(s, "/v1/company/logo/icon")
			var company struct {
				CompanyID string `json:"company_id"`
			}
			s.call("GET", "/v1/company", nil, nil, &company)
			return company.CompanyID
		},
		"entity_logo": func() string {
			entity := s.create["legal_entity"]()
			upload(s, "/v1/legal-entities/"+entity+"/logo")
			return entity
		},
		"anchor_name": func() string {
			var company struct {
				DisplayName string `json:"display_name"`
			}
			s.call("GET", "/v1/company", nil, nil, &company)
			return company.DisplayName
		},
		"voice_build": func() string {
			profile := s.create["voice_profile"]()
			s.call("POST", "/v1/voice-profiles/"+profile+"/sources", AnyMap{
				"kind": "document", "register": "long_form", "format": "text", "source_label": "Role sweep",
				"source_ref": "sweep-" + ids.NewV7().String(), "content": strings.Repeat("word ", 800),
			}, nil, nil)
			var build struct {
				ID string `json:"id"`
			}
			s.call("POST", "/v1/voice-profiles/"+profile+"/builds", AnyMap{"reason": "onboarding"}, nil, &build)
			s.pool.note(build.ID, "profile", profile)
			return build.ID
		},
		"site_read": func() string {
			var read struct {
				ID string `json:"id"`
			}
			s.call("POST", "/v1/company/site-reads", AnyMap{"url": "https://example.test/role-sweep"}, nil, &read)
			return read.ID
		},
		// A trace row is written as a mailbox's capture decides a message.
		"capture_trace": func() string {
			return s.ownerScalar(`INSERT INTO capture_trace (user_id, connector, source_system, source_id, stage, outcome, counterparty, subject)
				VALUES ($1, 'gmail', 'gmail', $2, 'tier_ladder', 'captured', 'buyer@role-sweep.test', 'Role sweep') RETURNING id::text`,
				sessionUserID(s.t, s.e), "sweep-"+ids.NewV7().String())
		},
		// A template is edited back to the questions it holds, so a closed
		// deal's review still asks what the review calls answer.
		"review_template": func() string {
			var templates struct {
				Data []struct {
					ID        string          `json:"id"`
					Version   int             `json:"version"`
					Questions json.RawMessage `json:"questions"`
				} `json:"data"`
			}
			s.call("GET", "/v1/activity-review-templates", nil, nil, &templates)
			if len(templates.Data) == 0 {
				s.t.Fatal("the workspace seeds no review template")
			}
			s.pool.note(templates.Data[0].ID, "version", fmt.Sprint(templates.Data[0].Version))
			s.pool.note(templates.Data[0].ID, "questions", string(templates.Data[0].Questions))
			return templates.Data[0].ID
		},
		"profile_field": func() string {
			company := s.create["company"]()
			var field struct {
				ID string `json:"id"`
			}
			s.call("PATCH", "/v1/companies/"+company+"/profile-fields/register_vat", AnyMap{"value": "DE811907980"}, nil, &field)
			s.pool.note(field.ID, "company", company)
			return field.ID
		},
	}
}
