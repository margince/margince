// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strconv"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/extraction"
)

// stateSeeders make the records the sweep's calls need in a given state: a
// sent offer, a paused room, a promoted lead. Each is reached through the
// app's own routes from a fresh record, so a call finds what a user would.
func stateSeeders(s sweepSeed) map[string]func() string {
	seeders := map[string]func() string{}
	for _, group := range []func(sweepSeed) map[string]func() string{
		peopleSeeders, accessSeeders, salesSeeders, leadSeeders, roomSeeders, workSeeders, meetingSeeders, reportSeeders, companySeeders, weeklySeeders,
	} {
		for kind, seed := range group(s) {
			seeders[kind] = seed
		}
	}
	return seeders
}

// pinned is the If-Match header for the version a record was read at.
func pinned(version int) map[string]string {
	return map[string]string{"If-Match": strconv.Itoa(version)}
}

// salesSeeders make offers past draft and deals past open.
func salesSeeders(s sweepSeed) map[string]func() string {
	priced := func() string {
		return s.post("/v1/deals/"+s.create["deal"]()+"/offers", AnyMap{
			"currency": "EUR", "source": "manual",
			"line_items": []AnyMap{{"description": "Retainer", "quantity": 1, "unit_price_minor": 500000, "tax_rate": 19.0}},
		})
	}
	return map[string]func() string{
		"priced_offer": priced,
		"sent_offer": func() string {
			id := priced()
			s.call("POST", "/v1/offers/"+id+"/send", nil, nil, nil)
			return id
		},
		"rendered_offer": func() string {
			id := priced()
			s.call("POST", "/v1/offers/"+id+"/render", AnyMap{}, nil, nil)
			return id
		},
		"lost_deal": func() string {
			id := s.create["deal"]()
			stages := apptest.DiscoverSeededPipeline(s.t, s.e)
			s.call("POST", "/v1/deals/"+id+"/advance", AnyMap{"to_stage_id": stages.Lost, "lost_reason": "Role sweep"}, nil, nil)
			var deal struct {
				ClosingOccurrenceID string `json:"closing_occurrence_id"`
			}
			s.call("GET", "/v1/deals/"+id, nil, nil, &deal)
			s.pool.note(id, "closing_occurrence_id", deal.ClosingOccurrenceID)
			return id
		},
		"open_stage": func() string {
			var pipelines struct {
				Data []struct {
					Stages []struct {
						ID       string `json:"id"`
						Semantic string `json:"semantic"`
					} `json:"stages"`
				} `json:"data"`
			}
			s.call("GET", "/v1/pipelines", nil, nil, &pipelines)
			first := apptest.DiscoverSeededPipeline(s.t, s.e).Open
			for _, pipeline := range pipelines.Data {
				for _, stage := range pipeline.Stages {
					if stage.Semantic == "open" && stage.ID != first {
						return stage.ID
					}
				}
			}
			s.t.Fatal("the seeded pipeline has one open stage, so no deal can move between two")
			return ""
		},
		// Only an automatic move can be reverted or accepted, and only a pass
		// over a deal's exit criteria makes one; the seed records its outcome.
		"auto_moved_deal": func() string {
			id, stages := s.create["deal"](), apptest.DiscoverSeededPipeline(s.t, s.e)
			next := s.create["open_stage"]()
			s.call("POST", "/v1/deals/"+id+"/advance", AnyMap{"to_stage_id": next}, nil, nil)
			move := ids.NewV7().String()
			s.ownerExec(`INSERT INTO stage_progression_outcome (approval_id, deal_id, pipeline_id, from_stage_id, to_stage_id,
				evidence_kinds, outcome, decided_by_system) VALUES ($1, $2, $3, $4, $5, '{}', 'auto_applied', true)`,
				move, id, stages.PipelineID, stages.Open, next)
			var deal struct {
				Version int `json:"version"`
			}
			s.call("GET", "/v1/deals/"+id, nil, nil, &deal)
			s.pool.note(id, "move", move)
			s.pool.note(id, "version", strconv.Itoa(deal.Version))
			return id
		},
		"legal_review": func() string {
			return s.post("/v1/deals/"+s.create["deal"]()+"/legal-reviews", AnyMap{})
		},
		"archived_tag": func() string {
			id := s.create["tag"]()
			s.call("DELETE", "/v1/tags/"+id, nil, nil, nil)
			return id
		},
	}
}

// leadSeeders make leads promoted, disqualified, referred by a partner or
// matching a company by its domain.
func leadSeeders(s sweepSeed) map[string]func() string {
	lead := func(body AnyMap) string {
		body["full_name"], body["source"] = "Role Sweep", "manual"
		if body["email"] == nil {
			body["email"] = uniqueEmail()
		}
		return s.post("/v1/leads", body)
	}
	versionOfLead := func(id string) int {
		var read struct {
			Version int `json:"version"`
		}
		s.call("GET", "/v1/leads/"+id, nil, nil, &read)
		return read.Version
	}
	return map[string]func() string{
		"promoted_lead": func() string {
			id := lead(AnyMap{})
			s.call("POST", "/v1/leads/"+id+"/promote", AnyMap{"trigger": "human_qualify"}, pinned(versionOfLead(id)), nil)
			return id
		},
		"disqualified_lead": func() string {
			id := lead(AnyMap{})
			s.call("DELETE", "/v1/leads/"+id, AnyMap{}, pinned(versionOfLead(id)), nil)
			return id
		},
		"partner_lead": func() string {
			partner := s.create["company"]()
			s.call("PUT", "/v1/companies/"+partner+"/partner", AnyMap{"partner_role": "hosting"}, nil, nil)
			return lead(AnyMap{"partner_company_id": partner})
		},
		"matched_lead": func() string {
			domain := "sweep-" + ids.NewV7().String()[24:] + ".test"
			company := s.post("/v1/companies", AnyMap{
				"display_name": "Sweep " + domain, "source": "manual", "domains": []AnyMap{{"domain": domain, "is_primary": true}},
			})
			id := lead(AnyMap{"email": "buyer@" + domain, "company_name": "Sweep " + domain})
			s.pool.note(id, "company", company)
			return id
		},
		"domain_company": func() string {
			domain := "sweep-" + ids.NewV7().String()[24:] + ".test"
			return s.post("/v1/companies", AnyMap{
				"display_name": "Sweep " + domain, "source": "manual", "domains": []AnyMap{{"domain": domain, "is_primary": true}},
			})
		},
		"linkable_contact": func() string {
			domain := "sweep-" + ids.NewV7().String()[24:] + ".test"
			contact := s.post("/v1/contacts", AnyMap{
				"full_name": "Role Sweep", "source": "manual",
				"emails": []AnyMap{{"email": "buyer@" + domain, "email_type": "work", "is_primary": true}},
			})
			company := s.post("/v1/companies", AnyMap{
				"display_name": "Sweep " + domain, "source": "manual", "domains": []AnyMap{{"domain": domain, "is_primary": true}},
			})
			s.pool.note(contact, "company", company)
			return contact
		},
	}
}

// roomSeeders make a deal room's documents, participants and threads, and a
// paused room, each on a room of its own.
func roomSeeders(s sweepSeed) map[string]func() string {
	room := func() (string, string) {
		deal := s.create["deal"]()
		return s.post("/v1/deal-rooms", AnyMap{"deal_id": deal, "title": "Role Sweep", "source": "manual"}), deal
	}
	dealFile := func(deal string) string {
		status, raw := uploadAs(s.t, s.e, nil, "deal", deal, []byte("%PDF-1.4 role sweep"))
		if status != http.StatusCreated {
			s.t.Fatalf("seeding a deal's file → %d %s", status, raw)
		}
		return attachmentIDOf(s.t, raw)
	}
	inRoom := func(path string, body AnyMap) string {
		id, _ := room()
		var created struct {
			ID          string `json:"id"`
			Participant struct {
				ID string `json:"id"`
			} `json:"participant"`
		}
		s.call("POST", "/v1/deal-rooms/"+id+path, body, nil, &created)
		child := created.ID + created.Participant.ID
		s.pool.note(child, "room", id)
		return child
	}
	return map[string]func() string{
		"room_attachment": func() string {
			id, deal := room()
			file := dealFile(deal)
			s.pool.note(file, "room", id)
			return file
		},
		"room_document": func() string {
			id, deal := room()
			var created struct {
				ID string `json:"id"`
			}
			s.call("POST", "/v1/deal-rooms/"+id+"/documents", AnyMap{"attachment_id": dealFile(deal), "group_key": "commercial", "source": "manual"}, nil, &created)
			s.pool.note(created.ID, "room", id)
			return created.ID
		},
		"room_participant": func() string {
			return inRoom("/participants", AnyMap{"full_name": "Role Sweep", "email": uniqueEmail(), "capability": "comment", "source": "manual"})
		},
		"room_thread": func() string { return inRoom("/threads", AnyMap{"body": "Role sweep", "source": "manual"}) },
		"paused_room": func() string {
			id, _ := room()
			s.call("POST", "/v1/deal-rooms/"+id+"/pause", AnyMap{}, nil, nil)
			return id
		},
		"read_attachment": func() string {
			file := dealFile(s.create["deal"]())
			s.pool.note(file, "reading", seedExtractionReadingHTTP(s.e, s.t, file, []extraction.ExtractedField{{
				Field: "amount_minor", Value: "150000", SourceQuote: "Total 1,500.00 EUR", PageOrSection: "p.1", Confidence: "high",
			}, {
				Field: "currency", Value: "EUR", SourceQuote: "Total 1,500.00 EUR", PageOrSection: "p.1", Confidence: "high",
			}}))
			return file
		},
		"deal_attachment": func() string {
			deal := s.create["deal"]()
			file := dealFile(deal)
			s.pool.note(file, "deal", deal)
			return file
		},
	}
}
