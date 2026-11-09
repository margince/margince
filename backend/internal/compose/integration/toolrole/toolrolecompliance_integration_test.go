// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// complianceFixtures fill the consent, privacy and retention calls. Each acts
// on a subject that can take the act. That is a contact with a mailbox, a stop,
// a vouch, a notice duty or a refused send.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
var complianceFixtures = map[string]func(p *recordPool) map[string]any{
	"list_contact_overrides": func(p *recordPool) map[string]any {
		vouch := p.take("consent_override")
		return map[string]any{"id": p.fact(vouch, "contact")}
	},
	"record_consent": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("mail_contact"), "request_body": map[string]any{
			"purpose_id": p.take("consent_purpose"), "new_state": "granted", "lawful_basis": "consent",
			"source": "web_form", "wording": "You agree we may write to you.",
		}}
	},
	"record_qualifying_event": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("contact"), "request_body": map[string]any{
			"kind": "in_person", "note": "Met at the fair.", "occurred_at": pastInstant(p),
		}}
	},
	"retract_qualifying_event": func(p *recordPool) map[string]any {
		event := p.take("qualifying_event")
		return map[string]any{"id": p.fact(event, "contact"), "event_id": event}
	},
	"retract_acquisition_request": byID("acquisition_contact"),
	"lift_contact_suppression": func(p *recordPool) map[string]any {
		stop := p.take("suppression")
		return map[string]any{"id": p.fact(stop, "contact"), "suppressionId": stop, "request_body": map[string]any{"reason": "They asked us to carry on."}}
	},
	"lift_lead_suppression": func(p *recordPool) map[string]any {
		stop := p.take("lead_suppression")
		return map[string]any{"id": p.fact(stop, "lead"), "suppressionId": stop, "request_body": map[string]any{"reason": "They asked us to carry on."}}
	},
	"revoke_contact_override": func(p *recordPool) map[string]any {
		vouch := p.take("consent_override")
		return map[string]any{"id": p.fact(vouch, "contact"), "overrideId": vouch, "request_body": map[string]any{"reason": "Another contact."}}
	},
	"place_legal_hold": func(p *recordPool) map[string]any {
		return map[string]any{"entityType": "contact", "recordId": p.take("contact"), "request_body": map[string]any{"reason": "Litigation."}}
	},
	"pin_activity_to_floor": func(p *recordPool) map[string]any {
		return map[string]any{"activityId": p.take("contact_note"), "request_body": map[string]any{"reason": "A record worth keeping."}}
	},
	"update_data_subject_request": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("subject_request"), "request_body": map[string]any{"status": "in_progress"}}
	},
	"get_notice_case":            byID("notice_case"),
	"get_notice_case_review":     byID("notice_case_review"),
	"get_notice_source_evidence": byID("notice_case_review"),
	"preview_notice_source": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("notice_case_review"), "request_body": map[string]any{
			"kind": "subject_initiated", "note": "Their own message confirms direct collection.",
		}}
	},
	"assign_notice_case": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("notice_case"), "request_body": map[string]any{"owner_user_id": p.take("me")}}
	},
	"assign_notice_case_review": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("notice_case_review"), "request_body": map[string]any{"owner_user_id": p.take("me")}}
	},
	"confirm_notice_source": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("notice_case_review"), "request_body": map[string]any{
			"kind": "subject_initiated", "note": "Their own message confirms direct collection.",
		}}
	},
	"resolve_confirm_submission": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("confirm_submission"), "request_body": map[string]any{"resolution": "accepted"}}
	},
	"get_communication_review": byID("communication_review"),
	"record_communication_context": func(p *recordPool) map[string]any {
		review := p.take("correspondence_review")
		return map[string]any{"id": review, "request_body": map[string]any{
			"subject_id": p.fact(review, "contact"), "kind": "in_person", "note": "They asked for it at the fair.", "occurred_at": pastInstant(p),
		}}
	},
	"direct_communication_send": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("communication_review"), "request_body": map[string]any{
			"reason_code": "contractual_necessity", "explanation": "The service agreement obliges us to send this notice.",
			"warning_version": consent.OverrideWarningVersion, "acknowledged": true,
		}}
	},
	"preview_controller_mail": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("mail_contact"), "kind": "privacy_notice"}
	},
	"request_details_confirmation": byID("mail_contact"),
	"send_privacy_notice":          byID("mail_contact"),
	"issue_double_opt_in": func(p *recordPool) map[string]any {
		return map[string]any{"id": p.take("mail_contact"), "request_body": map[string]any{"purpose_id": p.take("double_opt_in_purpose")}}
	},
}

// pastInstant is a moment a remembered exchange can have happened at.
func pastInstant(p *recordPool) string { return p.sweepStart.Add(-48 * time.Hour).Format(time.RFC3339) }

// complianceSeeders make the subjects the compliance calls act on, through the
// app's own routes where one makes them.
func complianceSeeders(s sweepSeed) map[string]func() string {
	mailContact := func() string {
		return s.post("/v1/contacts", integration.AnyMap{"full_name": "Role Sweep", "source": "manual", "emails": []integration.AnyMap{{"email": uniqueEmail()}}})
	}
	purpose := func(doubleOptIn bool) func() string {
		return func() string {
			var purposes struct {
				Data []struct {
					ID                  string `json:"id"`
					RequiresDoubleOptIn bool   `json:"requires_double_opt_in"`
				} `json:"data"`
			}
			s.call("GET", "/v1/consent-purposes", nil, nil, &purposes)
			for _, p := range purposes.Data {
				if p.RequiresDoubleOptIn == doubleOptIn {
					return p.ID
				}
			}
			s.t.Fatalf("the seeded purposes hold none with requires_double_opt_in=%v", doubleOptIn)
			return ""
		}
	}
	reviewCase := func() string {
		contact := s.create["contact"]()
		apptest.DriveNoticeCase(s.t, s.e, contact)
		return s.ownerScalar(`SELECT id::text FROM privacy_notice_case WHERE contact_id = $1`, contact)
	}
	qualifying := func() string {
		contact := s.create["contact"]()
		event := s.post("/v1/contacts/"+contact+"/consent/qualifying-events", integration.AnyMap{
			"kind": "in_person", "note": "Met at the fair.",
			"occurred_at": s.pool.sweepStart.Add(-48 * time.Hour).Format(time.RFC3339),
		})
		s.pool.note(event, "contact", contact)
		return event
	}
	return map[string]func() string{
		"mail_contact":          mailContact,
		"consent_purpose":       purpose(false),
		"double_opt_in_purpose": purpose(true),
		"qualifying_event":      qualifying,
		// A contact a web form or lead conversion created as one who asked for
		// a quote. No route of the sweep's creates one with that acquisition.
		"acquisition_contact": func() string {
			contact := s.create["contact"]()
			s.ownerExec(`INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
				VALUES ($1, 'requested_quote_or_meeting', $2, 'system')`, contact, clocktest.Now(s.t))
			return contact
		},
		// A stop is lifted only by a level above the one it was recorded at.
		// The stop reads as the engine's own, which every seat outranks, so the
		// lift measures the role's grant and not the stop's level.
		"suppression": func() string {
			contact := s.create["contact"]()
			s.call("POST", "/v1/contacts/"+contact+"/consent/suppress", integration.AnyMap{"kind": "subject_request", "reason": "Asked on the phone."}, nil, nil)
			stop := s.ownerScalar(`UPDATE communication_suppression SET decided_by_level = 'machine' WHERE contact_id = $1 RETURNING id::text`, contact)
			s.pool.note(stop, "contact", contact)
			return stop
		},
		"lead_suppression": func() string {
			lead := s.create["lead"]()
			stop := s.ownerScalar(`INSERT INTO communication_suppression (lead_id, kind, source, captured_by, decided_by_level)
				VALUES ($1, 'marketing_objection', 'role sweep', 'system', 'machine') RETURNING id::text`, lead)
			s.pool.note(stop, "lead", lead)
			return stop
		},
		"consent_override": func() string {
			contact := s.create["contact"]()
			var vouch struct {
				ID string `json:"override_id"`
			}
			s.call("POST", "/v1/contacts/"+contact+"/consent/allow", integration.AnyMap{"category": "marketing", "reason": "They asked at the fair."}, nil, &vouch)
			s.pool.note(vouch.ID, "contact", contact)
			return vouch.ID
		},
		"contact_note": func() string {
			return s.post("/v1/activities", integration.AnyMap{
				"kind": "note", "subject": "A record worth keeping", "source": "manual",
				"links": []integration.AnyMap{{"entity_type": "contact", "entity_id": s.create["contact"]()}},
			})
		},
		"subject_request": func() string {
			return s.post("/v1/data-subject-requests", integration.AnyMap{"kind": "access", "subject_ref": uniqueEmail()})
		},
		"notice_case_review": reviewCase,
		"notice_case": func() string {
			review := reviewCase()
			s.call("POST", "/v1/privacy/notice-case-reviews/"+review+"/source", integration.AnyMap{
				"kind": "referral", "note": "Confirmed referral", "obtained_at": s.pool.sweepStart.Add(-48 * time.Hour).Format(time.RFC3339),
			}, nil, nil)
			return review
		},
		// The subject's own confirm page files a submission; it takes a link
		// only the subject's mailbox holds.
		"confirm_submission": func() string {
			contact := mailContact()
			token := s.ownerScalar(`INSERT INTO confirm_token (contact_id, token_hash, delivered_to, expires_at)
				VALUES ($1, $2, 'subject@role-sweep.test', $3) RETURNING id::text`, contact, "role-sweep-"+contact, clocktest.Now(s.t).Add(14*24*time.Hour))
			return s.ownerScalar(`INSERT INTO contact_confirm_submission (contact_id, token_id, kind, field, proposed_value)
				VALUES ($1, $2, 'correction', 'full_name', 'Role Sweep Corrected') RETURNING id::text`, contact, token)
		},
		"communication_review":  func() string { return refusedSendReview(s, "business_correspondence") },
		"correspondence_review": func() string { return refusedSendReview(s, "business_correspondence") },
	}
}

// refusedSendReview opens the review a refused send leaves: a send for purpose
// to a contact nothing on record connects to us.
func refusedSendReview(s sweepSeed, purpose string) string {
	email := uniqueEmail()
	contact := s.post("/v1/contacts", integration.AnyMap{"full_name": "Role Sweep", "source": "manual", "emails": []integration.AnyMap{{"email": email}}})
	// A note, not their mail: mail from them would be the evidence the send lacks.
	anchor := s.post("/v1/activities", integration.AnyMap{
		"kind": "note", "subject": "Role sweep", "source": "manual",
		"links": []integration.AnyMap{{"entity_type": "contact", "entity_id": contact}},
	})
	if status := s.e.Call(s.t, "POST", "/v1/activities/"+anchor+"/send-email", integration.AnyMap{
		"subject": "Re: Inbound question", "body": "answer", "to": []string{email}, "consent_purpose": purpose,
	}, nil, nil); status != http.StatusConflict {
		s.t.Fatalf("a %s send with no grant → %d, want the refusal that opens a review", purpose, status)
	}
	review := s.ownerScalar(`SELECT id::text FROM communication_review
 WHERE resolved_at IS NULL AND initiated_by = @initiator ORDER BY id DESC LIMIT 1`, pgx.NamedArgs{"initiator": apptest.SessionUserID(s.t, s.e)})
	s.pool.note(review, "contact", contact)
	return review
}
