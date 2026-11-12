// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// setupFixtures fill the calls that set the workspace up and a user's own
// settings: pipelines and stages, rules, signals, rates and the voice profile.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
var setupFixtures = map[string]func(p *recordPool) map[string]any{
	"archive_signal": byID("signal"),
	"resolve_signal": byID("unresolved_signal"),
	"update_signal": func(p *recordPool) map[string]any {
		signal := p.take("signal")
		return map[string]any{"id": signal, "if_version": versionOf(p, signal), "request_body": map[string]any{"status": "acknowledged"}}
	},
	"import_vcards": fileOf("cards.vcf", "text/vcard", "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Role Sweep\r\nEMAIL:sweep@cards.test\r\nEND:VCARD\r\n"),
	"import_linkedin_connections": fileOf("Connections.csv", "text/csv",
		"First Name,Last Name,URL,Email Address,Company,Position,Connected On\nRole,Sweep,https://www.linkedin.com/in/role-sweep,,Sweep Co,Buyer,01 Jan 2026\n"),
	"upload_corpus_document": func(p *recordPool) map[string]any {
		args := fileOf("notes.txt", "text/plain", "Role sweep notes about the offer.")(p)
		args["id"] = p.take("corpus")
		return args
	},
	"upload_company_logo":      fileOf("logo.png", "image/png", string(sweepPNG())),
	"upload_company_logo_icon": fileOf("icon.png", "image/png", string(sweepPNG())),
	"upload_legal_entity_logo": func(p *recordPool) map[string]any {
		args := fileOf("logo.png", "image/png", string(sweepPNG()))(p)
		args["id"] = p.take("legal_entity")
		return args
	},
	"save_my_timezone": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"timezone": "Europe/Berlin", "source": "agent"}}
	},
	"save_my_working_hours": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"start_time": "09:00", "end_time": "17:00", "days": []any{1, 2, 3, 4, 5}}}
	},
	"create_voice_profile": func(p *recordPool) map[string]any {
		p.take("no_voice_profile")
		return map[string]any{"request_body": map[string]any{"personality_md": "Short sentences."}}
	},
	"update_voice_profile": func(p *recordPool) map[string]any {
		profile := p.take("voice_profile")
		return map[string]any{"id": profile, "request_body": map[string]any{"personality_md": "Short sentences."}}
	},
	"update_voice_source": func(p *recordPool) map[string]any {
		source := p.take("voice_source")
		return map[string]any{"id": p.fact(source, "profile"), "sourceId": source, "request_body": map[string]any{"included": true}}
	},
	"preview_voice_source": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("voice_profile"), "request_body": map[string]any{"format": "text", "content": "I keep my notes short."}}
	},
	"build_voice_profile": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("voice_ready"), "request_body": map[string]any{"reason": "manual"}}
	},
	"get_voice_profile":                  byID("voice_profile"),
	"list_voice_corpus_sources":          byID("voice_profile"),
	"get_voice_learning_summary":         byID("voice_profile"),
	"list_voice_profile_deltas":          byID("voice_profile"),
	"list_voice_profile_versions":        byID("voice_profile"),
	"archive_automation":                 byID("automation"),
	"preview_website_inquiry_assignment": func(p *recordPool) map[string]any { return inquiryAssignmentArgs(p, false) },
	"assign_website_inquiry":             func(p *recordPool) map[string]any { return inquiryAssignmentArgs(p, true) },
	"update_automation": func(p *recordPool) map[string]any {
		return versioned(p, "automation", map[string]any{"name": "Sweep rule"})
	},
	"preview_automation": func(p *recordPool) map[string]any { return map[string]any{"id": p.take("automation")} },
	"handle_website_inquiries": func(p *recordPool) map[string]any {
		form := p.take("existing_website_inquiry")
		source := p.fact(form, "source")
		return map[string]any{"id": form, "request_body": map[string]any{"automation_id": p.fact(form, "policy"), "automation_version": versionOf(p, form), "selected": []map[string]any{{"id": source, "version": versionOf(p, source)}}}}
	},
	"preview_website_inquiries": func(p *recordPool) map[string]any {
		form := p.take("versioned_web_form")
		return map[string]any{"id": form, "request_body": map[string]any{"policy": map[string]any{"form_id": form, "mode": "coordinator", "routing": map[string]any{}, "coordinator_id": p.take("user"), "task_title": "Review the website inquiry", "due_in_days": 1, "timezone": "Etc/UTC"}, "selected": []any{}}}
	},
	"create_automation": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"key": "assign_lead_owner", "name": "Sweep rule", "params": map[string]any{}}}
	},
	"archive_pipeline": byID("pipeline"),
	"restore_pipeline": byID("archived_pipeline"),
	"update_pipeline": func(p *recordPool) map[string]any {
		return unversioned(p, "pipeline", map[string]any{"name": uniqueName()})
	},
	"reorder_pipelines": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"pipeline_ids": idList(p, "pipeline_order")}}
	},
	"archive_stage": byID("stage"),
	"update_stage":  func(p *recordPool) map[string]any { return versioned(p, "stage", map[string]any{"name": uniqueName()}) },
	"reorder_stages": func(p *recordPool) map[string]any {
		pipeline := p.take("pipeline")
		return map[string]any{"id": pipeline, "request_body": map[string]any{"stage_ids": strings.Split(p.fact(pipeline, "stages"), ",")}}
	},
	"create_stage": func(p *recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"pipeline_id": p.take("pipeline"), "name": uniqueName(), "position": 3, "semantic": "open"}}
	},
	"create_exit_criterion": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("stage"), "request_body": map[string]any{"key": "sweep_" + strings.ReplaceAll(ids.NewV7().String(), "-", ""), "label": "Sweep", "kind": "custom"}}
	},
	"archive_exit_criterion": func(p *recordPool) map[string]any {
		criterion := p.take("exit_criterion")
		return map[string]any{"id": p.fact(criterion, "stage"), "criterion_id": criterion}
	},
	"update_exit_criterion": func(p *recordPool) map[string]any {
		criterion := p.take("exit_criterion")
		return map[string]any{"id": p.fact(criterion, "stage"), "criterion_id": criterion, "request_body": map[string]any{"label": "Sweep again"}}
	},
	"update_partner_stage": func(p *recordPool) map[string]any {
		return unversioned(p, "partner_stage", map[string]any{"label": uniqueName()})
	},
	"upsert_partner": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("live_partner"), "request_body": map[string]any{"partner_kinds": []any{"referral"}}}
	},
	"update_web_form": func(p *recordPool) map[string]any {
		return versioned(p, "versioned_web_form", map[string]any{"name": uniqueName()})
	},
	"put_product_translation": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("product"), "language": "de", "request_body": map[string]any{"name": "Rollen-Sweep"}}
	},
	"list_product_translations": byID("product"),
	// Each rate starts on its own day, as a second rate for one day is refused.
	"record_tax_rate": func(p *recordPool) map[string]any {
		day := p.sweepStart.AddDate(0, 0, int(taxRateDays.Add(1)))
		return map[string]any{"request_body": map[string]any{"country": "de", "category": "reduced", "rate": "7", "valid_from": day.Format("2006-01-02")}}
	},
	// Each rate is a new one, so every role's call takes the same path.
	"set_fx_rate": func(p *recordPool) map[string]any {
		day := p.sweepStart.AddDate(0, 0, -int(taxRateDays.Add(1)))
		return map[string]any{"request_body": map[string]any{"from_currency": "USD", "rate": "0.9", "effective_date": day.Format("2006-01-02")}}
	},
	"set_ai_model_rate": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{
			"provider": "anthropic", "model_id": "sweep-" + ids.NewV7().String(), "input_per_mtok": "1", "output_per_mtok": "2",
			"cache_read_per_mtok": "0.1", "cache_write_per_mtok": "1.25",
		}}
	},
	"disconnect_provider": func(p *recordPool) map[string]any { return map[string]any{"provider": p.take("connected_provider")} },
	"enrich_contact_from_provider": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("enrichable_contact"), "request_body": map[string]any{"provider": "surfe"}}
	},
	"save_my_onboarding_state": func(p *recordPool) map[string]any {
		state := p.take("onboarding_state")
		return map[string]any{"request_body": map[string]any{
			"expected_version": versionOf(p, state), "step": "voice", "source_mode": "manual", "company_draft": map[string]any{},
		}}
	},
	"read_onboarding_state": func(p *recordPool) map[string]any {
		p.take("onboarding_state")
		return map[string]any{}
	},
	"preview_ai_budget": func(p *recordPool) map[string]any {
		budget := p.take("ai_budget")
		return map[string]any{"request_body": map[string]any{
			"config": map[string]any{"tokens_per_full_user": 1000000, "company_monthly_tokens": nil}, "expected_revision": p.fact(budget, "revision"),
		}}
	},
	"get_ai_call":          byID("ai_call"),
	"resend_member_invite": byID("invited_user"),
	"rename_custom_field": func(p *recordPool) map[string]any {
		return versioned(p, "custom_field", map[string]any{"label": uniqueName()})
	},
	"retire_custom_field": byID("custom_field"),
	// A schema change is refused while its table is busy, so the fields are
	// added to contracts, the table the other roles' calls write least.
	"create_custom_field": func(*recordPool) map[string]any {
		return map[string]any{"request_body": map[string]any{"object": "contract", "label": uniqueName(), "type": "text", "source": "manual"}}
	},
	"update_custom_field_options": func(p *recordPool) map[string]any {
		return unversioned(p, "picklist_field", map[string]any{"options": []any{"A", "B", "C"}})
	},
}

// taxRateDays numbers the days the sweep's tax and exchange rates start on.
var taxRateDays atomic.Int32

// fileOf is the arguments of an upload that sends body as a file.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func fileOf(name, contentType, body string) func(*recordPool) map[string]any {
	return func(*recordPool) map[string]any {
		return map[string]any{"filename": name, "content_type": contentType, "content_base64": base64.StdEncoding.EncodeToString([]byte(body))}
	}
}

// versioned is a change to the record of kind at the version its seed read.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func versioned(p *recordPool, kind string, body map[string]any) map[string]any {
	id := p.take(kind)
	return map[string]any{"id": id, "if_version": versionOf(p, id), "request_body": body}
}

// unversioned is a change to the record of kind, for a route that takes no version.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func unversioned(p *recordPool, kind string, body map[string]any) map[string]any {
	return map[string]any{"id": p.take(kind), "request_body": body}
}

func versionOf(p *recordPool, id string) int {
	version, _ := strconv.Atoi(p.fact(id, "version"))
	return version
}

func idList(p *recordPool, kind string) []string {
	return strings.Split(p.fact(p.take(kind), "ids"), ",")
}

// connectSurfe connects the offline enrichment provider, as an administrator does.
func connectSurfe(s sweepSeed) {
	s.call("PUT", "/v1/provider-connections/surfe", integration.AnyMap{"api_key": "role-sweep"}, nil, nil)
}

// sweepPNG is a one-pixel PNG.
func sweepPNG() []byte {
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	return png
}

// setupSeeders make the records the setup calls act on.
func setupSeeders(s sweepSeed) map[string]func() string {
	versioned := func(path string, body integration.AnyMap) string {
		var row struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		}
		s.call("POST", path, body, nil, &row)
		s.pool.note(row.ID, "version", strconv.Itoa(row.Version))
		return row.ID
	}
	pipeline := func() string {
		id := versioned("/v1/pipelines", integration.AnyMap{"name": uniqueName(), "stages": []integration.AnyMap{
			{"name": "Talking", "position": 1, "semantic": "open"}, {"name": "Agreeing", "position": 2, "semantic": "open"},
		}})
		var read struct {
			Stages []struct {
				ID string `json:"id"`
			} `json:"stages"`
		}
		s.call("GET", "/v1/pipelines/"+id, nil, nil, &read)
		stages := make([]string, len(read.Stages))
		for i, stage := range read.Stages {
			stages[i] = stage.ID
		}
		s.pool.note(id, "stages", strings.Join(stages, ","))
		return id
	}
	stage := func() string {
		owner := pipeline()
		id, _, _ := strings.Cut(s.pool.fact(owner, "stages"), ",")
		var read struct {
			Version int `json:"version"`
		}
		s.call("GET", "/v1/stages/"+id, nil, nil, &read)
		s.pool.note(id, "version", strconv.Itoa(read.Version))
		return id
	}
	return map[string]func() string{
		"signal": func() string {
			return versioned("/v1/signals", integration.AnyMap{
				"kind": "reengagement", "summary": "They wrote back", "source": "manual", "entity_type": "company", "entity_id": s.create["company"](),
			})
		},
		"unresolved_signal": func() string {
			return versioned("/v1/signals", integration.AnyMap{"kind": "reengagement", "summary": "They wrote back", "source": "manual", "raw_ref": "Sweep Co"})
		},
		// An agent buys contact details only under the daily run limit an
		// administrator set on the connection.
		"enrichable_contact": func() string {
			connectSurfe(s)
			s.call("PATCH", "/v1/provider-connections/surfe", integration.AnyMap{"configuration": integration.AnyMap{"daily_run_limit": 1000}}, nil, nil)
			return consentedContact(s)
		},
		"connected_provider": func() string {
			connectSurfe(s)
			return "surfe"
		},
		// The wizard's state exists once a user saved a step of it.
		"onboarding_state": func() string {
			var state struct {
				Version int `json:"version"`
			}
			if s.e.Call(s.t, "GET", "/v1/onboarding/state", nil, nil, &state) == http.StatusNotFound {
				s.call("PUT", "/v1/onboarding/state", integration.AnyMap{"expected_version": 0, "step": "voice", "source_mode": "manual", "company_draft": integration.AnyMap{}}, nil, &state)
			}
			id := ids.NewV7().String()
			s.pool.note(id, "version", strconv.Itoa(state.Version))
			return id
		},
		"ai_budget": func() string {
			var budget struct {
				Revision string `json:"revision"`
			}
			s.call("GET", "/v1/ai/budget", nil, nil, &budget)
			id := ids.NewV7().String()
			s.pool.note(id, "revision", budget.Revision)
			return id
		},
		"pipeline": pipeline,
		"archived_pipeline": func() string {
			id := pipeline()
			s.call("DELETE", "/v1/pipelines/"+id, nil, nil, nil)
			return id
		},
		"pipeline_order": func() string {
			var list struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			s.call("GET", "/v1/pipelines", nil, nil, &list)
			order := make([]string, len(list.Data))
			for i, row := range list.Data {
				order[i] = row.ID
			}
			id := ids.NewV7().String()
			s.pool.note(id, "ids", strings.Join(order, ","))
			return id
		},
		"stage": stage,
		"exit_criterion": func() string {
			on := stage()
			id := versioned("/v1/stages/"+on+"/exit-criteria", integration.AnyMap{
				"key": "sweep_" + strings.ReplaceAll(ids.NewV7().String(), "-", ""), "label": "Sweep", "kind": "custom",
			})
			s.pool.note(id, "stage", on)
			return id
		},
		"partner_stage": func() string {
			return versioned("/v1/partner-stages", integration.AnyMap{"label": uniqueName(), "meaning": "candidate"})
		},
		"automation": func() string {
			return versioned("/v1/automations", integration.AnyMap{"key": "assign_lead_owner", "name": uniqueName(), "params": integration.AnyMap{}})
		},
		"existing_website_inquiry":   func() string { return seedExistingWebsiteInquiry(s) },
		"assignable_website_inquiry": func() string { return seedAssignableWebsiteInquiry(s) },
		"versioned_web_form": func() string {
			return versioned("/v1/web-forms", integration.AnyMap{"name": uniqueName(), "sender": "browser", "allowed_origins": []string{"https://example.test"}})
		},
		"product": func() string {
			return s.post("/v1/products", integration.AnyMap{"name": uniqueName(), "unit_price_minor": 1000, "currency": "EUR", "source": "manual"})
		},
		"voice_source": func() string {
			profile := s.create["voice_profile"]()
			var added struct {
				Source struct {
					ID string `json:"id"`
				} `json:"source"`
			}
			s.call("POST", "/v1/voice-profiles/"+profile+"/sources", integration.AnyMap{
				"kind": "other", "source_label": "A note", "register": "general", "source_ref": "sweep-" + ids.NewV7().String(),
				"format": "text", "content": "I keep my notes short and plain, and I say what I mean.",
			}, nil, &added)
			s.pool.note(added.Source.ID, "profile", profile)
			return added.Source.ID
		},
		// A build needs 800 words of the user's own, so the profile holds them.
		"voice_ready": func() string {
			profile := s.create["voice_profile"]()
			s.call("POST", "/v1/voice-profiles/"+profile+"/sources", integration.AnyMap{
				"kind": "document", "register": "long_form", "format": "text", "source_label": "Role sweep",
				"source_ref": "sweep-" + ids.NewV7().String(), "content": strings.Repeat("word ", 800),
			}, nil, nil)
			return profile
		},
		// The user's live profile is archived, so a create makes the one they have.
		"no_voice_profile": func() string {
			profile := s.create["voice_profile"]()
			s.call("DELETE", "/v1/voice-profiles/"+profile, nil, nil, nil)
			return profile
		},
		"picklist_field": func() string {
			return versioned("/v1/custom-fields", integration.AnyMap{"object": "contract", "label": uniqueName(), "type": "picklist", "options": []string{"A", "B"}, "source": "manual"})
		},
		"custom_field": func() string {
			return versioned("/v1/custom-fields", integration.AnyMap{"object": "contract", "label": uniqueName(), "type": "text", "source": "manual"})
		},
		// A model call is written by the model path, which the sweep's calls
		// never reach for every role.
		"ai_call": func() string {
			return s.ownerScalar(`INSERT INTO ai_call (task, tier, provider, model_id, request_fingerprint, logical_call_id)
				VALUES ('cold_start', 'cheap_cloud', 'anthropic', 'sweep-model', 'print', gen_random_uuid()) RETURNING id::text`)
		},
	}
}
