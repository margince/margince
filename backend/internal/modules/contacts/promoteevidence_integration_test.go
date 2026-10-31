// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// evidenceSeed is one activity a promotion may cite.
type evidenceSeed struct {
	kind, direction, meetingStatus string
	archived                       bool
}

var (
	inboundReply  = evidenceSeed{kind: "email", direction: "inbound"}
	outboundEmail = evidenceSeed{kind: "email", direction: "outbound"}
	heldMeeting   = evidenceSeed{kind: "meeting", meetingStatus: "held"}
	bookedMeeting = evidenceSeed{kind: "meeting", meetingStatus: "booked"}
)

// seedEvidence writes one hand-logged activity with the lead as its
// counterparty and, when lead is set, links it to that lead.
func (e *promoteConsentEnv) seedEvidence(t *testing.T, lead *ids.LeadID, seed evidenceSeed) ids.ActivityID {
	t.Helper()
	return e.seedEvidenceFrom(t, lead, seed, "")
}

// seedEvidenceFrom is seedEvidence with another counterparty than the lead.
func (e *promoteConsentEnv) seedEvidenceFrom(t *testing.T, lead *ids.LeadID, seed evidenceSeed, counterparty string) ids.ActivityID {
	t.Helper()
	id := ids.New[ids.ActivityKind]()
	var leadID *ids.UUID
	if lead != nil {
		leadID = &lead.UUID
	}
	e.mustExec(t,
		`INSERT INTO activity (id, kind, subject, direction, meeting_status, occurred_at, source, captured_by, archived_at,
		                       counterparty_email)
		 VALUES ($1, $2, 'Evidence', NULLIF($3, ''), NULLIF($4, ''), now(), 'manual', 'human:x', CASE WHEN $5 THEN now() END,
		         coalesce(NULLIF($6, ''), (SELECT email FROM lead WHERE id = $7)))`,
		id, seed.kind, seed.direction, seed.meetingStatus, seed.archived, counterparty, leadID)
	if lead != nil {
		e.mustExec(t, `INSERT INTO activity_link (activity_id, entity_type, lead_id) VALUES ($1, 'lead', $2)`, id, *lead)
	}
	return id
}

// promotedEvidence is the evidence the lead's promote audit row recorded, or "".
func (e *promoteConsentEnv) promotedEvidence(t *testing.T, lead ids.LeadID) string {
	t.Helper()
	var evidence *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT after->>'evidence_activity_id' FROM audit_log
		  WHERE entity_type = 'lead' AND entity_id = $1 AND action = 'promote'`, lead.UUID).Scan(&evidence); err != nil {
		t.Fatalf("read the promote audit row: %v", err)
	}
	if evidence == nil {
		return ""
	}
	return *evidence
}

func TestAPromotionRefusesEvidenceThatDoesNotShowItsTrigger(t *testing.T) {
	cases := []struct {
		name    string
		trigger PromoteTrigger
		// cite seeds the activity the promotion names; nil cites nothing.
		cite      func(t *testing.T, e *promoteConsentEnv, lead ids.LeadID) *ids.ActivityID
		wantField string
	}{
		{"another lead's reply", TriggerInboundReply, func(t *testing.T, e *promoteConsentEnv, _ ids.LeadID) *ids.ActivityID {
			other := e.seedLead(t, "other-"+ids.NewV7().String()+"@evidence.test")
			return new(e.seedEvidence(t, &other, inboundReply))
		}, "evidence.activity_id"},
		{"someone else's reply filed on the lead", TriggerInboundReply, func(t *testing.T, e *promoteConsentEnv, lead ids.LeadID) *ids.ActivityID {
			return new(e.seedEvidenceFrom(t, &lead, inboundReply, "colleague@elsewhere.test"))
		}, "evidence.activity_id"},
		{"a reply linked to nothing", TriggerInboundReply, func(t *testing.T, e *promoteConsentEnv, _ ids.LeadID) *ids.ActivityID {
			return new(e.seedEvidence(t, nil, inboundReply))
		}, "evidence.activity_id"},
		{"an archived reply", TriggerInboundReply, func(t *testing.T, e *promoteConsentEnv, lead ids.LeadID) *ids.ActivityID {
			return new(e.seedEvidence(t, &lead, evidenceSeed{kind: "email", direction: "inbound", archived: true}))
		}, "evidence.activity_id"},
		{"a cancelled meeting", TriggerMeetingBooked, func(t *testing.T, e *promoteConsentEnv, lead ids.LeadID) *ids.ActivityID {
			return new(e.seedEvidence(t, &lead, evidenceSeed{kind: "meeting", meetingStatus: "canceled"}))
		}, "evidence.activity_id"},
		{"a meeting cited as a reply", TriggerInboundReply, func(t *testing.T, e *promoteConsentEnv, lead ids.LeadID) *ids.ActivityID {
			return new(e.seedEvidence(t, &lead, heldMeeting))
		}, "evidence.activity_id"},
		{"a booked meeting cited as held", TriggerMeetingHeld, func(t *testing.T, e *promoteConsentEnv, lead ids.LeadID) *ids.ActivityID {
			return new(e.seedEvidence(t, &lead, bookedMeeting))
		}, "evidence.activity_id"},
		{"our own email cited as a reply", TriggerInboundReply, func(t *testing.T, e *promoteConsentEnv, lead ids.LeadID) *ids.ActivityID {
			return new(e.seedEvidence(t, &lead, outboundEmail))
		}, "evidence.activity_id"},
		{"no citation and no reply on the lead", TriggerInboundReply, func(t *testing.T, e *promoteConsentEnv, lead ids.LeadID) *ids.ActivityID {
			e.seedEvidence(t, &lead, outboundEmail)
			return nil
		}, "trigger"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := setupPromoteConsent(t)
			lead := e.seedLead(t, "refused@evidence.test")
			cited := tc.cite(t, e, lead)

			_, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: string(tc.trigger), EvidenceActivityID: cited})
			refusal, ok := errors.AsType[*PromoteEvidenceError](err)
			if !ok {
				t.Fatalf("promote as %s citing %s → %v, want PromoteEvidenceError", tc.trigger, tc.name, err)
			}
			if field, _, _ := refusal.FieldFault(); field != tc.wantField {
				t.Errorf("the refusal names %q, want %q", field, tc.wantField)
			}
			if _, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly); err != nil {
				t.Errorf("the refused promotion retired the lead anyway: %v", err)
			}
		})
	}
}

func TestAPromotionRecordsTheLeadsOwnEvidence(t *testing.T) {
	t.Run("the cited reply of this lead", func(t *testing.T) {
		e := setupPromoteConsent(t)
		lead := e.seedLead(t, "cited@evidence.test")
		reply := e.seedEvidence(t, &lead, inboundReply)
		if _, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: string(TriggerInboundReply), EvidenceActivityID: &reply}); err != nil {
			t.Fatalf("promote on the lead's own reply: %v", err)
		}
		if got := e.promotedEvidence(t, lead); got != reply.String() {
			t.Errorf("audit evidence = %q, want the cited reply %s", got, reply)
		}
	})
	t.Run("a held meeting proves it was booked", func(t *testing.T) {
		e := setupPromoteConsent(t)
		lead := e.seedLead(t, "booked@evidence.test")
		meeting := e.seedEvidence(t, &lead, heldMeeting)
		if _, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: string(TriggerMeetingBooked), EvidenceActivityID: &meeting}); err != nil {
			t.Fatalf("promote as meeting_booked on a held meeting: %v", err)
		}
	})
	t.Run("no citation takes the lead's matching engagement", func(t *testing.T) {
		e := setupPromoteConsent(t)
		lead := e.seedLead(t, "resolved@evidence.test")
		e.seedEvidence(t, &lead, inboundReply)
		meeting := e.seedEvidence(t, &lead, heldMeeting)
		if _, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: string(TriggerMeetingHeld)}); err != nil {
			t.Fatalf("promote as meeting_held with no citation: %v", err)
		}
		if got := e.promotedEvidence(t, lead); got != meeting.String() {
			t.Errorf("audit evidence = %q, want the held meeting %s", got, meeting)
		}
	})
	t.Run("a human's judgement needs none", func(t *testing.T) {
		e := setupPromoteConsent(t)
		lead := e.seedLead(t, "human@evidence.test")
		if _, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: string(TriggerHumanQualify)}); err != nil {
			t.Fatalf("promote as human_qualify with no evidence: %v", err)
		}
		if got := e.promotedEvidence(t, lead); got != "" {
			t.Errorf("audit evidence = %q, want none", got)
		}
	})
}

// The lead read offers the evidence the promotion then accepts: the
// qualification dialog sends the read's trigger and activity unchanged.
func TestTheLeadReadOffersEvidenceThePromotionAccepts(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "offered@evidence.test")
	e.seedEvidence(t, &lead, inboundReply)
	e.seedEvidence(t, &lead, evidenceSeed{kind: "meeting", meetingStatus: "canceled"})
	read, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	offered := read.QualificationEvidence
	if offered == nil || offered.ActivityId == nil {
		t.Fatal("the lead read offers no qualification evidence for a lead with an inbound reply")
	}
	cited := ids.From[ids.ActivityKind](ids.UUID(*offered.ActivityId))
	if _, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: string(offered.Trigger), EvidenceActivityID: &cited}); err != nil {
		t.Fatalf("promote on the evidence the read offered: %v", err)
	}
	if _, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the lead is still live after its promotion: %v", err)
	}
}

// A writer that archives or unfiles the cited activity while the promotion runs
// is waited for, and the promotion then refuses rather than record it.
func TestAPromotionWaitsForAConcurrentChangeToItsEvidence(t *testing.T) {
	cases := map[string]string{
		"archived": `UPDATE activity SET archived_at = now() WHERE id = $1 AND $2::uuid IS NOT NULL`,
		"unfiled":  `DELETE FROM activity_link WHERE activity_id = $1 AND lead_id = $2`,
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			e := setupPromoteConsent(t)
			ctx := context.Background()
			lead := e.seedLead(t, "racing@evidence.test")
			reply := e.seedEvidence(t, &lead, inboundReply)

			holder, err := e.owner.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { endTx(t, holder) })
			var holderPid int
			if err := holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holderPid); err != nil {
				t.Fatal(err)
			}
			if _, err := holder.Exec(ctx, change, reply, lead.UUID); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				_, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: string(TriggerInboundReply), EvidenceActivityID: &reply})
				done <- err
			}()
			mustBlockOn(t, holder, holderPid, done)
			if err := holder.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if _, refused := errors.AsType[*PromoteEvidenceError](<-done); !refused {
				t.Errorf("the promotion recorded evidence %s while it was %s", reply, name)
			}
		})
	}
}
