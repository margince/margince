// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Captured mail and meetings are filed under a live lead whose address is on
// them. The participant rule for contacts decides it, with its ceiling, order
// and admission. Filing never widens who may read the message.

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// leadMailEnv is one seat with a shared, connected mailbox whose connector may
// read leads, as a seat granted the lead object's read is.
type leadMailEnv struct {
	*owedEnv
	at time.Time
}

func setupLeadMail(t *testing.T) *leadMailEnv {
	t.Helper()
	// An hour ahead, so mail dated at l.at follows every lead the test creates,
	// whichever of the two clocks is ahead.
	return &leadMailEnv{owedEnv: setupOwed(t), at: clocktest.Now(t).Add(time.Hour)}
}

// connectorCtx is the seat's connector, with or without the lead read grant.
func (l *leadMailEnv) connectorCtx(t *testing.T, name string, readsLeads bool) context.Context {
	t.Helper()
	ctx := threadConnectorCtx(l.e, l.owner)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the connector context carries no actor")
	}
	actor.ID = "connector:" + name
	actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	actor.Permissions.Objects["project"] = principal.ObjectGrant{Read: true}
	if readsLeads {
		actor.Permissions.Objects["lead"] = principal.ObjectGrant{Read: true}
	} else {
		delete(actor.Permissions.Objects, "lead")
	}
	return principal.WithActor(ctx, actor)
}

// lead creates a live lead holding the address by hand.
func (l *leadMailEnv) lead(t *testing.T, address string) ids.LeadID {
	t.Helper()
	created, _, err := l.e.Contacts.CreateLead(l.e.Admin(), contacts.CreateLeadInput{
		FullName: new("Lena Prospect"), Email: new(address), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the lead for %s: %v", address, err)
	}
	return ids.From[ids.LeadKind](ids.UUID(created.Id))
}

// mail captures one message through the production sink as the seat's
// connector, readsLeads saying whether that connector holds the lead grant.
func (l *leadMailEnv) mail(t *testing.T, m owedMail, readsLeads bool) ids.UUID {
	t.Helper()
	if m.at.IsZero() {
		m.at = l.at
	}
	if m.messageID == "" {
		m.messageID = ids.NewV7().String() + "@mail.example"
	}
	parsed, err := mailmap.Parse(m.raw(), l.seat)
	if err != nil {
		t.Fatalf("parsing %s: %v", m.messageID, err)
	}
	parsed = parsed.AttestSentByOwner(m.from == l.seat)
	ref, err := newCaptureSink(l.e.Pool, CaptureConfig{}).Upsert(l.connectorCtx(t, "gmail", readsLeads), parsed.ToRecord("gmail", m.raw()))
	if err != nil {
		t.Fatalf("capturing %s: %v", m.messageID, err)
	}
	return ref.ID
}

// meeting captures one calendar event with the lead's address as an attendee.
func (l *leadMailEnv) meeting(t *testing.T, attendee string) ids.UUID {
	t.Helper()
	return l.meetingWith(t, "Discovery call", attendee)
}

// meetingWith captures one calendar event the seat called, with these attendees.
func (l *leadMailEnv) meetingWith(t *testing.T, subject string, attendees ...string) ids.UUID {
	t.Helper()
	event := meetingmap.Event{
		ID: "ev-" + ids.NewV7().String(), Subject: subject,
		StartsAt: l.at, EndsAt: l.at.Add(time.Hour),
		Organizer: meetingmap.Actor{Email: l.seat},
		Attendees: []meetingmap.Actor{{Email: l.seat}},
	}
	for _, attendee := range attendees {
		event.Attendees = append(event.Attendees, meetingmap.Actor{Email: attendee})
	}
	decode := func([]byte, string) (meetingmap.Event, error) { return event, nil }
	sink := newCaptureSink(l.e.Pool, CaptureConfig{})
	if err := meetingmap.CaptureOne(l.connectorCtx(t, "gcal", true), []byte(event.ID), sink, l.seat, "gcal", decode); err != nil {
		t.Fatalf("capturing the meeting: %v", err)
	}
	var id ids.UUID
	if err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT id FROM activity WHERE source_system = 'gcal' AND source_id = $1`, event.ID).Scan(&id)
	}); err != nil {
		t.Fatalf("reading the captured meeting: %v", err)
	}
	return id
}

func (l *leadMailEnv) filedUnderLead(t *testing.T, activity ids.UUID, lead ids.LeadID) bool {
	t.Helper()
	return l.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND lead_id = $2`, activity, lead) == 1
}

func TestCapturedMailAndMeetingsAreFiledUnderTheLeadOnThem(t *testing.T) {
	l := setupLeadMail(t)
	lead := l.lead(t, "lena@prospect.example")

	inbound := l.mail(t, owedMail{from: "Lena@Prospect.example", to: l.seat, subject: "Our requirements"}, true)
	outbound := l.mail(t, owedMail{from: l.seat, to: "lena@prospect.example", subject: "Re: Our requirements"}, true)
	meeting := l.meeting(t, "lena@prospect.example")
	for name, activity := range map[string]ids.UUID{"inbound mail": inbound, "outbound mail": outbound, "a meeting": meeting} {
		if !l.filedUnderLead(t, activity, lead) {
			t.Errorf("%s with the lead's address is not filed under the lead", name)
		}
	}
	if audience, _ := l.audienceOf(t, meeting); audience != "workspace" {
		t.Errorf("a meeting filed under its lead attendee is audience %q, want workspace: a filed meeting is not a link-less record", audience)
	}

	unread := l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "No grant"}, false)
	if l.filedUnderLead(t, unread, lead) {
		t.Error("a mailbox whose seat may not read leads filed mail under one")
	}
	near := l.mail(t, owedMail{from: "lena@prospect.example.org", to: l.seat, subject: "Lookalike"}, true)
	if l.filedUnderLead(t, near, lead) {
		t.Error("a near-miss address was filed under the lead; only an exact address names a lead")
	}
	suppressed := l.lead(t, "dse@eu.docusign.net")
	notice := l.mail(t, owedMail{from: "dse@eu.docusign.net", to: l.seat, subject: "Please sign"}, true)
	if l.filedUnderLead(t, notice, suppressed) {
		t.Error("a message the ladder suppressed was filed under a lead holding its sender's address")
	}
}

func TestARetiredLeadIsNoFilingTarget(t *testing.T) {
	l := setupLeadMail(t)
	promoted := l.lead(t, "pat@prospect.example")
	if _, _, err := l.e.Contacts.PromoteLead(l.e.Admin(), promoted, contacts.PromoteLeadInput{Trigger: "human_qualify"}); err != nil {
		t.Fatalf("promoting the lead: %v", err)
	}
	dropped := l.lead(t, "kim@prospect.example")
	if _, err := l.e.Contacts.DisqualifyLead(l.e.Admin(), dropped, contacts.DisqualifyLeadInput{Note: new("No budget")}); err != nil {
		t.Fatalf("disqualifying the lead: %v", err)
	}
	for address, lead := range map[string]ids.LeadID{"pat@prospect.example": promoted, "kim@prospect.example": dropped} {
		mail := l.mail(t, owedMail{from: address, to: l.seat, subject: "Following up"}, true)
		if l.filedUnderLead(t, mail, lead) {
			t.Errorf("mail from %s was filed under the retired lead that held the address", address)
		}
	}
}

// The ceiling is spent in one order for contacts and leads together: contacts
// first, so a lead is the record the cut drops.
func TestTheLinkCeilingFilesContactsBeforeLeads(t *testing.T) {
	l := setupLeadMail(t)
	lead := l.lead(t, "lena@prospect.example")
	copied := make([]string, 0, maxMailParticipantLinks)
	for i := range maxMailParticipantLinks {
		address := fmt.Sprintf("buyer%02d@customer.example", i)
		l.contact(t, fmt.Sprintf("Buyer %02d", i), address)
		copied = append(copied, address)
	}
	full := l.mail(t, owedMail{from: l.seat, to: "desk@customer.example", cc: strings.Join(append(copied, "lena@prospect.example"), ", "), subject: "Kickoff"}, true)
	if l.filedUnderLead(t, full, lead) {
		t.Error("the lead took a seat under the ceiling ahead of a contact")
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND entity_type = 'contact'`, full); n < maxMailParticipantLinks {
		t.Errorf("the message is filed under %d contacts, want all %d copied ones", n, maxMailParticipantLinks)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1`, full); n > 25 {
		t.Errorf("the message is filed under %d records, past the ceiling of 25", n)
	}
	room := l.mail(t, owedMail{from: l.seat, to: "desk@customer.example", cc: strings.Join(append(copied[1:], "lena@prospect.example"), ", "), subject: "Kickoff 2"}, true)
	if !l.filedUnderLead(t, room, lead) {
		t.Error("with a seat left under the ceiling the lead was still not filed")
	}
}

// maxMailParticipantLinks is the room a new message has for its participants:
// the ceiling of 25 less the three seats kept for the writers after capture.
const maxMailParticipantLinks = 22

func TestAHeldMailFiledUnderALeadStaysUnreadableThroughTheLead(t *testing.T) {
	l := setupLeadMail(t)
	lead := l.lead(t, "lena@prospect.example")
	held := l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "[Vertraulich] Terms", body: "Our walk-away price."}, true)
	if !l.filedUnderLead(t, held, lead) {
		t.Fatal("the held mail is not filed under the lead, so this test would prove nothing")
	}
	if audience, reason := l.audienceOf(t, held); audience != "participants" || reason != "explicitly_confidential" {
		t.Fatalf("the held mail is audience %q (%s) after filing, want participants: filing must not widen it", audience, reason)
	}
	colleague := l.e.As(l.e.Rep2, nil, integration.AdminPerms)
	timeline := l.leadTimeline(colleague, t, lead)
	if len(timeline) != 1 {
		t.Fatalf("the lead timeline lists %d rows for a colleague, want the held mail as one withheld row", len(timeline))
	}
	row := timeline[0]
	if row.ContentState == nil || *row.ContentState != crmcontracts.ActivityContentStateWithheld || row.Subject != nil || row.Body != nil {
		t.Errorf("a colleague outside the mail's audience reads it through the lead: %+v", row)
	}
	seat := l.e.As(l.owner, nil, integration.AdminPerms)
	if own := l.leadTimeline(seat, t, lead); len(own) != 1 || own[0].Subject == nil {
		t.Errorf("the seat that received the mail cannot read it through the lead: %+v", own)
	}
}

func (l *leadMailEnv) leadTimeline(ctx context.Context, t *testing.T, lead ids.LeadID) []crmcontracts.Activity {
	t.Helper()
	rows, _, err := l.e.Activities.ListActivities(ctx, activities.ListActivitiesInput{EntityType: new("lead"), EntityID: &lead.UUID})
	if err != nil {
		t.Fatalf("listing the lead timeline: %v", err)
	}
	return rows
}

func (l *leadMailEnv) audienceOf(t *testing.T, activity ids.UUID) (audience, reason string) {
	t.Helper()
	if err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT audience, coalesce(audience_reason, '') FROM activity WHERE id = $1`, activity).Scan(&audience, &reason)
	}); err != nil {
		t.Fatalf("reading the audience: %v", err)
	}
	return audience, reason
}

// A promoter outside a held mail's audience still previews the promotion: the
// held mail is counted as moving and never named.
func TestThePromotionPreviewCountsAnActivityTheReaderMayNotRead(t *testing.T) {
	l := setupLeadMail(t)
	lead := l.lead(t, "lena@prospect.example")
	held := l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "[Vertraulich] Terms"}, true)
	open := l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "Agenda"}, true)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
	in := contacts.PromoteLeadInput{Trigger: "human_qualify"}

	outside, err := l.e.Contacts.PreviewLeadContactChange(l.e.As(l.e.Rep2, nil, perms), lead, in)
	if err != nil {
		t.Fatalf("a promoter outside a held mail's audience cannot preview the promotion: %v", err)
	}
	if len(outside.Activities) != 1 || outside.Activities[0].ID != open || outside.WithheldActivities != 1 {
		t.Fatalf("the preview for an outsider names %+v and withholds %d, want the open mail named and the held one counted",
			outside.Activities, outside.WithheldActivities)
	}
	inside, err := l.e.Contacts.PreviewLeadContactChange(l.e.As(l.owner, nil, perms), lead, in)
	if err != nil || len(inside.Activities) != 2 || inside.WithheldActivities != 0 {
		t.Fatalf("the preview for the mail's own seat = %+v withheld %d, %v; want both mails named", inside.Activities, inside.WithheldActivities, err)
	}
	if _, _, err := l.e.Contacts.PromoteLead(l.e.Admin(), lead, in); err != nil {
		t.Fatalf("promoting: %v", err)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = ANY($1) AND entity_type = 'contact'`, []ids.UUID{held, open}); n != 2 {
		t.Errorf("after promotion %d of the two mails sit on the contact, want both, the withheld one included", n)
	}
}
