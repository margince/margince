// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// systemContext is a background pass's principal in the installation's
// workspace: the scouts and notifiers that raise what a user then decides.
func (s sweepSeed) systemContext(actor string) context.Context {
	ws := apptest.InstallationWorkspaceUUID(context.Background(), s.t, s.e.Pool)
	return principal.SystemActing(principal.WithWorkspaceID(context.Background(), ws), actor)
}

// ownerScalar reads one value as the database owner, for an id no route lists.
//
//craft:ignore naked-any SQL arguments take whichever Go type their column holds
func (s sweepSeed) ownerScalar(sql string, args ...any) string {
	s.t.Helper()
	var value string
	if err := s.e.Owner.QueryRow(s.t.Context(), sql, args...).Scan(&value); err != nil {
		s.t.Fatalf("reading %q: %v", sql, err)
	}
	return value
}

// workSeeders make what the attention and activity calls act on: suggestions
// a scout raised, approvals an agent staged, a draft, a captured thread.
func workSeeders(s sweepSeed) map[string]func() string {
	return map[string]func() string{
		"inbound_email": func() string {
			return s.post("/v1/activities", AnyMap{
				"kind": "email", "direction": "inbound", "subject": "Role sweep", "body": "Role sweep",
				"source": "manual", "rfc_message_id": "<sweep-" + ids.NewV7().String() + "@role-sweep.test>",
			})
		},
		"mail_draft": func() string {
			company := s.create["company"]()
			var draft struct {
				ID string `json:"id"`
			}
			s.call("PUT", "/v1/mail-drafts", AnyMap{"anchor_type": "company", "anchor_id": company}, nil, &draft)
			s.pool.note(draft.ID, "anchor", company)
			return draft.ID
		},
		// Only a mailbox's capture records which seat imported a thread.
		"captured_thread": func() string {
			key := "<sweep-" + ids.NewV7().String() + "@role-sweep.test>"
			email := s.post("/v1/activities", AnyMap{
				"kind": "email", "direction": "inbound", "subject": "Role sweep", "source": "manual", "rfc_message_id": key,
			})
			s.ownerExec(`INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, email, sessionUserID(s.t, s.e))
			return s.ownerScalar(`SELECT thread_key FROM activity WHERE id = $1`, email)
		},
		"deal_suggestion": func() string {
			stem := ids.NewV7().String()[24:]
			_, suggestion := seedScoutedNamed(s.t, s.e, "Sweep "+stem, "Buyer "+stem)
			return suggestion
		},
		"tag_suggestion": func() string { return tagSuggestion(s) },
		"notice": func() string {
			id, err := notices.NewStore(compose.InstallationDB(s.e.Pool)).Create(s.systemContext("system:role-sweep"), notices.NewNotice{
				Recipient: ids.From[ids.UserKind](ids.MustParse(sessionUserID(s.t, s.e))), Kind: "automation",
				Subject: "Role sweep", DedupeKey: "role-sweep:" + ids.NewV7().String(),
			})
			if err != nil {
				s.t.Fatalf("raising a notice: %v", err)
			}
			return id.String()
		},
		"approval_bundle": func() string { return stageBundleRows(s.t, s.e, s.create["company"](), 2).String() },
		"approval":        func() string { return stagedApproval(s) },
		// Only a released effect that errors fails, so the seed records the
		// failure the release would have.
		"failed_approval": func() string {
			id := stagedApproval(s)
			s.call("POST", "/v1/approvals/"+id+"/approve", AnyMap{}, nil, nil)
			s.pool.note(id, "failed_at", s.ownerScalar(`
				UPDATE approval SET effect_failed_at = now(), effect_failure = 'the released work did not run'
				 WHERE id = $1 RETURNING to_char(effect_failed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`, id))
			return id
		},
		"claim": func() string {
			contact := s.create["contact"]()
			activity := s.post("/v1/activities", AnyMap{
				"kind": "note", "subject": "Role sweep", "body": "Role sweep", "source": "manual",
				"links": []AnyMap{{"entity_type": "contact", "entity_id": contact}},
			})
			return s.post("/v1/contacts/"+contact+"/claims", AnyMap{
				"kind": "commitment_ours", "body": "Send the deck", "source_activity_id": activity, "source_quote": "Role sweep",
			})
		},
		"routed_contact": func() string {
			contact, colleague := s.create["contact"](), s.create["user"]()
			seedRoute(s.t, s.e, ids.MustParse(colleague), ids.MustParse(contact))
			s.pool.note(contact, "via", colleague)
			return contact
		},
		"owner_contact": func() string {
			id := s.create["contact"]()
			s.call("PATCH", "/v1/contacts/"+id, AnyMap{"visibility": "owner"}, nil, nil)
			return id
		},
		"dedupe_candidate": func() string {
			stem := "Sweep" + ids.NewV7().String()[24:]
			incumbent := seedCompanyPairHTTP(s.t, s.e, stem, strings.ToLower(stem)+".test", strings.ToLower(stem)+"-inc.test")
			var queue struct {
				Data []json.RawMessage `json:"data"`
			}
			s.call("GET", "/v1/dedupe/candidates", nil, nil, &queue)
			for _, candidate := range queue.Data {
				var row struct {
					ID string `json:"id"`
				}
				if strings.Contains(string(candidate), incumbent) && json.Unmarshal(candidate, &row) == nil {
					return row.ID
				}
			}
			s.t.Fatalf("no dedupe candidate names the company %s", incumbent)
			return ""
		},
	}
}

// consentedContact is a contact with an address of its own, transactional
// consent granted, and an inbound email from it to answer.
func consentedContact(s sweepSeed) string {
	email := uniqueEmail()
	contact := s.post("/v1/contacts", AnyMap{"full_name": "Role Sweep", "source": "manual", "emails": []AnyMap{{"email": email}}})
	var purposes struct {
		Data []struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"data"`
	}
	s.call("GET", "/v1/consent-purposes", nil, nil, &purposes)
	for _, purpose := range purposes.Data {
		if purpose.Key == "transactional" {
			s.call("POST", "/v1/contacts/"+contact+"/consent", AnyMap{
				"purpose_id": purpose.ID, "new_state": "granted", "lawful_basis": "consent", "wording": "Yes, you may contact me about this.",
			}, nil, nil)
		}
	}
	// Transactional consent holds where a live deal stakes the recipient and the
	// message answered is filed on it.
	deal := apptest.StakeOnOpenDeal(s.t, s.e, "Sweep "+ids.NewV7().String(), apptest.DiscoverSeededPipeline(s.t, s.e), contact)
	anchor := s.post("/v1/activities", AnyMap{
		"kind": "email", "subject": "Role sweep", "direction": "inbound", "source": "manual",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": contact}, {"entity_type": "deal", "entity_id": deal}},
	})
	s.pool.note(contact, "email", email)
	s.pool.note(contact, "anchor", anchor)
	s.pool.note(contact, "deal", deal)
	return contact
}

// bookedSlots counts the half hours the sweep has booked, so no two bookings
// in one run ask for the same one.
var bookedSlots atomic.Int64

// bookableSlot is a half hour no earlier booking of the sweep holds, inside the
// booking page's hours. There are twelve a weekday from 10:00 UTC, starting two
// days after the sweep began.
func bookableSlot(p *recordPool) time.Time {
	n := int(bookedSlots.Add(1))
	day := p.sweepStart.Truncate(24 * time.Hour).Add(48 * time.Hour)
	for skipped := 0; ; day = day.Add(24 * time.Hour) {
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		if skipped == n/12 {
			break
		}
		skipped++
	}
	return day.Add(10*time.Hour + time.Duration(n%12)*30*time.Minute)
}

// meeting is one the admin hosts, with its preparation's revision, and
// confirmed ready at it when ready is asked.
func meeting(s sweepSeed, ready bool) string {
	id := s.post("/v1/activities", AnyMap{
		"kind": "meeting", "subject": "Role sweep", "source": "manual", "meeting_status": "booked",
		"occurred_at": s.pool.sweepStart.Add(30 * 24 * time.Hour).Format(time.RFC3339),
	})
	var prep struct {
		Preparation struct {
			Revision string `json:"revision"`
		} `json:"preparation"`
	}
	s.call("GET", "/v1/meeting-prep?record_type=activity&record_id="+id, nil, nil, &prep)
	if ready {
		s.call("PUT", "/v1/worklist/ready", AnyMap{"activity_id": id, "preparation_revision": prep.Preparation.Revision}, nil, nil)
	}
	s.pool.note(id, "revision", prep.Preparation.Revision)
	return id
}

// doneLine is the id of the newest line the magic answer counts as done.
func doneLine(magic json.RawMessage) string {
	var answer struct {
		Done []struct {
			ID string `json:"id"`
		} `json:"done"`
	}
	if json.Unmarshal(magic, &answer) != nil || len(answer.Done) == 0 {
		return ""
	}
	return answer.Done[0].ID
}

// worklistRow is the id of the worklist row of a lane that names the record.
//
//craft:ignore naked-any a worklist is walked as decoded JSON, whatever its shape
func worklistRow(worklist json.RawMessage, source, record string) string {
	var tree any
	if json.Unmarshal(worklist, &tree) != nil {
		return ""
	}
	var walk func(node any) string
	walk = func(node any) string {
		switch typed := node.(type) {
		case map[string]any:
			if raw, err := json.Marshal(typed); err == nil && typed["source"] == source && strings.Contains(string(raw), record) {
				if id, ok := typed["id"].(string); ok {
					return id
				}
			}
			for _, child := range typed {
				if found := walk(child); found != "" {
					return found
				}
			}
		case []any:
			for _, child := range typed {
				if found := walk(child); found != "" {
					return found
				}
			}
		}
		return ""
	}
	return walk(tree)
}

// stagedApproval is one approval an agent's pass staged for the admin.
func stagedApproval(s sweepSeed) string {
	bundle := stageBundleRows(s.t, s.e, s.create["company"](), 1)
	return s.ownerScalar(`SELECT id::text FROM approval WHERE bundle_id = $1`, bundle)
}

// tagSuggestion is what the tag scout raises for a suggestible tag a note on
// a contact names.
func tagSuggestion(s sweepSeed) string {
	word := "Zq" + ids.NewV7().String()[24:]
	tag := s.post("/v1/tags", AnyMap{"name": word, "description": strings.ToLower(word) + " pricing"})
	s.call("PATCH", "/v1/tags/"+tag, AnyMap{"suggestible": true}, nil, nil)
	contact := s.create["contact"]()
	s.post("/v1/activities", AnyMap{
		"kind": "note", "subject": "Pricing", "body": "Asked about " + word + " pricing.", "source": "manual",
		"occurred_at": clocktest.Now(s.t).Add(-time.Hour).UTC().Format(time.RFC3339),
		"links":       []AnyMap{{"entity_type": "contact", "entity_id": contact}},
	})
	ctx := s.systemContext("agent:tag-scout")
	if err := database.WithWorkspaceTx(ctx, s.e.Pool, func(tx pgx.Tx) error {
		_, err := compose.RunTagScout(ctx, tx, clocktest.Now(s.t))
		return err
	}); err != nil {
		s.t.Fatalf("the tag scout pass: %v", err)
	}
	return s.ownerScalar(`SELECT id::text FROM tag_suggestion WHERE tag_id = $1 AND contact_id = $2`, tag, contact)
}

// meetingSeeders make the conversations and meetings the activity and send
// calls act on: a consented contact, a meeting, an invitation, a channel.
func meetingSeeders(s sweepSeed) map[string]func() string {
	return map[string]func() string{
		"consented_contact": func() string { return consentedContact(s) },
		"invitation": func() string {
			contact := consentedContact(s)
			start := bookableSlot(s.pool)
			var invited struct {
				ID      string `json:"id"`
				Version int    `json:"version"`
			}
			s.call("POST", "/v1/scheduling/invitations", AnyMap{
				"contact_id": contact, "attendee_email": s.pool.fact(contact, "email"), "subject": "Role sweep",
				"start": start.Format(time.RFC3339), "end": start.Add(30 * time.Minute).Format(time.RFC3339),
			}, nil, &invited)
			s.pool.note(invited.ID, "version", strconv.Itoa(invited.Version))
			moved := bookableSlot(s.pool)
			s.pool.note(invited.ID, "start", moved.Format(time.RFC3339))
			s.pool.note(invited.ID, "end", moved.Add(30*time.Minute).Format(time.RFC3339))
			return invited.ID
		},
		"enrichment_run": func() string {
			s.call("PUT", "/v1/provider-connections/surfe", AnyMap{"api_key": "role-sweep"}, nil, nil)
			contact := consentedContact(s)
			var run struct {
				ID string `json:"id"`
			}
			s.call("POST", "/v1/contacts/"+contact+"/enrichment-runs", AnyMap{"provider": "surfe"}, nil, &run)
			s.pool.note(run.ID, "contact", contact)
			return run.ID
		},
		// A Telegram conversation is one per bot and account, so each role
		// replies on the one the first seed captured.
		"channel_conversation": func() string {
			c := &channelSendEnv{AppEnv: s.e, user: sessionUserID(s.t, s.e)}
			if existing := s.ownerScalar(`SELECT count(*)::text FROM activity WHERE thread_key = $1`, channelSendThreadKey); existing != "0" {
				return s.ownerScalar(`SELECT id::text FROM activity WHERE thread_key = $1`, channelSendThreadKey)
			}
			c.contactID = s.create["contact"]()
			c.bindIdentity(s.t)
			c.seedInboundMessage(s.t)
			c.grantConsent(s.t, "transactional")
			c.connectBot(s.t)
			return c.activityID
		},
		"transcript": func() string {
			return s.post("/v1/activities", AnyMap{
				"kind": "meeting", "subject": "Role sweep", "body": "We agreed to send the deck.", "source": "manual", "source_system": "transcript",
			})
		},
		"meeting": func() string { return meeting(s, false) },
		// The worklist offers a meeting today with an outside party as a row
		// with Done.
		"worklist_meeting": func() string {
			contact := s.create["contact"]()
			id := s.post("/v1/activities", AnyMap{
				"kind": "meeting", "subject": "Role sweep", "source": "manual", "occurred_precision": "day",
				"occurred_at": clocktest.Now(s.t).UTC().Format(time.RFC3339),
				"links":       []AnyMap{{"entity_type": "contact", "entity_id": contact}},
			})
			var worklist json.RawMessage
			s.call("GET", "/v1/worklist", nil, nil, &worklist)
			row := worklistRow(worklist, "meeting", id)
			if row == "" {
				s.t.Fatalf("the worklist offers no meeting row for %s: %.2000s", id, worklist)
			}
			return row
		},
		// Only an invitation the calendar took is confirmed, which the meeting
		// delivery job records.
		// A done line is an agent's work, so an agent's passport makes a company.
		"magic_line": func() string {
			bearer, _ := passportWithID(s.t, s.e, "role sweep magic "+ids.NewV7().String(), "read", "write")
			var created json.RawMessage
			if status := s.e.Call(s.t, "POST", "/v1/companies", AnyMap{"display_name": uniqueName(), "source": "manual"}, bearer, &created); status != 201 {
				s.t.Fatalf("an agent making a company → %d %s", status, created)
			}
			var magic json.RawMessage
			s.call("GET", "/v1/magic", nil, nil, &magic)
			line := doneLine(magic)
			if line == "" {
				s.t.Fatalf("the magic answer names no done line: %.3000s", magic)
			}
			s.pool.note(line, "since", clocktest.Now(s.t).Add(-24*time.Hour).UTC().Format(time.RFC3339))
			return line
		},
		"confirmed_invitation": func() string {
			id := s.create["invitation"]()
			s.ownerExec(`UPDATE meeting_invitation SET status = 'confirmed' WHERE activity_id = $1`, id)
			return id
		},
		"vcard_contact": func() string {
			email := uniqueEmail()
			contact := s.post("/v1/contacts", AnyMap{"full_name": "Rita Sweep", "source": "manual", "title": "CFO",
				"emails": []AnyMap{{"email": email, "email_type": "work", "is_primary": true}}})
			card := "BEGIN:VCARD\nVERSION:3.0\nFN:Rita Sweep\nTITLE:VP Finance\nEMAIL;TYPE=WORK:" + email + "\nEND:VCARD\n"
			form, contentType := multipartDocument(s.t, "rita.vcf", "text/vcard", []byte(card))
			if status, raw := postUpload(s.t, s.e, "/v1/contacts/vcard-import", nil, form, contentType); status < 200 || status >= 300 {
				s.t.Fatalf("importing a vCard → %d %s", status, raw)
			}
			return contact
		},
		"ready_meeting": func() string { return meeting(s, true) },
		"scheduled_send": func() string {
			contact := consentedContact(s)
			var scheduled struct {
				ID      string `json:"id"`
				Version int    `json:"version"`
			}
			s.call("POST", "/v1/activities/"+s.pool.fact(contact, "anchor")+"/send-email", AnyMap{
				"subject": "Role sweep", "body": "Role sweep", "to": []string{s.pool.fact(contact, "email")},
				"consent_purpose": "transactional", "scheduled_tz": "Europe/Berlin",
				"scheduled_at": s.pool.sweepStart.Add(30 * 24 * time.Hour).Format(time.RFC3339),
			}, nil, &scheduled)
			s.pool.note(scheduled.ID, "version", strconv.Itoa(scheduled.Version))
			return scheduled.ID
		},
	}
}
