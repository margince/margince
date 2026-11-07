// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The deal rule end to end. A captured mail goes on the one deal it is about,
// or on none when several fit. A human's choice is never undone. Each
// record is written by its own store; mail arrives through the production sink.

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// mailDay is the Date every fixture mail carries, so last_activity_at has one
// known answer.
var mailDay = time.Date(2026, 6, 4, 8, 0, 0, 0, time.UTC)

// dealWorld is one customer company with its contacts, seeded through the
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
	// Employment and stakeholders are edges. The rule reads them with the
	// capturing seat's own edge grant, as a rep's role has it.
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

// employee creates a contact with one address, employed at company.
func (w dealWorld) employee(t *testing.T, address string, company ids.UUID) ids.UUID {
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

// authenticated adds the receiving server's verdict that the From domain
// passed DMARC, as Gmail stamps it.
func authenticated(raw []byte, domain string) []byte {
	return append([]byte("Authentication-Results: mx.google.com; dmarc=pass header.from="+domain+"\r\n"), raw...)
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
// and the deal's last activity moves to the mail. Each link is audited as the
// connector's, which tells it from a human relink.
func TestCaptureFilesMailOnTheCustomersOnlyOpenDeal(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	shareMailbox(t, env)
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
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
	w.employee(t, "alice@acme.example", w.customer)
	deal := w.deal(t, "Acme rollout")

	env.sync(t, mailFrom("alice@acme.example", "priv1@acme.example", "", "status"))

	wantDeals(t, e, "priv1@acme.example", deal)
	if last := lastActivity(t, e, deal); last != nil {
		t.Fatalf("private mail moved the deal's last_activity_at to %v", last)
	}
}

// Two open deals and nothing that ties the thread to one: neither mail goes on
// a deal. The colleague who wrote the first is asked once for the thread. A
// stakeholder on only one of the deals is the positive control.
func TestCaptureAsksOnceWhenSeveralOpenDealsFitAMail(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	bob := w.employee(t, "bob@acme.example", w.customer)
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
	w.employee(t, "alice@acme.example", w.customer)
	w.employee(t, "carol@acme.example", w.customer)
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
// files nothing even though the company has one, and a project with two asks.
func TestCaptureFilesMailOnItsProjectsDealFirst(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	crm := w.project(t, "CRM project")
	crmDeal := w.dealOnProject(t, "CRM licences", crm.ID)
	w.deal(t, "Acme operations")
	env.sync(t, mailFrom("alice@acme.example", "pj1@acme.example", "", "["+crm.Key+"] kickoff"))
	wantDeals(t, e, "pj1@acme.example", crmDeal)

	empty := w.project(t, "Audit")
	idle := w.at(t, "Idle")
	idle.employee(t, "dora@idle.example", idle.customer)
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
	w.employee(t, "alice@acme.example", w.customer)
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
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme rollout")
	partner := w.company(t, "Partner")
	w.employee(t, "pat@partner.example", partner)

	env.sync(t, mailFrom("alice@acme.example", "two1@acme.example", "", "intro", "pat@partner.example"))
	wantDeals(t, e, "two1@acme.example")
	if n, _ := whichDealQuestions(t, e); n != 0 {
		t.Fatalf("%d deal questions for conflicting companies, want none", n)
	}
}

// A sender types the thread they claim to reply to. Our customer's reply in a
// thread we sent and filed on one deal goes on that deal. The same claim from
// another company, or from nobody we know, does not.
func TestCaptureFollowsOurThreadOnlyForTheSameCompany(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme operations")
	reporting := w.deal(t, "Acme reporting")
	evil := w.company(t, "Evil")
	w.employee(t, "mallory@evil.example", evil)

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
		authenticated(mailFrom("alice@acme.example", "thr2@acme.example", "thr1@myco.example", "Re: project"), "acme.example"),
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
	// Workspace mail, so every colleague may open it and only the evidence
	// rule stands between the named address and the question.
	shareMailbox(t, env)
	// The named colleague is a working seat who could hold the task.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO role_assignment (role_id, user_id)
			SELECT id, $1 FROM role WHERE key = 'capture_rep'`, e.Rep3)
		return err
	}); err != nil {
		t.Fatalf("giving the named colleague a role: %v", err)
	}
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme operations")
	w.deal(t, "Acme reporting")

	env.sync(t, emailAbout2("alice@acme.example", captureOwner+", rep1@search.test", "named1@acme.example", "", "next steps"))

	wantDeals(t, e, "named1@acme.example")
	if n, holders := whichDealQuestions(t, e); n != 0 {
		t.Fatalf("a To header the sender typed handed the deal question to %v", holders)
	}
}

// Mail that reads as ours without the provider attesting it does not prove we
// wrote it. Its deal is no clue for a reply, and it names nobody to ask.
func TestCaptureCountsOnlyAttestedMailAsOurs(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme operations")
	reporting := w.deal(t, "Acme reporting")

	env.sync(t, email(captureOwner, "", "alice@acme.example", "una1@myco.example", ""))
	if n := countRows(t, e, `SELECT count(*) FROM activity
		 WHERE source_id = 'una1@myco.example' AND direction = 'outbound' AND NOT counterparty_outbound_attested
		   AND sent_by_colleague IS NULL`); n != 1 {
		t.Fatal("the fixture is not unattested outbound mail, so this case proves nothing")
	}
	wantDeals(t, e, "una1@myco.example")
	if n, holders := whichDealQuestions(t, e); n != 0 {
		t.Fatalf("unattested outbound mail handed the deal question to %v", holders)
	}
	ours := ids.From[ids.ActivityKind](activityOf(t, e, "una1@myco.example"))
	if _, err := activities.NewStore(e.DB()).RelinkActivity(w.ctx, ours, activities.RelinkActivityInput{
		EntityType: "deal", EntityID: reporting,
	}); err != nil {
		t.Fatalf("filing the mail on a deal as a human: %v", err)
	}
	env.sync(t, authenticated(mailFrom("alice@acme.example", "una2@acme.example", "una1@myco.example", "Re: project"), "acme.example"))
	wantDeals(t, e, "una2@acme.example")
}

// secondSentMailbox connects user's own mailbox, whose syncs may carry the
// provider's sent attestation and a connector's deal refs.
func secondSentMailbox(t *testing.T, e *integration.SearchEnv, user ids.UUID) func(*testing.T, map[string]bool, map[string][]ids.UUID, ...[]byte) {
	t.Helper()
	seedCaptureRoleFor(t, e, user)
	conn := &mailBatchConnector{accountLabel: secondSeatAddress}
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{Now: func() time.Time { return captureNow }})
	registry.Register(conn)
	connID, err := registry.Connect(humanWithScopes(e, user, []principal.Scope{principal.ScopeRead}), "gmail", connector.Auth("refresh"))
	if err != nil {
		t.Fatalf("connecting the second mailbox: %v", err)
	}
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	return func(t *testing.T, sent map[string]bool, filed map[string][]ids.UUID, raws ...[]byte) {
		t.Helper()
		conn.raws, conn.sent, conn.deals, conn.kinds = raws, sent, filed, nil
		if err := registry.SyncOnce(wsCtx, connID); err != nil {
			t.Fatalf("second mailbox SyncOnce: %v", err)
		}
	}
}

// repairer is the deal repair as the data task runs it: system principal,
// deciding each mail as the mailbox that captured it.
type repairer struct {
	e      *integration.SearchEnv
	repair capturemod.DealRepair
	system context.Context
}

func newRepairer(e *integration.SearchEnv, w dealWorld) repairer {
	return repairer{
		e:      e,
		repair: capturemod.DealRepair{Candidates: w.store, Authority: identity.NewService(e.Pool), Filer: compose.ActivityDealFiler{}},
		system: principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:data-task"),
	}
}

// preview reads the repair's outcomes through the operator preview the API
// serves, as an operator with the data task read grant.
func (r repairer) preview(t *testing.T) map[capturemod.DealOutcome]int {
	t.Helper()
	rec := r.operatorPreview(`{"limit":1000}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("the operator preview answered %d: %s", rec.Code, rec.Body.String())
	}
	var page crmcontracts.MailDealRepairPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("reading the operator preview: %v", err)
	}
	if !page.Complete {
		t.Fatalf("the operator preview says more mail follows a page of 1000: %+v", page)
	}
	counts := map[capturemod.DealOutcome]int{}
	for _, row := range page.Outcomes {
		counts[capturemod.DealOutcome(row.Outcome)] = row.Count
	}
	return counts
}

// operatorPreview posts body to the operator preview as the API serves it,
// as an operator holding the data task read grant.
func (r repairer) operatorPreview(body string) *httptest.ResponseRecorder {
	operator := principal.WithActor(principal.WithWorkspaceID(context.Background(), r.e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + r.e.Rep1.String(), UserID: r.e.Rep1, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{"data_task": {Read: true}}, RowScope: principal.RowScopeAll,
		},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/mail-deal-repair/preview", strings.NewReader(body))
	compose.MailDealRepairPreviewHandler(r.e.Pool)(rec, req.WithContext(operator))
	return rec
}

// runErr is run for a goroutine, which may not fail the test itself.
func (r repairer) runErr() (int, error) {
	var out datatask.BatchOut
	err := database.WithWorkspaceTx(r.system, r.e.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = r.repair.BatchTx(r.system, tx, datatask.BatchIn{Limit: 50, RunID: ids.NewV7()})
		return err
	})
	return out.Changed, err
}

func (r repairer) run(t *testing.T) int {
	t.Helper()
	var out datatask.BatchOut
	if err := database.WithWorkspaceTx(r.system, r.e.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = r.repair.BatchTx(r.system, tx, datatask.BatchIn{Limit: 50, RunID: ids.NewV7()})
		return err
	}); err != nil {
		t.Fatalf("running the repair: %v", err)
	}
	return out.Changed
}

// Mail captured before its deal existed is filed by the repair where capture
// would file it today. The preview says what the run will do. The run links
// only where one deal fits, leaves a human's link alone and asks nobody. A
// second run changes nothing.
func TestTheDealRepairFilesOldMailOnceAndLeavesHumansAlone(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	beta := w.at(t, "Beta")
	beta.employee(t, "bob@beta.example", beta.customer)
	env.sync(t,
		mailFrom("alice@acme.example", "old1@acme.example", "", "status"),
		mailFrom("bob@beta.example", "old2@beta.example", "", "status"),
		mailFrom("alice@acme.example", "old3@acme.example", "", "other"),
	)
	only := w.deal(t, "Acme rollout")
	beta.deal(t, "Beta one")
	beta.deal(t, "Beta two")
	elsewhere := w.at(t, "Elsewhere").deal(t, "Elsewhere")
	human := ids.From[ids.ActivityKind](activityOf(t, e, "old3@acme.example"))
	if _, err := activities.NewStore(e.DB()).RelinkActivity(w.ctx, human, activities.RelinkActivityInput{
		EntityType: "deal", EntityID: elsewhere,
	}); err != nil {
		t.Fatalf("filing a mail as a human: %v", err)
	}
	r := newRepairer(e, w)

	preview := r.preview(t)
	want := map[capturemod.DealOutcome]int{capturemod.DealLinked: 1, capturemod.DealAmbiguous: 1, capturemod.DealHumanKept: 1}
	if !maps.Equal(preview, want) {
		t.Fatalf("the preview reads %v, want %v", preview, want)
	}
	wantDeals(t, e, "old1@acme.example")
	if applied := r.run(t); applied != preview[capturemod.DealLinked] {
		t.Fatalf("the repair linked %d mails, the preview said %d", applied, preview[capturemod.DealLinked])
	}
	wantDeals(t, e, "old1@acme.example", only)
	wantDeals(t, e, "old2@beta.example")
	wantDeals(t, e, "old3@acme.example", elsewhere)
	if n, _ := whichDealQuestions(t, e); n != 0 {
		t.Fatalf("the repair raised %d deal questions, want none", n)
	}
	if again := r.run(t); again != 0 {
		t.Fatalf("a second repair linked %d more mails, want none", again)
	}
}

// The repair decides as the mailbox that captured the mail, never with the
// system's reach. A reply whose only filed sibling is a colleague's private
// sent mail stays on no deal. A reply beside our own filed mail goes on its
// deal. Live capture and the repair agree on both.
func TestTheDealRepairSeesOnlyWhatTheCapturingMailboxSaw(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	w.employee(t, "alice@acme.example", w.customer)
	w.deal(t, "Acme operations")
	reporting := w.deal(t, "Acme reporting")
	colleague := secondSentMailbox(t, e, e.Rep3)

	colleague(t, map[string]bool{"priv1@myco.example": true}, map[string][]ids.UUID{"priv1@myco.example": {reporting}},
		email(secondSeatAddress, "", "alice@acme.example", "priv1@myco.example", ""))
	env.sync(t, authenticated(mailFrom("alice@acme.example", "priv2@acme.example", "priv1@myco.example", "Re: project"), "acme.example"))
	wantDeals(t, e, "priv2@acme.example")

	// Our own thread at a second customer, captured before its deals existed,
	// so no question was asked. Then a colleague files our mail on one deal.
	beta := w.at(t, "Beta")
	beta.employee(t, "bob@beta.example", beta.customer)
	env.syncSent(t, map[string]bool{"own1@myco.example": true},
		email(captureOwner, "", "bob@beta.example", "own1@myco.example", ""))
	env.sync(t, authenticated(mailFrom("bob@beta.example", "own2@beta.example", "own1@myco.example", "Re: project"), "beta.example"))
	beta.deal(t, "Beta one")
	betaTwo := beta.deal(t, "Beta two")
	ours := ids.From[ids.ActivityKind](activityOf(t, e, "own1@myco.example"))
	if _, err := activities.NewStore(e.DB()).RelinkActivity(w.ctx, ours, activities.RelinkActivityInput{
		EntityType: "deal", EntityID: betaTwo,
	}); err != nil {
		t.Fatalf("filing our mail on a deal as a human: %v", err)
	}
	wantDeals(t, e, "own2@beta.example")

	r := newRepairer(e, w)
	if applied := r.run(t); applied != 1 {
		t.Fatalf("the repair linked %d mails, want only the reply beside our own filed mail", applied)
	}
	wantDeals(t, e, "priv2@acme.example")
	wantDeals(t, e, "own2@beta.example", betaTwo)

	// Live capture decides the same way for the same mail.
	env.sync(t,
		authenticated(mailFrom("alice@acme.example", "priv2@acme.example", "priv1@myco.example", "Re: project"), "acme.example"),
		authenticated(mailFrom("bob@beta.example", "own3@beta.example", "own1@myco.example", "Re: project"), "beta.example"))
	wantDeals(t, e, "priv2@acme.example")
	wantDeals(t, e, "own3@beta.example", betaTwo)
}

// openQuestions reads the deal questions still open, and whether each is done.
func openQuestions(t *testing.T, e *integration.SearchEnv) int {
	t.Helper()
	return countRows(t, e, `SELECT count(*) FROM activity WHERE source_system = 'which_deal' AND NOT is_done AND archived_at IS NULL`)
}

// A colleague answers the question by filing one mail of the conversation on a
// deal. The rest of the conversation follows, its promise shows on that deal's
// timeline and not on the other deal's, and the question is done. A mail of
// the conversation already filed elsewhere keeps its deal.
func TestFilingOneMailAnswersTheDealQuestionForItsConversation(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	alice := w.employee(t, "alice@acme.example", w.customer)
	operations := w.deal(t, "Acme operations")
	reporting := w.deal(t, "Acme reporting")

	env.syncSent(t, map[string]bool{"ans1@myco.example": true},
		email(captureOwner, "", "alice@acme.example", "ans1@myco.example", ""))
	env.sync(t,
		mailFrom("alice@acme.example", "ans2@acme.example", "ans1@myco.example", "Re: project"),
		authenticated(mailFrom("alice@acme.example", "ans4@acme.example", "ans1@myco.example", "Re: project"), "acme.example"),
		mailFrom("alice@acme.example", "ans5@acme.example", "ans1@myco.example", "Re: spoofed"))
	// One reply arrives already on a deal, as a connector files it.
	env.syncFiledUnderDeal(t, map[string][]ids.UUID{"ans3@acme.example": {operations}},
		mailFrom("alice@acme.example", "ans3@acme.example", "ans1@myco.example", "Re: project"))
	if n := openQuestions(t, e); n != 1 {
		t.Fatalf("%d open deal questions before the answer, want 1", n)
	}
	// Alice's promise in our own mail of the conversation, read before the
	// answer, as the extractor files one.
	promise, err := w.contacts.RecordConversationClaim(w.ctx, contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](alice), Kind: "commitment_theirs", Body: "Send the figures",
		ActivityID: activityOf(t, e, "ans1@myco.example"), Quote: "hello", Source: "extraction",
	})
	if err != nil {
		t.Fatalf("filing the promise: %v", err)
	}
	store := activities.NewStore(e.DB())
	relink := func(sourceID string, deal ids.UUID) {
		t.Helper()
		mail := ids.From[ids.ActivityKind](activityOf(t, e, sourceID))
		if _, err := store.RelinkActivity(w.ctx, mail, activities.RelinkActivityInput{EntityType: "deal", EntityID: deal}); err != nil {
			t.Fatalf("filing %s as a colleague: %v", sourceID, err)
		}
	}
	relink("ans2@acme.example", reporting)

	wantDeals(t, e, "ans1@myco.example", reporting)
	wantDeals(t, e, "ans2@acme.example", reporting)
	wantDeals(t, e, "ans4@acme.example", reporting)
	wantDeals(t, e, "ans3@acme.example", operations)
	// A reply whose sender nobody authenticated may be a spoof of the thread.
	wantDeals(t, e, "ans5@acme.example")
	if n := openQuestions(t, e); n != 0 {
		t.Fatalf("%d deal questions still open after the answer, want 0", n)
	}
	timeline := func(deal ids.UUID) int {
		t.Helper()
		rows, _, err := store.ListActivities(w.ctx, activities.ListActivitiesInput{
			EntityType: new("deal"), EntityID: &deal, Kind: new("email"),
		})
		if err != nil {
			t.Fatalf("reading the deal timeline: %v", err)
		}
		return len(rows)
	}
	if got := timeline(reporting); got != 3 {
		t.Fatalf("the answered deal's timeline shows %d mails, want the 3 proven mails of the conversation", got)
	}
	if got := timeline(operations); got != 1 {
		t.Fatalf("the other deal's timeline shows %d mails, want only the one already filed there", got)
	}
	// The company page still shows the whole conversation.
	account := w.customer
	onCompany, _, err := store.ListActivities(w.ctx, activities.ListActivitiesInput{
		EntityType: new("company"), EntityID: &account, Kind: new("email"),
	})
	if err != nil || len(onCompany) != 5 {
		t.Fatalf("the company timeline shows %d mails (err %v), want the 5 of the conversation", len(onCompany), err)
	}
	// The promise watches the answered deal and not the other one.
	for deal, want := range map[ids.UUID]int{reporting: 1, operations: 0} {
		if err := database.WithWorkspaceTx(w.ctx, e.Pool, func(tx pgx.Tx) error {
			rows, _, err := w.contacts.OpenTheirCommitmentsForDeal(w.ctx, tx, deal, 20)
			if err == nil && (len(rows) != want || (want == 1 && rows[0].ID != ids.UUID(promise.Id))) {
				t.Errorf("deal %s watches %d promises, want %d", deal, len(rows), want)
			}
			return err
		}); err != nil {
			t.Fatalf("reading the deal's promises: %v", err)
		}
	}
}

// The repair re-checks the links the rule wrote, from the rule's own audit
// rows. A link the rule still finds stays. One it now finds elsewhere moves,
// audited as the repair with its reason. A connector's own deal ref has no
// such record, so it is reported and left alone.
func TestTheDealRepairRechecksOnlyTheRulesOwnLinks(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w := newDealWorld(t, e)
	alice := w.employee(t, "alice@acme.example", w.customer)
	first := w.deal(t, "Acme rollout")
	beta := w.at(t, "Beta")
	beta.employee(t, "bob@beta.example", beta.customer)
	betaDeal := beta.deal(t, "Beta rollout")
	env.sync(t,
		mailFrom("alice@acme.example", "re1@acme.example", "", "status"),
		mailFrom("bob@beta.example", "re2@beta.example", "", "status"))
	env.syncFiledUnderDeal(t, map[string][]ids.UUID{"re3@acme.example": {first}},
		mailFrom("alice@acme.example", "re3@acme.example", "", "other"))
	wantDeals(t, e, "re1@acme.example", first)
	wantDeals(t, e, "re2@beta.example", betaDeal)

	// Acme gains a second deal that alice is a stakeholder on, so the rule now
	// files her mail there.
	second := w.deal(t, "Acme reporting")
	w.stakeholder(t, alice, second)
	r := newRepairer(e, w)
	preview := r.preview(t)
	want := map[capturemod.DealOutcome]int{
		capturemod.DealMachineMoved: 1, capturemod.DealMachineConfirmed: 1, capturemod.DealUnknownLink: 1,
	}
	if !maps.Equal(preview, want) {
		t.Fatalf("the preview reads %v, want %v", preview, want)
	}
	wantDeals(t, e, "re1@acme.example", first)
	if changed := r.run(t); changed != 1 {
		t.Fatalf("the repair changed %d mails, want the one the rule now files elsewhere", changed)
	}
	wantDeals(t, e, "re1@acme.example", second)
	wantDeals(t, e, "re2@beta.example", betaDeal)
	wantDeals(t, e, "re3@acme.example", first)
	if n := countRows(t, e, `SELECT count(*) FROM audit_log WHERE action = 'activity_relink'
		 AND actor_type = 'system' AND evidence->>'reason' = 'captured_mail_deal_repair'`); n != 2 {
		t.Fatalf("%d repair audit rows, want the retraction and the new link, each naming the repair", n)
	}
	if again := r.run(t); again != 0 {
		t.Fatalf("a second repair changed %d more mails, want none", again)
	}
}

// The question is asked only about current mail, judged by the mail's own
// time against the waiting horizon, never by when it was captured. The same
// clause keeps the question in the colleague's task list, and drops it once
// the mail ages past the horizon.
func TestTheDealQuestionIsAskedOnlyAboutCurrentMail(t *testing.T) {
	horizon := time.Duration(storekit.DefaultWaitingHorizonDays) * 24 * time.Hour
	for _, c := range []struct {
		name  string
		now   time.Time
		asked bool
	}{
		{"on the horizon", mailDay.Add(horizon), true},
		{"a second past it", mailDay.Add(horizon + time.Second), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			env := newCaptureEnvAt(t, c.now)
			e := env.e
			w := newDealWorld(t, e)
			w.employee(t, "alice@acme.example", w.customer)
			w.deal(t, "Acme operations")
			w.deal(t, "Acme reporting")
			env.syncSent(t, map[string]bool{"cur1@myco.example": true},
				email(captureOwner, "", "alice@acme.example", "cur1@myco.example", ""))
			if n, _ := whichDealQuestions(t, e); (n == 1) != c.asked {
				t.Fatalf("%d deal questions for mail of %s, want asked=%v", n, mailDay, c.asked)
			}
		})
	}
	t.Run("the task list drops it past the horizon", func(t *testing.T) {
		env := newCaptureEnv(t)
		e := env.e
		w := newDealWorld(t, e)
		w.employee(t, "alice@acme.example", w.customer)
		w.deal(t, "Acme operations")
		w.deal(t, "Acme reporting")
		env.syncSent(t, map[string]bool{"cur2@myco.example": true},
			email(captureOwner, "", "alice@acme.example", "cur2@myco.example", ""))
		rep := ids.From[ids.UserKind](e.Rep1)
		listed := func(asOf time.Time) int {
			t.Helper()
			rows, _, err := activities.NewStore(e.DB()).ListActivities(w.ctx, activities.ListActivitiesInput{
				AssigneeID: &rep, AsOf: &asOf, Kind: new("task"),
			})
			if err != nil {
				t.Fatalf("reading the colleague's tasks: %v", err)
			}
			return len(rows)
		}
		if got := listed(captureNow); got != 1 {
			t.Fatalf("the colleague's task list shows %d deal questions for current mail, want 1", got)
		}
		if got := listed(mailDay.Add(horizon + time.Hour)); got != 0 {
			t.Fatalf("the task list still shows %d deal questions after the mail aged out", got)
		}
		// A question the colleague took up by hand is theirs, whatever its age.
		var question ids.UUID
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(), `SELECT id FROM activity WHERE source_system = 'which_deal'`).Scan(&question)
		}); err != nil {
			t.Fatal(err)
		}
		subject := "Which deal: ask Alice on Monday"
		if _, err := activities.NewStore(e.DB()).UpdateActivity(w.ctx, ids.From[ids.ActivityKind](question),
			activities.UpdateActivityInput{Subject: &subject}); err != nil {
			t.Fatalf("taking the question up by hand: %v", err)
		}
		if got := listed(mailDay.Add(horizon + time.Hour)); got != 1 {
			t.Fatalf("the task list shows %d questions the colleague took up, want 1", got)
		}
	})
}
