// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedToolRoleRecords creates what these calls will take, through the app's own
// routes, as the admin. The admin owns them, or the named owner does.
func seedToolRoleRecords(t *testing.T, e *apptest.AppEnv, calls []toolRoleCall, owner string) *recordPool {
	t.Helper()
	counting := countingPool(t)
	for _, call := range calls {
		call.args(counting)
	}
	start := counting.sweepStart
	create := toolRoleSeeders(t, e, owner)
	pool := &recordPool{fresh: map[string][]string{}, sweepStart: start}
	for kind, n := range counting.wanted {
		seed, known := create[kind]
		if !known {
			t.Fatalf("a call takes a %s and the sweep has no way to seed one", kind)
		}
		for range n {
			pool.fresh[kind] = append(pool.fresh[kind], seed())
		}
	}
	return pool
}

// toolRoleSeeders makes one record of each kind a call can take. An owner, when
// named, owns the records that carry one; the admin owns them otherwise.
func toolRoleSeeders(t *testing.T, e *apptest.AppEnv, owner string) map[string]func() string {
	t.Helper()
	seeded := apptest.DiscoverSeededPipeline(t, e)
	owned := func(body AnyMap) AnyMap {
		if owner != "" {
			body["owner_id"] = owner
		}
		return body
	}
	post := func(path string, body AnyMap) string { return seededID(t, e, path, body) }
	unique := func(prefix string) string { return prefix + " " + ids.NewV7().String() }
	create := map[string]func() string{}
	create["company"] = func() string {
		return post("/v1/companies", owned(AnyMap{"display_name": unique("Role Sweep"), "source": "manual"}))
	}
	create["contact"] = func() string {
		return post("/v1/contacts", owned(AnyMap{"full_name": "Role Sweep", "source": "manual"}))
	}
	create["deal"] = func() string {
		return post("/v1/deals", owned(AnyMap{
			"name": "Role Sweep", "amount_minor": 100, "currency": "EUR", "pipeline_id": seeded.PipelineID,
			"stage_id": seeded.Open, "company_id": create["company"](), "source": "manual",
		}))
	}
	create["lead"] = func() string {
		return post("/v1/leads", owned(AnyMap{"full_name": "Role Sweep", "email": ids.NewV7().String() + "@lead.test", "source": "manual"}))
	}
	create["project"] = func() string {
		return post("/v1/projects", owned(AnyMap{"name": "Role Sweep", "company_id": create["company"](), "source": "manual"}))
	}
	create["activity"] = func() string { return post("/v1/activities", AnyMap{"kind": "task", "subject": "Role sweep"}) }
	create["attachment"] = func() string {
		status, raw := uploadAs(t, e, nil, "company", create["company"](), []byte("PDF"))
		if status != http.StatusCreated {
			t.Fatalf("seeding an attachment → %d %s", status, raw)
		}
		return attachmentIDOf(t, raw)
	}
	maps.Copy(create, configSeeders(post, unique, create))
	create["voice_profile"] = func() string { return liveVoiceProfile(t, e) }
	create["commitment"] = func() string {
		if status := e.Call(t, "POST", "/v1/weekly-plans/current", AnyMap{}, nil, nil); status >= http.StatusBadRequest && status != http.StatusConflict {
			t.Fatalf("starting the week's plan → %d", status)
		}
		return post("/v1/weekly-plans/commitments", AnyMap{"label": unique("Sweep")})
	}
	return create
}

// configSeeders make the workspace's own records: users, vocabulary, settings
// rows and the sales documents that hang off a deal.
func configSeeders(post func(string, AnyMap) string, unique func(string) string, create map[string]func() string) map[string]func() string {
	label := func(path string) func() string {
		return func() string { return post(path, AnyMap{"label": unique("Role Sweep")}) }
	}
	return map[string]func() string{
		"user": func() string {
			return post("/v1/users", AnyMap{"email": "sweep-" + ids.NewV7().String() + "@role-sweep.test", "display_name": "Role Sweep", "role": "rep"})
		},
		"team":                   func() string { return post("/v1/teams", AnyMap{"name": unique("Role Sweep")}) },
		"tag":                    func() string { return post("/v1/tags", AnyMap{"name": unique("Sweep")}) },
		"lead_source":            label("/v1/lead-sources"),
		"lead_origin":            label("/v1/lead-origins"),
		"lead_disqualify_reason": label("/v1/lead-disqualify-reasons"),
		"acquisition_source":     label("/v1/acquisition-sources"),
		"loss_reason":            label("/v1/loss-reasons"),
		"record_role": func() string {
			return post("/v1/record-roles", AnyMap{"label": unique("Sweep"), "record_types": []string{"company"}, "assignee_kinds": []string{"user"}})
		},
		"legal_entity": func() string {
			return post("/v1/legal-entities", AnyMap{"display_name": unique("Sweep"), "legal_name": unique("Sweep GmbH"), "country": "de"})
		},
		"list": func() string { return post("/v1/lists", AnyMap{"name": unique("Sweep"), "entity_type": "company"}) },
		"saved_view": func() string {
			return post("/v1/views", AnyMap{"resource": "companies", "name": unique("Sweep"), "query": AnyMap{"columns": []string{"display_name"}}})
		},
		"web_form": func() string {
			return post("/v1/web-forms", AnyMap{"name": unique("Sweep"), "sender": "browser", "allowed_origins": []string{"https://example.test"}})
		},
		"corpus": func() string {
			return post("/v1/knowledge/corpora", AnyMap{"name": unique("Sweep"), "topic_statement": "Role sweep"})
		},
		"deal_room": func() string {
			return post("/v1/deal-rooms", AnyMap{"deal_id": create["deal"](), "title": "Role Sweep", "source": "manual"})
		},
		"offer": func() string {
			return post("/v1/deals/"+create["deal"]()+"/offers", AnyMap{"currency": "EUR", "source": "manual"})
		},
		"contract": func() string {
			return post("/v1/contracts", AnyMap{"company_id": create["company"](), "title": "Role Sweep", "value_basis": "total"})
		},
		"intro_request": func() string {
			return post("/v1/contacts/"+create["contact"]()+"/intro-requests", AnyMap{
				"introducer_user_id": create["user"](), "route_type": "direct", "internal_reason": "Role sweep",
			})
		},
		"initiative": func() string {
			return post("/v1/analytics/initiatives", AnyMap{"name": unique("Sweep"), "objective_metric": "bookings_won", "deadline": "2099-12-31"})
		},
		"assignment": func() string {
			return post("/v1/records/company/"+create["company"]()+"/assignments", AnyMap{
				"subject_kind": "user", "subject_id": create["user"](), "role_id": create["record_role"](),
			})
		},
	}
}

// countingPool answers placeholders and counts what each kind is asked for.
func countingPool(t *testing.T) *recordPool {
	t.Helper()
	return &recordPool{wanted: map[string]int{}, sweepStart: clocktest.Now(t).UTC().Truncate(24 * time.Hour).Add(9 * time.Hour)}
}

// liveVoiceProfile is the user's one live voice profile, made on first need.
func liveVoiceProfile(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var live struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/voice-profiles", nil, nil, &live); status != http.StatusOK {
		t.Fatalf("listing voice profiles → %d", status)
	}
	if len(live.Data) > 0 {
		return live.Data[0].ID
	}
	return seededID(t, e, "/v1/voice-profiles", AnyMap{})
}

// seededID posts one record as the bootstrap admin and returns its id. A seed
// that fails stops the sweep: the calls it feeds would read as measured.
func seededID(t *testing.T, e *apptest.AppEnv, path string, body AnyMap) string {
	t.Helper()
	var created json.RawMessage
	status := e.Call(t, "POST", path, body, nil, &created)
	var answer struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created, &answer); err != nil || (status != http.StatusCreated && status != http.StatusOK) || answer.ID == "" {
		t.Fatalf("seeding POST %s → %d %s", path, status, created)
	}
	return answer.ID
}
