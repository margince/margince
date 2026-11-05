// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedToolRoleRecords creates what these calls will take, through the app's own
// routes, as the admin. The session's user owns them, or the named owner does.
func seedToolRoleRecords(t *testing.T, e *apptest.AppEnv, calls []toolRoleCall, owner string) *recordPool {
	t.Helper()
	counting := countingPool(t)
	for _, call := range calls {
		call.args(counting)
	}
	pool := &recordPool{fresh: map[string][]string{}, facts: map[string]map[string]string{}, sweepStart: counting.sweepStart}
	create := toolRoleSeeders(t, e, owner, pool)
	batched := map[string]bool{}
	for _, batch := range toolRoleBatchSeeders(t, e) {
		wanted := map[string]int{}
		for _, kind := range batch.kinds {
			batched[kind], wanted[kind] = true, counting.wanted[kind]
		}
		if slices.ContainsFunc(batch.kinds, func(kind string) bool { return wanted[kind] > 0 }) {
			maps.Copy(pool.fresh, batch.seed(wanted))
		}
	}
	pool.late = map[string]func() string{}
	for _, kind := range slices.Sorted(maps.Keys(counting.wanted)) {
		n := counting.wanted[kind]
		if batched[kind] {
			continue
		}
		seed, known := create[kind]
		if !known {
			t.Fatalf("a call takes a %s and the sweep has no way to seed one", kind)
		}
		if lateKinds[kind] {
			pool.late[kind] = seed
			continue
		}
		for range n {
			pool.fresh[kind] = append(pool.fresh[kind], seed())
		}
	}
	return pool
}

// lateKinds are seeded as a call takes them. The anchor company's logo is one:
// the sweep's own calls take the logo off before the reads run. So is the
// Telegram bot, which a disconnect takes off before the sends run. So is an
// agent's own act, made as the passport of the call that takes it. So is the
// state of what the installation has one of, which a call made alone takes.
var lateKinds = map[string]bool{
	"anchor_logo": true, "live_channel": true, "channel_conversation": true,
	"agent_withdrawn_owner_identity": true,
	"idle_input_check":               true, "finished_site_read": true, "review_template": true,
	"custom_field": true, "picklist_field": true, "pipeline_order": true,
	"voice_profile": true, "no_voice_profile": true, "onboarding_state": true, "ai_budget": true,
	"voice_build": true, "voice_source": true, "voice_ready": true,
}

// installationKinds are the late kinds that make a call run alone. Each is what
// the installation has one of, or a state only the admin may seed.
var installationKinds = map[string]bool{
	"anchor_logo": true, "live_channel": true, "channel_conversation": true,
	"idle_input_check": true, "finished_site_read": true, "review_template": true,
	"custom_field": true, "picklist_field": true, "pipeline_order": true, "no_voice_profile": true,
	"voice_profile": true, "ai_budget": true, "onboarding_state": true,
	"voice_build": true, "voice_source": true, "voice_ready": true,
}

// toolRoleSeeders makes one record of each kind a call can take. An owner, when
// named, owns the records that carry one. Otherwise the session's user does: a
// role is then measured on its own records, and its grants alone decide. A
// route leaves some records, such as a lead, with no owner unless told. A seed
// notes in the pool what it made beside its record.
func toolRoleSeeders(t *testing.T, e *apptest.AppEnv, owner string, pool *recordPool) map[string]func() string {
	t.Helper()
	seeded := apptest.DefaultPipeline(t, e)
	if owner == "" {
		owner = apptest.SessionUserID(t, e)
	}
	owned := func(body integration.AnyMap) integration.AnyMap {
		body["owner_id"] = owner
		return body
	}
	post := func(path string, body integration.AnyMap) string { return seededID(t, e, path, body) }
	unique := func(prefix string) string { return prefix + " " + ids.NewV7().String() }
	create := map[string]func() string{}
	create["company"] = func() string {
		return post("/v1/companies", owned(integration.AnyMap{"display_name": unique("Role Sweep"), "source": "manual"}))
	}
	create["contact"] = func() string {
		return post("/v1/contacts", owned(integration.AnyMap{"full_name": "Role Sweep", "source": "manual"}))
	}
	create["deal"] = func() string {
		return post("/v1/deals", owned(integration.AnyMap{
			"name": "Role Sweep", "amount_minor": 100, "currency": "EUR", "pipeline_id": seeded.PipelineID,
			"stage_id": seeded.Open, "company_id": create["company"](), "source": "manual",
		}))
	}
	create["lead"] = func() string {
		return post("/v1/leads", owned(integration.AnyMap{"full_name": "Role Sweep", "email": ids.NewV7().String() + "@lead.test", "source": "manual"}))
	}
	create["project"] = func() string {
		return post("/v1/projects", owned(integration.AnyMap{"name": "Role Sweep", "company_id": create["company"](), "source": "manual"}))
	}
	create["activity"] = func() string {
		return post("/v1/activities", integration.AnyMap{"kind": "task", "subject": "Role sweep"})
	}
	create["attachment"] = func() string {
		status, raw := apptest.UploadAs(t, e, nil, "company", create["company"](), []byte("PDF"))
		if status != http.StatusCreated {
			t.Fatalf("seeding an attachment → %d %s", status, raw)
		}
		return apptest.AttachmentIDOf(t, raw)
	}
	maps.Copy(create, configSeeders(post, unique, create))
	maps.Copy(create, stateSeeders(sweepSeed{t: t, e: e, pool: pool, create: create}))
	create["voice_profile"] = func() string { return liveVoiceProfile(t, e) }
	create["commitment"] = func() string {
		if status := e.Call(t, "POST", "/v1/weekly-plans/current", integration.AnyMap{}, nil, nil); status >= http.StatusBadRequest && status != http.StatusConflict {
			t.Fatalf("starting the week's plan → %d", status)
		}
		return post("/v1/weekly-plans/commitments", integration.AnyMap{"label": unique("Sweep")})
	}
	return create
}

// configSeeders make the workspace's own records: teams, vocabulary, settings
// rows and the sales documents that hang off a deal.
func configSeeders(post func(string, integration.AnyMap) string, unique func(string) string, create map[string]func() string) map[string]func() string {
	label := func(path string) func() string {
		return func() string { return post(path, integration.AnyMap{"label": unique("Role Sweep")}) }
	}
	return map[string]func() string{
		"team":                   func() string { return post("/v1/teams", integration.AnyMap{"name": unique("Role Sweep")}) },
		"tag":                    func() string { return post("/v1/tags", integration.AnyMap{"name": unique("Sweep")}) },
		"lead_source":            label("/v1/lead-sources"),
		"lead_origin":            label("/v1/lead-origins"),
		"lead_disqualify_reason": label("/v1/lead-disqualify-reasons"),
		"acquisition_source":     label("/v1/acquisition-sources"),
		"loss_reason":            label("/v1/loss-reasons"),
		"record_role": func() string {
			return post("/v1/record-roles", integration.AnyMap{"label": unique("Sweep"), "record_types": []string{"company"}, "assignee_kinds": []string{"user"}})
		},
		"legal_entity": func() string {
			return post("/v1/legal-entities", integration.AnyMap{"display_name": unique("Sweep"), "legal_name": unique("Sweep GmbH"), "country": "de"})
		},
		"list": func() string {
			return post("/v1/lists", integration.AnyMap{"name": unique("Sweep"), "entity_type": "company"})
		},
		"saved_view": func() string {
			return post("/v1/views", integration.AnyMap{"resource": "companies", "name": unique("Sweep"), "query": integration.AnyMap{"columns": []string{"display_name"}}})
		},
		"web_form": func() string {
			return post("/v1/web-forms", integration.AnyMap{"name": unique("Sweep"), "sender": "browser", "allowed_origins": []string{"https://example.test"}})
		},
		"corpus": func() string {
			return post("/v1/knowledge/corpora", integration.AnyMap{"name": unique("Sweep"), "topic_statement": "Role sweep"})
		},
		"deal_room": func() string {
			return post("/v1/deal-rooms", integration.AnyMap{"deal_id": create["deal"](), "title": "Role Sweep", "source": "manual"})
		},
		"offer": func() string {
			return post("/v1/deals/"+create["deal"]()+"/offers", integration.AnyMap{"currency": "EUR", "source": "manual"})
		},
		"contract": func() string {
			return post("/v1/contracts", integration.AnyMap{"company_id": create["company"](), "title": "Role Sweep", "value_basis": "total"})
		},
		"intro_request": func() string {
			return post("/v1/contacts/"+create["contact"]()+"/intro-requests", integration.AnyMap{
				"introducer_user_id": create["user"](), "route_type": "direct", "internal_reason": "Role sweep",
			})
		},
		"initiative": func() string {
			return post("/v1/analytics/initiatives", integration.AnyMap{"name": unique("Sweep"), "objective_metric": "bookings_won", "deadline": "2099-12-31"})
		},
		"assignment": func() string {
			return post("/v1/records/company/"+create["company"]()+"/assignments", integration.AnyMap{
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
			ID      string `json:"id"`
			OwnerID string `json:"owner_id"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/voice-profiles", nil, nil, &live); status != http.StatusOK {
		t.Fatalf("listing voice profiles → %d", status)
	}
	// A wider seat lists its colleagues' profiles too.
	me := apptest.SessionUserID(t, e)
	for _, profile := range live.Data {
		if profile.OwnerID == me {
			return profile.ID
		}
	}
	return seededID(t, e, "/v1/voice-profiles", integration.AnyMap{})
}

// seededID posts one record as the bootstrap admin and returns its id. A seed
// that fails stops the sweep: the calls it feeds would read as measured.
func seededID(t *testing.T, e *apptest.AppEnv, path string, body integration.AnyMap) string {
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
