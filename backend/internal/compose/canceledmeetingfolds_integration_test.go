// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A canceled meeting is no interaction, in every fold that asks when we last
// had one: the colleague and contact graph edges, deal engagement, the
// ghosted-thread scan, the account's newest message and the follow-up
// reconciler. Each meeting lands through the calendar Sink and is called off
// through its cancel path, the rows a live calendar pull produces.

import (
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// foldMeeting is one calendar meeting as the Sink lands it.
type foldMeeting struct {
	event     string
	at        time.Time
	direction string
	links     []datasource.EntityRef
	parties   []connector.MessageParticipant
}

func (m foldMeeting) capture(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	ref, err := calendarSink(e).Upsert(calendarOwnerCtx(e, e.AdminUser), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: calendarSystem, SourceID: m.event},
		Fields: capture.ActivityFields{
			Kind: "meeting", Subject: "Fold review", OccurredAt: m.at, Direction: m.direction,
		},
		Links:        m.links,
		Participants: m.parties,
		Source:       calendarSystem + ":" + m.event,
		CapturedBy:   "connector:" + calendarSystem,
		Raw:          []byte(`{"id":"` + m.event + `"}`),
	}.WithProviderAttestedParticipants(true))
	if err != nil {
		t.Fatalf("capturing meeting %s: %v", m.event, err)
	}
	return ref.ID
}

func (m foldMeeting) cancel(t *testing.T, e *integration.Env) {
	t.Helper()
	key := connector.NaturalKey{SourceSystem: calendarSystem, SourceID: m.event}
	if err := calendarSink(e).CancelMeeting(calendarOwnerCtx(e, e.AdminUser), key, m.at); err != nil {
		t.Fatalf("canceling meeting %s: %v", m.event, err)
	}
}

func contactLink(contact ids.UUID) []datasource.EntityRef {
	return []datasource.EntityRef{{Type: "contact", ID: contact}}
}

func contactWithEmail(t *testing.T, e *integration.Env, name, email string) ids.UUID {
	t.Helper()
	p, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.UUID(p.Id)
}

func employedAt(t *testing.T, e *integration.Env, company ids.UUID, name string) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, name, nil)
	contactID, companyID, primary := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](company), true
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, IsCurrentPrimary: &primary,
	}); err != nil {
		t.Fatalf("employing %s: %v", name, err)
	}
	return contact
}

func logMail(t *testing.T, e *integration.Env, contact ids.UUID, direction string, at time.Time) {
	t.Helper()
	subject := "Proposal"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}); err != nil {
		t.Fatalf("logging the %s mail: %v", direction, err)
	}
}

func inWorkspaceTx(t *testing.T, e *integration.Env, read func(tx pgx.Tx) error) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, read); err != nil {
		t.Fatal(err)
	}
}

func TestACanceledMeetingIsNoGraphEdgesLastInteraction(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now().UTC().Truncate(time.Second)
	dana := contactWithEmail(t, e, "Dana Fold", "dana@fold.example")
	eli := contactWithEmail(t, e, "Eli Fold", "eli@fold.example")
	parties := []connector.MessageParticipant{
		{Email: "dana@fold.example", Role: connector.ParticipantRoleAttendee},
		{Email: "eli@fold.example", Role: connector.ParticipantRoleAttendee},
	}
	held := foldMeeting{event: "evt-fold-held", at: now.AddDate(0, 0, -10), links: contactLink(dana), parties: parties}
	called := foldMeeting{event: "evt-fold-called", at: now.AddDate(0, 0, -2), links: contactLink(dana), parties: parties}
	touched := []ids.UUID{held.capture(t, e), called.capture(t, e)}
	called.cancel(t, e)

	assertLastAtHeld := func(path string) {
		t.Helper()
		if got := e.WsCount(t, `SELECT count(*) FROM graph_interaction_edge
		                         WHERE user_id = $1 AND contact_id = $2 AND last_at = $3`,
			e.AdminUser, dana, held.at); got != 1 {
			t.Errorf("%s: the colleague edge's last_at is not the held meeting — a canceled one is no interaction", path)
		}
		if got := e.WsCount(t, `SELECT count(*) FROM graph_contact_edge
		                         WHERE contact_a = least($1::uuid, $2::uuid) AND contact_b = greatest($1::uuid, $2::uuid)
		                           AND last_at = $3`,
			dana, eli, held.at); got != 1 {
			t.Errorf("%s: the contact edge's last_at is not the held meeting — a canceled one is no interaction", path)
		}
	}
	inWorkspaceTx(t, e, func(tx pgx.Tx) error { return search.RecomputeEdgesForActivities(e.Admin(), tx, touched) })
	assertLastAtHeld("incremental fold")
	inWorkspaceTx(t, e, func(tx pgx.Tx) error { return search.RebuildEdges(e.Admin(), tx) })
	assertLastAtHeld("rebuild")
}

func TestACanceledMeetingIsNoDealEngagement(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now().UTC()
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Fold deal", pipeline, open, nil))
	contact := e.SeedContact(t, "Sam Stake", nil)
	contactID, role := ids.From[ids.ContactKind](contact), "champion"
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "deal_stakeholder", ContactID: &contactID, DealID: &deal, Role: &role,
	}); err != nil {
		t.Fatalf("seating the stakeholder: %v", err)
	}
	logMail(t, e, contact, "inbound", now.AddDate(0, 0, -3))
	pitch := foldMeeting{event: "evt-fold-pitch", at: now.AddDate(0, 0, -2), direction: "outbound", links: contactLink(contact)}
	pitch.capture(t, e)

	engaged := func() bool {
		var found []ids.UUID
		inWorkspaceTx(t, e, func(tx pgx.Tx) error {
			var err error
			found, err = deals.EngagedStakeholders(e.Admin(), tx, deal, now)
			return err
		})
		return slices.Contains(found, contact)
	}
	if !engaged() {
		t.Fatal("their reply and our booked meeting do not engage the stakeholder — the case below proves nothing")
	}
	pitch.cancel(t, e)
	if engaged() {
		t.Error("a canceled meeting still counts as our side of the exchange")
	}
}

func TestACanceledMeetingIsNotTheGhostedScansNewestInteraction(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now().UTC()
	company := e.SeedCompany(t, "Fold Ghost Co", nil)
	e.WsExec(t, `UPDATE company SET lifecycle = 'opportunity' WHERE id = $1`, company)
	contact := employedAt(t, e, company, "Gale Ghost")
	foldMeeting{event: "evt-fold-theirs", at: now.AddDate(0, 0, -20), direction: "inbound", links: contactLink(contact)}.
		capture(t, e)
	ours := foldMeeting{event: "evt-fold-ours", at: now.AddDate(0, 0, -16), direction: "outbound", links: contactLink(contact)}
	ours.capture(t, e)

	ghosted := func() bool {
		var found []ghostedCandidate
		inWorkspaceTx(t, e, func(tx pgx.Tx) error {
			var err error
			found, err = scanGhostedThreads(e.Admin(), tx, now)
			return err
		})
		return slices.ContainsFunc(found, func(c ghostedCandidate) bool { return c.CompanyID == company })
	}
	if !ghosted() {
		t.Fatal("our unanswered meeting does not read as ghosted — the case below proves nothing")
	}
	ours.cancel(t, e)
	if ghosted() {
		t.Error("a canceled meeting is still the account's newest interaction, so the account reads as ghosted")
	}
}

func TestACanceledMeetingIsNotTheAccountsNewestMessage(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Fold Reply Co", nil)
	acct := quietAccount{company: ids.From[ids.CompanyKind](company), now: time.Now().UTC()}
	acct.contact = employedAt(t, e, company, "Rae Reply")
	logMail(t, e, acct.contact, "outbound", acct.now.AddDate(0, 0, -10))
	chat := foldMeeting{event: "evt-fold-chat", at: acct.now.AddDate(0, 0, -2), links: contactLink(acct.contact)}
	chat.capture(t, e)

	asksForAReply := func() bool {
		page := pageOf(t, e, acct)
		if page.Suggestions == nil {
			t.Fatal("the account page carries no suggestions section")
		}
		return slices.ContainsFunc(*page.Suggestions, func(s crmcontracts.Company360Suggestion) bool {
			return s.Kind == crmcontracts.Company360SuggestionKindNoReply
		})
	}
	if asksForAReply() {
		t.Fatal("a meeting after our mail still reads as no reply — the case below proves nothing")
	}
	chat.cancel(t, e)
	if !asksForAReply() {
		t.Error("a canceled meeting still answers our mail, so the no-reply suggestion never fires")
	}
}

func TestACanceledMeetingIsNoFollowUpEvidence(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Fold follow-up", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery call", 5)
	called := foldMeeting{
		event: "evt-fold-review", at: time.Now().UTC().Add(-2 * time.Hour),
		links: []datasource.EntityRef{{Type: "deal", ID: deal}},
	}
	called.capture(t, e.Env)
	called.cancel(t, e.Env)

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingFollowUps(t, deal); got != 1 {
		t.Fatalf("%d follow-ups staged, want 1 for the call", got)
	}
	if _, proposal := e.followUpApproval(t, deal); proposal.EvidenceKind != "call" {
		t.Errorf("the follow-up cites a %q, want the call — a canceled meeting left no conversation behind",
			proposal.EvidenceKind)
	}
}
