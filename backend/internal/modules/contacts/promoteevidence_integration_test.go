// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// evidenceSeed is one activity a promotion may cite.
type evidenceSeed struct {
	kind, direction, meetingStatus string
	archived                       bool
	// at is when it happened; zero is the database's now.
	at time.Time
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
	var at *time.Time
	if !seed.at.IsZero() {
		at = &seed.at
	}
	e.mustExec(t,
		`INSERT INTO activity (id, kind, subject, direction, meeting_status, occurred_at, source, captured_by, archived_at,
		                       counterparty_email)
		 VALUES ($1, $2, 'Evidence', NULLIF($3, ''), NULLIF($4, ''), coalesce($8, now()), 'manual', 'human:x', CASE WHEN $5 THEN now() END,
		         coalesce(NULLIF($6, ''), (SELECT email FROM lead WHERE id = $7)))`,
		id, seed.kind, seed.direction, seed.meetingStatus, seed.archived, counterparty, leadID, at)
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
	t.Run("a meeting with no recorded status proves a booking", func(t *testing.T) {
		e := setupPromoteConsent(t)
		lead := e.seedLead(t, "unrecorded@evidence.test")
		meeting := e.seedEvidence(t, &lead, evidenceSeed{kind: "meeting"})
		if _, _, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: string(TriggerMeetingBooked)}); err != nil {
			t.Fatalf("promote as meeting_booked on a captured meeting with no status: %v", err)
		}
		if got := e.promotedEvidence(t, lead); got != meeting.String() {
			t.Errorf("audit evidence = %q, want the unrecorded meeting %s", got, meeting)
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

// Engagement older than the waiting horizon no longer qualifies a lead. The read
// stops offering it, and a promotion that must find it on the lead is refused.
func TestEngagementOutsideTheWaitingHorizonNoLongerQualifies(t *testing.T) {
	asOf := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	previous := leadSLAClock
	leadSLAClock = func() time.Time { return asOf }
	t.Cleanup(func() { leadSLAClock = previous })
	horizon := storekit.DefaultWaitingHorizonDays * 24 * time.Hour
	for _, tc := range []struct {
		name    string
		age     time.Duration
		offered bool
	}{{"inside the horizon", horizon - time.Hour, true}, {"outside the horizon", horizon + time.Hour, false}} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupPromoteConsent(t)
			lead := e.seedLead(t, "aged@evidence.test")
			e.seedEvidence(t, &lead, evidenceSeed{kind: "email", direction: "inbound", at: asOf.Add(-tc.age)})
			read, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if (read.QualificationEvidence != nil) != tc.offered {
				t.Errorf("a reply %s old: the read offers %+v, want offered=%t", tc.age, read.QualificationEvidence, tc.offered)
			}
			_, _, err = e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: string(TriggerInboundReply)})
			refusal, refused := errors.AsType[*PromoteEvidenceError](err)
			if refused == tc.offered || (!refused && err != nil) {
				t.Fatalf("promote on a reply %s old: %v, want refused=%t", tc.age, err, !tc.offered)
			}
			if !refused {
				return
			}
			if field, _, _ := refusal.FieldFault(); field != fieldTrigger {
				t.Errorf("the refusal names %q, want %q", field, fieldTrigger)
			}
		})
	}
}

// One meeting reads the same to the lead's evidence, its score and its status
// ladder, whatever its recorded status, through each reader's own query.
func TestEvidenceScoringAndTheLadderReadAMeetingAlike(t *testing.T) {
	for status, want := range map[string]PromoteTrigger{
		"": TriggerMeetingBooked, "booked": TriggerMeetingBooked, "held": TriggerMeetingHeld, "canceled": "", "no_show": "",
	} {
		t.Run("status "+status, func(t *testing.T) {
			e := setupPromoteConsent(t)
			lead := e.seedLead(t, "alike@evidence.test")
			e.seedEvidence(t, &lead, evidenceSeed{kind: "meeting", meetingStatus: status})
			read, err := e.store.GetLead(e.ctx, lead, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			var offered PromoteTrigger
			if read.QualificationEvidence != nil {
				offered = PromoteTrigger(read.QualificationEvidence.Trigger)
			}
			var scored PromoteTrigger
			var touches []leadResponseTouch
			if err := e.store.tx(e.ctx, func(tx pgx.Tx) error {
				signals, err := leadBehavioralSignals(e.ctx, tx, lead)
				if len(signals) == 1 {
					scored = PromoteTrigger(signals[0].Kind)
				}
				if err != nil {
					return err
				}
				touches, err = leadTouchesFor(e.ctx, tx, lead, endOfTime)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if len(touches) != 1 {
				t.Fatalf("the ladder read %d touches, want the one meeting", len(touches))
			}
			step, engaged := ladderStepFor(touches[0])
			if offered != want || scored != want || engaged != (want != "") || (engaged && step != LeadStatusEngaged) {
				t.Errorf("evidence %q, score signal %q, ladder engaged=%t; want %q from all three", offered, scored, engaged, want)
			}
		})
	}
}
