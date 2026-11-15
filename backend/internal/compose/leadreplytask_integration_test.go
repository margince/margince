// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A lead who writes to us stays a lead. The mail files on the lead, no contact
// is made, and the lead's owner is asked to qualify them, once at a time.

import (
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// ownedLead creates a live lead holding the address, owned by owner.
func (l *leadMailEnv) ownedLead(t *testing.T, address string, owner ids.UUID) ids.LeadID {
	t.Helper()
	ownerID := ids.From[ids.UserKind](owner)
	created, _, err := l.e.Contacts.CreateLead(l.e.Admin(), contacts.CreateLeadInput{
		FullName: new("Lena Prospect"), Email: new(address), Source: "manual", OwnerID: &ownerID,
	})
	if err != nil {
		t.Fatalf("creating the lead for %s: %v", address, err)
	}
	return ids.From[ids.LeadKind](ids.UUID(created.Id))
}

// qualifyAsks counts the open asks to qualify the lead assigned to owner.
func (l *leadMailEnv) qualifyAsks(t *testing.T, lead ids.LeadID, owner ids.UUID) int {
	t.Helper()
	return l.e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link al ON al.activity_id = a.id
		WHERE al.lead_id = $1 AND a.kind = 'task' AND a.source_system = $2
		  AND NOT a.is_done AND a.assignee_id = $3`, lead, provenance.LeadRepliedSource, owner)
}

func TestAReplyFromALeadAsksItsOwnerToQualifyThemOnce(t *testing.T) {
	l := setupLeadMail(t)
	const address = "lena@prospect.example"
	lead := l.ownedLead(t, address, l.e.Rep2)

	offer := "offer-" + ids.NewV7().String() + "@mail.example"
	outbound := l.mail(t, owedMail{from: l.seat, to: address, subject: "Our offer", messageID: offer}, true)
	if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 0 {
		t.Fatalf("mail we sent asked to qualify the lead %d times, want none: they have not written", n)
	}
	first := l.mail(t, owedMail{
		from: address, to: l.seat, subject: "Re: Our offer",
		inReplyTo: offer, at: l.at.Add(time.Minute),
	}, true)
	second := l.mail(t, owedMail{
		from: address, to: l.seat, subject: "Re: Our offer",
		inReplyTo: offer, at: l.at.Add(2 * time.Minute),
	}, true)

	for name, activity := range map[string]ids.UUID{"outbound": outbound, "first reply": first, "second reply": second} {
		if !l.filedUnderLead(t, activity, lead) {
			t.Errorf("the %s mail is not filed under the lead", name)
		}
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM contact_email WHERE email = $1 AND archived_at IS NULL`, address); n != 0 {
		t.Errorf("%d contact addresses were made for a live lead's address, want none", n)
	}
	if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 1 {
		t.Errorf("two replies left %d open asks to qualify the lead with its owner, want exactly one", n)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND archived_at IS NULL AND status <> 'promoted'`, lead); n != 1 {
		t.Error("a reply converted the lead; only somebody working the lead converts it")
	}

	// The owner answers without closing the ask, the lead writes again, and a
	// meeting with them is held: the one ask stays the only one.
	l.mail(t, owedMail{from: l.seat, to: address, subject: "Re: Our offer", inReplyTo: offer, at: l.at.Add(3 * time.Minute)}, true)
	l.mail(t, owedMail{from: address, to: l.seat, subject: "Re: Our offer", inReplyTo: offer, at: l.at.Add(4 * time.Minute)}, true)
	store := l.e.Activities.WithLeadQualifyTasks(newLeadWorkTasks(InstallationDB(l.e.Pool), slog.Default()))
	at, held := l.at.Add(-time.Hour), meetingoutcome.Held
	meeting, _, err := store.LogActivity(l.e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: new("Demo"), Source: "manual",
		OccurredAt: &at, Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: lead.UUID}},
	})
	if err == nil {
		_, err = store.UpdateActivity(l.e.Admin(), ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), activities.UpdateActivityInput{MeetingStatus: &held})
	}
	if err != nil {
		t.Fatalf("holding a meeting with the lead: %v", err)
	}
	if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 1 {
		t.Errorf("a reply, a new message and a held meeting left %d open asks, want one", n)
	}
}

// One meeting held with two leads asks each lead's owner to qualify their own
// lead, so neither lead loses its ask to the other.
func TestAMeetingHeldWithTwoLeadsAsksAboutEach(t *testing.T) {
	l := setupLeadMail(t)
	first := l.ownedLead(t, "lena@prospect.example", l.e.Rep2)
	second := l.ownedLead(t, "mira@prospect.example", l.e.Rep2)
	store := l.e.Activities.WithLeadQualifyTasks(newLeadWorkTasks(InstallationDB(l.e.Pool), slog.Default()))
	at, held := l.at.Add(-time.Hour), meetingoutcome.Held
	if _, _, err := store.LogActivity(l.e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: new("Demo"), Source: "manual", OccurredAt: &at, MeetingStatus: &held,
		Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: first.UUID}, {EntityType: "lead", EntityID: second.UUID}},
	}); err != nil {
		t.Fatalf("holding a meeting with both leads: %v", err)
	}
	for name, lead := range map[string]ids.LeadID{"first": first, "second": second} {
		if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 1 {
			t.Errorf("the %s lead has %d open asks after the meeting, want one", name, n)
		}
	}
}
