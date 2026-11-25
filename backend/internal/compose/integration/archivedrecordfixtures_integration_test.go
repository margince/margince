// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// One record of each kind TestEveryArchivedRecordAnswersTheSameWay archives,
// made through the same routes a client uses, with an edit its update route
// takes.

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// madeRecord is the part of a create answer the fixtures read.
type madeRecord struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Version int64  `json:"version"`
}

func made(t *testing.T, e *apptest.AppEnv, path string, body AnyMap) madeRecord {
	t.Helper()
	var out madeRecord
	if status := e.Call(t, "POST", path, body, nil, &out); status != http.StatusCreated {
		t.Fatalf("POST %s → %d, want 201", path, status)
	}
	return out
}

func byID(r madeRecord, edit AnyMap) archivedSubject {
	return archivedSubject{params: map[string]string{"id": r.ID}, record: r.ID, version: r.Version, edit: edit}
}

func madeCompany(t *testing.T, e *apptest.AppEnv) madeRecord {
	t.Helper()
	return made(t, e, "/v1/companies", AnyMap{"display_name": "Archive Fixture GmbH", "source": "manual"})
}

func madeContact(t *testing.T, e *apptest.AppEnv) madeRecord {
	t.Helper()
	return made(t, e, "/v1/contacts", AnyMap{"full_name": "Archive Fixture", "source": "manual", "linkedin_url": apptest.ContactProfile()})
}

func madeDeal(t *testing.T, e *apptest.AppEnv) madeRecord {
	t.Helper()
	stages := apptest.DefaultPipeline(t, e)
	return made(t, e, "/v1/deals", AnyMap{"name": "Archive fixture deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual"})
}

func madePipeline(t *testing.T, e *apptest.AppEnv) madeRecord {
	t.Helper()
	return made(t, e, "/v1/pipelines", AnyMap{"name": "Archive fixture pipeline " + ids.NewV7().String()[24:], "stages": []AnyMap{{"name": "Scout", "position": 1}}})
}

func myUserID(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("reading the caller → %d", status)
	}
	return me.User.ID
}

type archivedRecordFixture func(t *testing.T, e *apptest.AppEnv) archivedSubject

var archiveFixtures = map[string]archivedRecordFixture{
	"archiveContact": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		return byID(madeContact(t, e), AnyMap{"full_name": "Edited"})
	},
	"archiveCompany": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		return byID(madeCompany(t, e), AnyMap{"display_name": "Edited GmbH"})
	},
	"archiveDeal": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		return byID(madeDeal(t, e), AnyMap{"name": "Edited"})
	},
	"archiveProject": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		company := madeCompany(t, e)
		return byID(made(t, e, "/v1/projects", AnyMap{"name": "Archive fixture project", "company_id": company.ID, "source": "manual"}), AnyMap{"name": "Edited"})
	},
	"archivePipeline": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		return byID(madePipeline(t, e), AnyMap{"name": "Edited"})
	},
	"archiveStage": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		pipeline := madePipeline(t, e)
		return byID(made(t, e, "/v1/stages", AnyMap{"pipeline_id": pipeline.ID, "name": "Extra", "position": 2}), AnyMap{"name": "Edited"})
	},
	"archiveStageExitCriterion": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		pipeline := madePipeline(t, e)
		stage := made(t, e, "/v1/stages", AnyMap{"pipeline_id": pipeline.ID, "name": "Gated", "position": 2})
		criterion := made(t, e, "/v1/stages/"+stage.ID+"/exit-criteria", AnyMap{"label": "Budget confirmed", "kind": "custom"})
		return archivedSubject{
			params: map[string]string{"id": stage.ID, "criterion_id": criterion.ID},
			record: criterion.ID, version: criterion.Version, edit: AnyMap{"label": "Edited"},
		}
	},
	"archiveActivity": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		return byID(made(t, e, "/v1/activities", AnyMap{"kind": "note", "subject": "Archive fixture note"}), AnyMap{"subject": "Edited"})
	},
	"archiveRecordAssignment": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		role := made(t, e, "/v1/record-roles", AnyMap{"label": "Archive fixture role", "record_types": []string{"company"}, "assignee_kinds": []string{"user"}})
		company := madeCompany(t, e)
		assignment := made(t, e, "/v1/records/company/"+company.ID+"/assignments", AnyMap{"subject_kind": "user", "subject_id": myUserID(t, e), "role_id": role.ID})
		return byID(assignment, AnyMap{"role_id": role.ID})
	},
	"archiveRelationship": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		contact, company := madeContact(t, e), madeCompany(t, e)
		return byID(made(t, e, "/v1/relationships", AnyMap{"kind": "employment", "contact_id": contact.ID, "company_id": company.ID}), AnyMap{"role": "Edited"})
	},
	"archiveList": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		list := made(t, e, "/v1/lists", AnyMap{"name": "Archive fixture list", "entity_type": "contact", "list_type": "static"})
		return byID(list, AnyMap{"name": "Edited", "version": list.Version + 1})
	},
	"archiveReportingReport": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		var catalog struct {
			Metrics []struct {
				ID string `json:"id"`
			} `json:"metrics"`
		}
		if status := e.Call(t, "GET", "/v1/analytics/metrics", nil, nil, &catalog); status != http.StatusOK || len(catalog.Metrics) == 0 {
			t.Fatalf("reading the metric catalog → %d %+v", status, catalog)
		}
		selection := AnyMap{"period": "this_month", "target_basis": "month", "close_window": "all_open", "metrics": []string{catalog.Metrics[0].ID}}
		report := AnyMap{"name": "Archive fixture report", "audience": "private", "selection": selection}
		edited := AnyMap{"name": "Edited", "audience": "private", "selection": report["selection"]}
		subject := byID(made(t, e, "/v1/analytics/reports", report), edited)
		subject.editHeaders = map[string]string{"If-Match": strconv.FormatInt(subject.version+1, 10)}
		return subject
	},
	"archiveSavedView": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		return byID(made(t, e, "/v1/views", AnyMap{"resource": "deals", "name": "Archive fixture view", "query": AnyMap{}}), AnyMap{"name": "Edited"})
	},
	"archiveWebForm": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		return byID(made(t, e, "/v1/web-forms", AnyMap{"name": "Archive fixture form", "sender": "browser", "allowed_origins": []string{"https://site.example"}}), AnyMap{"name": "Edited"})
	},
	"archiveWebhookSubscription": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		var created struct {
			Subscription madeRecord `json:"subscription"`
		}
		if status := e.Call(t, "POST", "/v1/webhook-subscriptions", AnyMap{"target_url": "https://ok.example/hook", "event_types": []string{"deal.created"}}, nil, &created); status != http.StatusCreated {
			t.Fatalf("POST /v1/webhook-subscriptions → %d, want 201", status)
		}
		return byID(created.Subscription, AnyMap{"event_types": []string{"deal.updated"}})
	},
	"archiveTag": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		return byID(made(t, e, "/v1/tags", AnyMap{"name": "Archive fixture tag"}), AnyMap{"description": "edited after archive"})
	},
	"archiveLegalEntity": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		entity := made(t, e, "/v1/legal-entities", AnyMap{"display_name": "Fixture Wien", "legal_name": "Fixture Wien GmbH", "country": "at"})
		return byID(entity, AnyMap{"display_name": "Edited Wien"})
	},
	"archiveServiceMember": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		return byID(made(t, e, "/v1/users/service-members", AnyMap{"extension": "archivefixture", "display_name": "Archive fixture feed"}), nil)
	},
	"archiveRole": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		role := made(t, e, "/v1/roles", AnyMap{"copy_from": "rep", "name": "Archive fixture role"})
		return archivedSubject{params: map[string]string{"key": role.Key}, record: role.Key, version: role.Version, edit: AnyMap{"name": "Edited"}}
	},
	"deleteVoiceProfile": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		return byID(made(t, e, "/v1/voice-profiles", AnyMap{}), AnyMap{"auto_learning_enabled": true})
	},
	"archiveProduct": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		return byID(made(t, e, "/v1/products", AnyMap{"name": "Archive fixture product", "currency": "EUR", "unit_price_minor": 10000}), AnyMap{"name": "Edited"})
	},
	"archiveDealRoom": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		deal := madeDeal(t, e)
		return byID(made(t, e, "/v1/deal-rooms", AnyMap{"deal_id": deal.ID, "title": "Archive fixture room"}), AnyMap{"title": "Edited"})
	},
	"archiveOfferTemplate": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		template := AnyMap{"name": "Archive fixture template", "layout": AnyMap{"logo_url": "https://example.test/logo.png"}}
		return byID(made(t, e, "/v1/offer-templates", template), AnyMap{"name": "Edited", "layout": template["layout"]})
	},
	"archiveOffer": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		deal := madeDeal(t, e)
		return byID(made(t, e, "/v1/deals/"+deal.ID+"/offers", AnyMap{"currency": "EUR"}), AnyMap{"intro_text": "Edited"})
	},
	"archiveSignal": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		company := madeCompany(t, e)
		signal := made(t, e, "/v1/signals", AnyMap{"kind": "reengagement", "summary": "They wrote back", "entity_type": "company", "entity_id": company.ID})
		return byID(signal, AnyMap{"note": "Edited"})
	},
	"archiveContract": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		company := madeCompany(t, e)
		return byID(made(t, e, "/v1/contracts", AnyMap{"company_id": company.ID, "title": "Archive fixture MSA", "value_basis": "total"}), AnyMap{"title": "Edited"})
	},
	"archiveCorpus": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		corpus := made(t, e, "/v1/knowledge/corpora", AnyMap{"name": "Archive fixture corpus", "topic_statement": "How this product is operated."})
		return byID(corpus, AnyMap{"name": "Edited"})
	},
	"deleteAttachment": func(t *testing.T, e *apptest.AppEnv) archivedSubject {
		contact := madeContact(t, e)
		status, raw := apptest.UploadAs(t, e, nil, "contact", contact.ID, []byte("%PDF-1.4 archive fixture"))
		if status != http.StatusCreated {
			t.Fatalf("uploading the attachment → %d: %s", status, raw)
		}
		id := apptest.AttachmentIDOf(t, raw)
		return archivedSubject{params: map[string]string{"id": id}, record: id}
	},
}
