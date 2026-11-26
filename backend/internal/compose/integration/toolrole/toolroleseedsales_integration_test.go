// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"maps"
	"net/http"
	"strconv"

	"github.com/margince/margince/backend/internal/compose/integration"
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
		memberSeeders, accessSeeders, salesSeeders, leadSeeders, roomSeeders, workSeeders, meetingSeeders, reportSeeders, companySeeders, weeklySeeders, captureSeeders, complianceSeeders, reportingSeeders, projectSeeders, setupSeeders, adminSeeders, voiceSeeders, channelSeeders,
	} {
		maps.Copy(seeders, group(s))
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
		return s.post("/v1/deals/"+s.create["deal"]()+"/offers", integration.AnyMap{
			"currency": "EUR", "source": "manual",
			"line_items": []integration.AnyMap{{"description": "Retainer", "quantity": 1, "unit_price_minor": 500000, "tax_rate": 19.0}},
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
			s.call("POST", "/v1/offers/"+id+"/render", integration.AnyMap{}, nil, nil)
			return id
		},
		"lost_deal": func() string {
			id := s.create["deal"]()
			stages := apptest.DefaultPipeline(s.t, s.e)
			s.call("POST", "/v1/deals/"+id+"/advance", integration.AnyMap{"to_stage_id": stages.Lost, "lost_reason": "Role sweep"}, nil, nil)
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
					IsDefault bool `json:"is_default"`
					Stages    []struct {
						ID       string `json:"id"`
						Semantic string `json:"semantic"`
					} `json:"stages"`
				} `json:"data"`
			}
			s.call("GET", "/v1/pipelines", nil, nil, &pipelines)
			first := apptest.DefaultPipeline(s.t, s.e).Open
			for _, pipeline := range pipelines.Data {
				for _, stage := range pipeline.Stages {
					if pipeline.IsDefault && stage.Semantic == "open" && stage.ID != first {
						return stage.ID
					}
				}
			}
			s.t.Fatal("the seeded pipeline has one open stage, so no deal can move between two")
			return ""
		},
		// Only a change Margince applied can be accepted, and only a correction
		// row makes one; the seed records it against the move's audit row.
		"auto_moved_deal": func() string {
			id := s.create["deal"]()
			s.call("POST", "/v1/deals/"+id+"/advance", integration.AnyMap{"to_stage_id": s.create["open_stage"]()}, nil, nil)
			move := s.ownerScalar(`SELECT id::text FROM audit_log
				WHERE entity_type = 'deal' AND entity_id = $1 AND after ? 'stage_id' ORDER BY occurred_at DESC LIMIT 1`, id)
			s.ownerExec(`INSERT INTO deal_correction (deal_id, audit_log_id, correction, fields,
				remaining_open_stages, asking, standing_close_date, captured_by)
				VALUES ($1, $2, 'stage_move', '{stage_id}', '', '', '', 'role-sweep')`, id, move)
			var deal struct {
				Version int `json:"version"`
			}
			s.call("GET", "/v1/deals/"+id, nil, nil, &deal)
			s.pool.note(id, "move", move)
			s.pool.note(id, "version", strconv.Itoa(deal.Version))
			return id
		},
		// A deal priced from one line, at the revision that line left it on.
		"priced_deal": func() string {
			id := s.create["deal"]()
			var state struct {
				CommercialRevision int64 `json:"commercial_revision"`
				Lines              []struct {
					ID string `json:"id"`
				} `json:"lines"`
			}
			s.call("POST", "/v1/deals/"+id+"/lines", integration.AnyMap{
				"expected_revision": 0, "replace_estimate": true, "description": "Role Sweep",
				"quantity": 1, "unit_price_minor": 1000,
			}, nil, &state)
			s.pool.note(id, "line", state.Lines[0].ID)
			s.pool.note(id, "revision", strconv.FormatInt(state.CommercialRevision, 10))
			return id
		},
		"offer_from_lines": func() string {
			deal := s.create["priced_deal"]()
			revision := s.pool.fact(deal, "revision")
			from, err := strconv.ParseInt(revision, 10, 64)
			if err != nil {
				s.t.Fatalf("reading the seeded revision: %v", err)
			}
			var offer struct {
				ID      string `json:"id"`
				Version int    `json:"version"`
			}
			s.call("POST", "/v1/deals/"+deal+"/offers", integration.AnyMap{
				"currency": "EUR", "source": "manual", "from_deal_revision": from,
			}, nil, &offer)
			s.pool.note(offer.ID, "revision", revision)
			s.pool.note(offer.ID, "version", strconv.Itoa(offer.Version))
			return offer.ID
		},
		"legal_review": func() string {
			return s.post("/v1/deals/"+s.create["deal"]()+"/legal-reviews", integration.AnyMap{})
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
	me := apptest.SessionUserID(s.t, s.e)
	lead := func(body integration.AnyMap) string {
		body["full_name"], body["source"], body["owner_id"] = "Role Sweep", "manual", me
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
			id := lead(integration.AnyMap{})
			s.call("POST", "/v1/leads/"+id+"/promote", integration.AnyMap{"trigger": "human_qualify"}, pinned(versionOfLead(id)), nil)
			return id
		},
		"disqualified_lead": func() string {
			id := lead(integration.AnyMap{})
			s.call("DELETE", "/v1/leads/"+id, integration.AnyMap{}, pinned(versionOfLead(id)), nil)
			return id
		},
		// An agent changes the terms of a live partner and makes no new one.
		"live_partner": func() string {
			partner := s.create["company"]()
			s.call("PUT", "/v1/companies/"+partner+"/partner", integration.AnyMap{"partner_role": "hosting"}, nil, nil)
			return partner
		},
		"partner_lead": func() string {
			partner := s.create["company"]()
			s.call("PUT", "/v1/companies/"+partner+"/partner", integration.AnyMap{"partner_role": "hosting"}, nil, nil)
			return lead(integration.AnyMap{"partner_company_id": partner})
		},
		"matched_lead": func() string {
			domain := "sweep-" + ids.NewV7().String()[24:] + ".test"
			company := s.post("/v1/companies", integration.AnyMap{
				"display_name": "Sweep " + domain, "source": "manual", "domains": []integration.AnyMap{{"domain": domain, "is_primary": true}},
			})
			id := lead(integration.AnyMap{"email": "buyer@" + domain, "company_name": "Sweep " + domain})
			s.pool.note(id, "company", company)
			return id
		},
		"domain_company": func() string {
			domain := "sweep-" + ids.NewV7().String()[24:] + ".test"
			return s.post("/v1/companies", integration.AnyMap{
				"display_name": "Sweep " + domain, "source": "manual", "domains": []integration.AnyMap{{"domain": domain, "is_primary": true}},
			})
		},
		"linkable_contact": func() string {
			domain := "sweep-" + ids.NewV7().String()[24:] + ".test"
			contact := s.post("/v1/contacts", integration.AnyMap{
				"full_name": "Role Sweep", "source": "manual",
				"emails": []integration.AnyMap{{"email": "buyer@" + domain, "email_type": "work", "is_primary": true}},
			})
			company := s.post("/v1/companies", integration.AnyMap{
				"display_name": "Sweep " + domain, "source": "manual", "domains": []integration.AnyMap{{"domain": domain, "is_primary": true}},
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
		return s.post("/v1/deal-rooms", integration.AnyMap{"deal_id": deal, "title": "Role Sweep", "source": "manual"}), deal
	}
	dealFile := func(deal string) string {
		status, raw := apptest.UploadAs(s.t, s.e, nil, "deal", deal, []byte("%PDF-1.4 role sweep"))
		if status != http.StatusCreated {
			s.t.Fatalf("seeding a deal's file → %d %s", status, raw)
		}
		return apptest.AttachmentIDOf(s.t, raw)
	}
	inRoom := func(path string, body integration.AnyMap) string {
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
			s.call("POST", "/v1/deal-rooms/"+id+"/documents", integration.AnyMap{"attachment_id": dealFile(deal), "group_key": "commercial", "source": "manual"}, nil, &created)
			s.pool.note(created.ID, "room", id)
			return created.ID
		},
		"room_participant": func() string {
			return inRoom("/participants", integration.AnyMap{"full_name": "Role Sweep", "email": uniqueEmail(), "capability": "comment", "source": "manual"})
		},
		"room_thread": func() string { return inRoom("/threads", integration.AnyMap{"body": "Role sweep", "source": "manual"}) },
		"paused_room": func() string {
			id, _ := room()
			s.call("POST", "/v1/deal-rooms/"+id+"/pause", integration.AnyMap{}, nil, nil)
			return id
		},
		"read_attachment": func() string {
			file := dealFile(s.create["deal"]())
			s.pool.note(file, "reading", apptest.SeedExtractionReading(s.e, s.t, file, []extraction.ExtractedField{{
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
