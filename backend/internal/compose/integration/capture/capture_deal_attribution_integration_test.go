// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The deal rule end to end: a captured mail goes on the one deal it is about,
// on no deal when several fit, and a human's choice is never undone. Companies,
// contacts, employment, stakeholders, projects and deals are written by their
// own stores; mail arrives through the production sink.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mailDay is the Date every fixture mail carries, so last_activity_at has one
// known answer.
var mailDay = time.Date(2026, 6, 4, 8, 0, 0, 0, time.UTC)

// dealWorld is one customer company with its people, seeded through the
// stores that own each table.
type dealWorld struct {
	projectSeeder
	contacts *contacts.Store
	customer ids.UUID
}

func newDealWorld(t *testing.T, e *integration.SearchEnv) dealWorld {
	t.Helper()
	seed := newProjectSeeder(t, e)
	seed.ctx = principal.WithActor(seed.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"project":      {Create: true, Read: true, Update: true, Delete: true},
				"deal":         {Create: true, Read: true, Update: true},
				"company":      {Create: true, Read: true, Update: true},
				"contact":      {Create: true, Read: true, Update: true},
				"relationship": {Create: true, Read: true, Update: true},
				"activity":     {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	// Who works where, and who is a stakeholder, are edges, and the rule reads
	// them with the capturing seat's own edge grant, as a rep's role has it.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE role
			   SET permissions = jsonb_set(permissions, '{objects,relationship}', '{"read":true}')
			 WHERE key = 'capture_rep'`)
		return err
	}); err != nil {
		t.Fatalf("granting the capture role the edge read: %v", err)
	}
	w := dealWorld{projectSeeder: seed, contacts: contacts.NewStore(e.DB())}
	w.customer = w.company(t, "Acme")
	seed.companyID = w.customer
	w.projectSeeder = seed
	return w
}

// at is the same world with another customer company, whose deals the
// fixtures below then create.
func (w dealWorld) at(t *testing.T, name string) dealWorld {
	t.Helper()
	w.customer = w.company(t, name)
	w.companyID = w.customer
	return w
}

func (w dealWorld) company(t *testing.T, name string) ids.UUID {
	t.Helper()
	created, err := w.contacts.CreateCompany(w.ctx, contacts.CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatalf("creating company %q: %v", name, err)
	}
	return ids.UUID(created.Id)
}

// person creates a contact with one address, employed at company.
func (w dealWorld) person(t *testing.T, address string, company ids.UUID) ids.UUID {
	t.Helper()
	created, err := w.contacts.CreateContact(w.ctx, contacts.CreateContactInput{
		FullName: strings.Split(address, "@")[0], Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating contact %s: %v", address, err)
	}
	contact, companyID := ids.From[ids.ContactKind](ids.UUID(created.Id)), ids.From[ids.CompanyKind](company)
	if _, err := w.contacts.CreateRelationship(w.ctx, contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contact, CompanyID: &companyID, IsCurrentPrimary: new(true), Source: "manual",
	}); err != nil {
		t.Fatalf("employing %s: %v", address, err)
	}
	return contact.UUID
}

func (w dealWorld) stakeholder(t *testing.T, contactID, dealID ids.UUID) {
	t.Helper()
	contact, deal := ids.From[ids.ContactKind](contactID), ids.From[ids.DealKind](dealID)
	if _, err := w.contacts.CreateRelationship(w.ctx, contacts.CreateRelationshipInput{
		Kind: "deal_stakeholder", ContactID: &contact, DealID: &deal, Source: "manual",
	}); err != nil {
		t.Fatalf("making %s a stakeholder on %s: %v", contactID, dealID, err)
	}
}

// shareMailbox opens the connected mailbox to the workspace through the real
// settings, so captured mail is workspace-readable and moves the deal's clock.
func shareMailbox(t *testing.T, env captureEnv) {
	t.Helper()
	e, on := env.e, true
	admin := principal.WithCorrelationID(principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1, SeatType: principal.SeatFull,
			Permissions: principal.Permissions{
				Objects:  map[string]principal.ObjectGrant{"capture_settings": {Read: true, Update: true}},
				RowScope: principal.RowScopeAll,
			},
		}), ids.NewV7())
	settings := capturemod.NewSettings(compose.NewSettingsStore(e.Pool), compose.StoredConfig(e.Pool))
	if _, err := settings.Update(admin, capturemod.SettingsPatch{SharedPostureAllowed: &on}); err != nil {
		t.Fatalf("allowing the shared posture: %v", err)
	}
	owner := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := env.registry.SetMailPosture(owner, "gmail", capturemod.PostureShared, false); err != nil {
		t.Fatalf("putting the mailbox in shared: %v", err)
	}
}

// lastActivity is the deal's clock.
func lastActivity(t *testing.T, e *integration.SearchEnv, deal ids.UUID) *time.Time {
	t.Helper()
	var last *time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT last_activity_at FROM deal WHERE id = $1`, deal).Scan(&last)
	}); err != nil {
		t.Fatalf("reading the deal's last activity: %v", err)
	}
	return last
}

// emailAbout2 is one mail with the sender, recipient and subject a case needs.
func emailAbout2(from, to, msgID, refs, subject string) []byte {
	raw := string(mailFrom(from, msgID, refs, subject))
	return []byte(strings.Replace(raw, "To: "+captureOwner, "To: "+to, 1))
}

// mailFrom is one inbound mail from a customer address to the mailbox owner.
func mailFrom(from, msgID, refs, subject string, cc ...string) []byte {
	lines := []string{"From: " + from, "To: " + captureOwner}
	if len(cc) > 0 {
		lines = append(lines, "Cc: "+strings.Join(cc, ", "))
	}
	lines = append(lines, "Subject: "+subject, "Date: Thu, 04 Jun 2026 08:00:00 +0000", "Message-ID: <"+msgID+">")
	if refs != "" {
		lines = append(lines, "References: <"+refs+">")
	}
	lines = append(lines, "Content-Type: text/plain", "", "hello", "")
	return []byte(strings.Join(lines, "\r\n"))
}

// dealsOf answers the deal links of one captured message, in id order.
func dealsOf(t *testing.T, e *integration.SearchEnv, sourceID string) []ids.UUID {
	t.Helper()
	var found []ids.UUID
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT l.deal_id FROM activity a
			  JOIN activity_link l ON l.activity_id = a.id AND l.entity_type = 'deal'
			 WHERE a.source_id = $1 ORDER BY l.deal_id`, sourceID)
		if err != nil {
			return err
		}
		found, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	if err != nil {
		t.Fatalf("reading the deal links of %s: %v", sourceID, err)
	}
	return found
}

func activityOf(t *testing.T, e *integration.SearchEnv, sourceID string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT id FROM activity WHERE source_id = $1`, sourceID).Scan(&id)
	}); err != nil {
		t.Fatalf("reading the activity of %s: %v", sourceID, err)
	}
	return id
}

// whichDealQuestions counts the disambiguation tasks, and who holds them.
func whichDealQuestions(t *testing.T, e *integration.SearchEnv) (int, []ids.UUID) {
	t.Helper()
	var holders []ids.UUID
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT assignee_id FROM activity WHERE kind = 'task' AND source_system = 'which_deal' ORDER BY id`)
		if err != nil {
			return err
		}
		holders, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	if err != nil {
		t.Fatalf("reading the deal questions: %v", err)
	}
	return len(holders), holders
}

func wantDeals(t *testing.T, e *integration.SearchEnv, sourceID string, want ...ids.UUID) {
	t.Helper()
	got := dealsOf(t, e, sourceID)
	if len(got) != len(want) {
		t.Fatalf("%s is on deals %v, want %v", sourceID, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s is on deals %v, want %v", sourceID, got, want)
		}
	}
}

// One customer with one open deal: inbound and outbound mail both land on it,
// the deal's last activity moves to the mail, and each link is audited as the
// connector's, which is what tells it from a human relink.
func TestCaptureFilesMailOnTheCustomersOnlyOpenDeal(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	shareMailbox(t, env)
	w := newDealWorld(t, e)
	w.person(t, "alice@acme.example", w.customer)
	deal := w.deal(t, "Acme rollout")

	env.syncSent(t, map[string]bool{"out1@myco.example": true},
		mailFrom("alice@acme.example", "in1@acme.example", "", "status"),
		email(captureOwner, "", "alice@acme.example", "out1@myco.example", "in1@acme.example"),
	)

	wantDeals(t, e, "in1@acme.example", deal)
	wantDeals(t, e, "out1@myco.example", deal)
	if n := countRows(t, e, `
		SELECT count(*) FROM audit_log
		 WHERE action = 'activity_relink' AND entity_type = 'activity'
		   AND after->>'entity_type' = 'deal' AND actor_type = 'connector'`); n != 2 {
		t.Fatalf("%d connector deal-link audit rows, want one per mail", n)
	}
	if last := lastActivity(t, e, deal); last == nil || !last.Equal(mailDay) {
		t.Fatalf("deal last_activity_at = %v, want the mail's time %v", last, mailDay)
	}
	if n, _ := whichDealQuestions(t, e); n != 0 {
		t.Fatalf("%d deal questions for a mail with one deal, want none", n)
	}
}

// Mail only its participants may read goes on the deal's timeline, but does
// not move the deal's clock, which counts workspace mail alone.
func TestCaptureFilesPrivateMailWithoutMovingTheDealsClock(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.person(t, "alice@acme.example", w.customer)
	deal := w.deal(t, "Acme rollout")

	env.sync(t, mailFrom("alice@acme.example", "priv1@acme.example", "", "status"))

	wantDeals(t, e, "priv1@acme.example", deal)
	if last := lastActivity(t, e, deal); last != nil {
		t.Fatalf("private mail moved the deal's last_activity_at to %v", last)
	}
}

// Two open deals and nothing that ties the thread to one: neither mail goes on
// a deal, and the colleague who wrote the first is asked once for the thread.
// A stakeholder on exactly one of the deals is the positive control.
func TestCaptureAsksOnceWhenSeveralOpenDealsFitAMail(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.person(t, "alice@acme.example", w.customer)
	bob := w.person(t, "bob@acme.example", w.customer)
	w.deal(t, "Acme operations")
	reporting := w.deal(t, "Acme reporting")
	w.stakeholder(t, bob, reporting)

	env.syncSent(t, map[string]bool{"amb1@myco.example": true},
		email(captureOwner, "", "alice@acme.example", "amb1@myco.example", ""))
	env.sync(t, mailFrom("alice@acme.example", "amb2@acme.example", "amb1@myco.example", "Re: next steps"))
	env.sync(t, mailFrom("bob@acme.example", "staked@acme.example", "", "dashboards"))

	wantDeals(t, e, "amb1@myco.example")
	wantDeals(t, e, "amb2@acme.example")
	wantDeals(t, e, "staked@acme.example", reporting)
	n, holders := whichDealQuestions(t, e)
	if n != 1 {
		t.Fatalf("%d deal questions for one ambiguous thread, want exactly 1", n)
	}
	if holders[0] != e.Rep1 {
		t.Fatalf("the deal question went to %s, want the colleague who wrote the mail (%s)", holders[0], e.Rep1)
	}
}

// Nobody the mail names is responsible: a customer writes to a colleague
// outside this installation with our mailbox only on Cc. The mail stays on its
// company and contacts, and nobody is asked.
func TestCaptureAsksNobodyWhenNoColleagueIsResponsible(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.person(t, "alice@acme.example", w.customer)
	w.person(t, "carol@acme.example", w.customer)
	w.deal(t, "Acme operations")
	w.deal(t, "Acme reporting")

	raw := strings.Join([]string{
		"From: alice@acme.example", "To: carol@acme.example", "Cc: " + captureOwner,
		"Subject: internal", "Date: Thu, 04 Jun 2026 08:00:00 +0000", "Message-ID: <cc1@acme.example>",
		"Content-Type: text/plain", "", "hello", "",
	}, "\r\n")
	env.sync(t, []byte(raw))

	wantDeals(t, e, "cc1@acme.example")
	if n, _ := whichDealQuestions(t, e); n != 0 {
		t.Fatalf("%d deal questions with no responsible colleague, want none", n)
	}
}

// The project comes first. A mail naming a project with one open deal goes on
// that deal, not on the company's other open deal. A project with no open deal
// files nothing even though the company has exactly one, and a project with
// two asks.
func TestCaptureFilesMailOnItsProjectsDealFirst(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.person(t, "alice@acme.example", w.customer)
	crm := w.project(t, "CRM project")
	crmDeal := w.dealOnProject(t, "CRM licences", crm.ID)
	w.deal(t, "Acme operations")
	env.sync(t, mailFrom("alice@acme.example", "pj1@acme.example", "", "["+crm.Key+"] kickoff"))
	wantDeals(t, e, "pj1@acme.example", crmDeal)

	empty := w.project(t, "Audit")
	idle := w.at(t, "Idle")
	idle.person(t, "dora@idle.example", idle.customer)
	idle.deal(t, "Idle renewal")
	env.sync(t, mailFrom("dora@idle.example", "pj2@idle.example", "", "["+empty.Key+"] scope"))
	wantDeals(t, e, "pj2@idle.example")

	twin := w.project(t, "Twin")
	w.dealOnProject(t, "Twin phase one", twin.ID)
	w.dealOnProject(t, "Twin phase two", twin.ID)
	env.syncSent(t, map[string]bool{"pj3@myco.example": true},
		emailAbout2(captureOwner, "alice@acme.example", "pj3@myco.example", "", "["+twin.Key+"] which phase"))
	wantDeals(t, e, "pj3@myco.example")
	if n, _ := whichDealQuestions(t, e); n != 1 {
		t.Fatalf("%d deal questions, want exactly 1 for the project with two deals", n)
	}
}

// A human's relink always wins: capturing the mail again leaves it on the deal
// the human chose.
func TestCaptureNeverUndoesAHumanDealRelink(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.person(t, "alice@acme.example", w.customer)
	only := w.deal(t, "Acme rollout")
	other := w.at(t, "Elsewhere").deal(t, "Elsewhere")

	env.sync(t, mailFrom("alice@acme.example", "hr1@acme.example", "", "status"))
	wantDeals(t, e, "hr1@acme.example", only)

	mail := ids.From[ids.ActivityKind](activityOf(t, e, "hr1@acme.example"))
	if _, err := activities.NewStore(e.DB()).RelinkActivity(w.ctx, mail, activities.RelinkActivityInput{
		EntityType: "deal", EntityID: other, ReplaceExistingOfType: true,
	}); err != nil {
		t.Fatalf("relinking as a human: %v", err)
	}
	env.sync(t, mailFrom("alice@acme.example", "hr1@acme.example", "", "status"))
	wantDeals(t, e, "hr1@acme.example", other)
}

// Being copied with someone from a second company is conflicting evidence: the
// mail goes on no deal and nobody is asked.
func TestCaptureFilesNothingWhenTheMailNamesTwoCompanies(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.person(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme rollout")
	partner := w.company(t, "Partner")
	w.person(t, "pat@partner.example", partner)

	env.sync(t, mailFrom("alice@acme.example", "two1@acme.example", "", "intro", "pat@partner.example"))
	wantDeals(t, e, "two1@acme.example")
	if n, _ := whichDealQuestions(t, e); n != 0 {
		t.Fatalf("%d deal questions for conflicting companies, want none", n)
	}
}

// A sender types the thread they claim to reply to. A reply from our customer
// in a thread we sent and a colleague filed on one deal goes on that deal; the
// same claim from someone at another company, or from nobody we know, does not.
func TestCaptureFollowsOurThreadOnlyForTheSameCompany(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.person(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme operations")
	reporting := w.deal(t, "Acme reporting")
	evil := w.company(t, "Evil")
	w.person(t, "mallory@evil.example", evil)

	env.syncSent(t, map[string]bool{"thr1@myco.example": true},
		email(captureOwner, "", "alice@acme.example", "thr1@myco.example", ""))
	wantDeals(t, e, "thr1@myco.example")
	ours := ids.From[ids.ActivityKind](activityOf(t, e, "thr1@myco.example"))
	if _, err := activities.NewStore(e.DB()).RelinkActivity(w.ctx, ours, activities.RelinkActivityInput{
		EntityType: "deal", EntityID: reporting,
	}); err != nil {
		t.Fatalf("filing our mail on a deal as a human: %v", err)
	}

	env.sync(t,
		mailFrom("alice@acme.example", "thr2@acme.example", "thr1@myco.example", "Re: project"),
		mailFrom("mallory@evil.example", "thr3@evil.example", "thr1@myco.example", "Re: project"),
		mailFrom("stranger@nowhere.example", "thr4@nowhere.example", "thr1@myco.example", "Re: project"),
	)
	wantDeals(t, e, "thr2@acme.example", reporting)
	wantDeals(t, e, "thr3@evil.example")
	wantDeals(t, e, "thr4@nowhere.example")
}

// The colleague asked is one the mailbox confirms. A To header naming another
// colleague's address is typed by the sender and hands that colleague nothing.
func TestCaptureAsksNoColleagueTheSenderNamed(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.person(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme operations")
	w.deal(t, "Acme reporting")

	env.sync(t, emailAbout2("alice@acme.example", captureOwner+", rep2@search.test", "named1@acme.example", "", "next steps"))

	wantDeals(t, e, "named1@acme.example")
	if n, holders := whichDealQuestions(t, e); n != 0 {
		t.Fatalf("a To header the sender typed handed the deal question to %v", holders)
	}
}
