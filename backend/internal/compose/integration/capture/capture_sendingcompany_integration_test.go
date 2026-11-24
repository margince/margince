// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A mailbox connected before connections carried a sending company must still
// count for the send guard when the installation has two legal entities.
// The guard reads the stored company of each mail and meeting. So the company
// must reach every import, through connect, reconnect and the one-off correction.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/importscopes"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	oldMailAuthor   = "olivia@partner.example"
	newMailAuthor   = "nina@partner.example"
	scopeCorrespond = "business_correspondence"
)

type companyEnv struct {
	captureEnv
	defaultEntity, branch ids.UUID
}

// twoCompanies is an installation with two legal entities. Rep1 is in the
// default one, Rep3 in the branch, and Rep1's connection has no company.
func twoCompanies(t *testing.T) companyEnv {
	t.Helper()
	env := newCaptureEnv(t)
	e := env.e
	c := companyEnv{captureEnv: env, branch: ids.NewV7()}
	if err := e.Owner.QueryRow(context.Background(), `SELECT id FROM legal_entity WHERE is_default`).Scan(&c.defaultEntity); err != nil {
		t.Fatal(err)
	}
	exec(t, e, `INSERT INTO legal_entity (id, display_name, legal_name, country, captured_by) VALUES ($1, 'Branch', 'Branch Ltd', 'vn', 'human:x')`, c.branch)
	exec(t, e, `INSERT INTO legal_entity_member (user_id, legal_entity_id, assigned_by) VALUES ($1, $2, 'human:x')`, e.Rep3, c.branch)
	exec(t, e, `UPDATE capture_connection SET legal_entity_id = NULL WHERE user_id = $1`, e.Rep1)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return consent.SeedDefaultPurposesTx(e.Admin(), tx)
	}); err != nil {
		t.Fatalf("seeding the purpose catalog: %v", err)
	}
	return c
}

func exec(t *testing.T, e *integration.SearchEnv, sql string, args ...any) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func partnerContact(t *testing.T, e *integration.SearchEnv, address string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	exec(t, e, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, $2, 'manual', 'human:x')`, id, address)
	exec(t, e, `INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by) VALUES ($1, $2, true, 'manual', 'human:x')`, id, address)
	return id
}

// guardAs reads the send guard's answer for business correspondence as one member.
func guardAs(t *testing.T, e *integration.SearchEnv, user, contact ids.UUID) (verdict, reason string) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}, "activity": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	guard, err := consent.NewStore(e.DB()).ContactConsentGuard(ctx, ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("reading the guard: %v", err)
	}
	for _, entry := range guard.Entries {
		if entry.PurposeKey == scopeCorrespond {
			return string(entry.Verdict), string(entry.ReasonCode)
		}
	}
	t.Fatal("the guard did not report on business correspondence")
	return "", ""
}

func requireGuard(t *testing.T, e *integration.SearchEnv, user, contact ids.UUID, who, want string) {
	t.Helper()
	if got, reason := guardAs(t, e, user, contact); got != want {
		t.Fatalf("%s: guard = %s (%s), want %s", who, got, reason, want)
	}
}

func importCompany(t *testing.T, e *integration.SearchEnv, contact ids.UUID) *ids.UUID {
	t.Helper()
	var entity *ids.UUID
	if err := e.Owner.QueryRow(context.Background(), `SELECT i.legal_entity_id FROM capture_import i
		JOIN activity_link l ON l.activity_id = i.activity_id AND l.contact_id = $1`, contact).Scan(&entity); err != nil {
		t.Fatalf("reading the import of %s: %v", contact, err)
	}
	return entity
}

// scopeOlderImports is the data task's write, run as the system.
func scopeOlderImports(t *testing.T, e *integration.SearchEnv) int {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	var scoped []ids.UUID
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		scoped, err = capture.ScopeImportsTx(ctx, tx, ids.Nil, 100)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return len(scoped)
}

func reconnect(t *testing.T, c companyEnv) {
	t.Helper()
	grant := humanWithScopes(c.e, c.e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := c.registry.Connect(grant, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
}

func TestMailFromAMailboxConnectedBeforeTheScopeCountsForItsOwnersCompany(t *testing.T) {
	c := twoCompanies(t)
	e := c.e
	old, fresh := partnerContact(t, e, oldMailAuthor), partnerContact(t, e, newMailAuthor)

	c.sync(t, email(oldMailAuthor, "Olivia", captureOwner, "old-mail@partner.example", ""))
	if got := importCompany(t, e, old); got != nil {
		t.Fatalf("the old connection scoped its import to %s", got)
	}
	requireGuard(t, e, e.Rep1, old, "before the fix", "unknown")

	reconnect(t, c)
	var connection *ids.UUID
	if err := e.Owner.QueryRow(context.Background(), `SELECT legal_entity_id FROM capture_connection WHERE user_id = $1 AND provider = 'gmail'`, e.Rep1).Scan(&connection); err != nil {
		t.Fatal(err)
	}
	if connection == nil || *connection != c.defaultEntity {
		t.Fatalf("the reconnected mailbox has company %v, want the owner's %s", connection, c.defaultEntity)
	}

	c.sync(t, email(newMailAuthor, "Nina", captureOwner, "new-mail@partner.example", ""))
	if got := importCompany(t, e, fresh); got == nil || *got != c.defaultEntity {
		t.Fatalf("mail captured after the reconnect has company %v, want %s", got, c.defaultEntity)
	}
	requireGuard(t, e, e.Rep1, fresh, "owner, mail after the reconnect", "allowed")
	requireGuard(t, e, e.Rep3, fresh, "colleague of another company", "unknown")

	requireGuard(t, e, e.Rep1, old, "owner, mail before the correction", "unknown")
	if n := scopeOlderImports(t, e); n != 1 {
		t.Fatalf("the correction scoped %d imports, want the one old mail", n)
	}
	requireGuard(t, e, e.Rep1, old, "owner, mail after the correction", "allowed")
	requireGuard(t, e, e.Rep3, old, "colleague of another company, old mail", "unknown")
	if n := scopeOlderImports(t, e); n != 0 {
		t.Fatalf("a second run scoped %d imports, want none", n)
	}
}

func TestAnImportWhoseMailboxHasNoOwnerStaysUnscoped(t *testing.T) {
	c := twoCompanies(t)
	e := c.e
	old := partnerContact(t, e, oldMailAuthor)
	c.sync(t, email(oldMailAuthor, "Olivia", captureOwner, "old-mail@partner.example", ""))
	exec(t, e, `UPDATE capture_import SET user_id = NULL, connection_id = (SELECT id FROM capture_connection WHERE user_id IS NULL AND account_label = $1)
		WHERE legal_entity_id IS NULL`, fixtureSharedMailbox)
	if n := scopeOlderImports(t, e); n != 0 {
		t.Fatalf("the correction guessed a company for %d imports of an ownerless mailbox", n)
	}
	if got := importCompany(t, e, old); got != nil {
		t.Fatalf("the import was given company %s", got)
	}
}

func TestMeetingsFromACalendarConnectedBeforeTheScopeCountForItsOwnersCompany(t *testing.T) {
	c := twoCompanies(t)
	e := c.e
	guest := partnerContact(t, e, meetingGuest)

	syncMeeting(t, c.captureEnv, "meet-scope@partner.example")
	exec(t, e, `UPDATE capture_connection SET legal_entity_id = NULL WHERE user_id = $1`, e.Rep1)
	exec(t, e, `UPDATE capture_import SET legal_entity_id = NULL`)
	requireGuard(t, e, e.Rep1, guest, "meeting from an unscoped calendar", "unknown")

	exec(t, e, `UPDATE capture_connection SET legal_entity_id = $1 WHERE user_id = $2`, c.defaultEntity, e.Rep1)
	if n := scopeOlderImports(t, e); n == 0 {
		t.Fatal("the correction scoped no meeting import")
	}
	requireGuard(t, e, e.Rep1, guest, "owner, meeting after the correction", "allowed")
	requireGuard(t, e, e.Rep3, guest, "colleague of another company", "unknown")
}

// An import takes the company of the live connection that delivered it.
// A seat with mailboxes in two companies is scoped by the one named in delivered_by.
// It stays unscoped when both delivered it or the mailbox has no company.
func TestTheImportScopeTaskFollowsTheDeliveringMailboxAndGuessesNone(t *testing.T) {
	c := twoCompanies(t)
	e := c.e
	for label, entity := range map[string]ids.UUID{"first@scope.test": c.branch, "second@scope.test": c.defaultEntity} {
		exec(t, e, `INSERT INTO capture_connection (provider, user_id, status, credential_ref, account_label, legal_entity_id)
			VALUES ('gmail', $1, 'connected', 'vault:test', $2, $3)`, e.Rep1, label, entity)
	}
	exec(t, e, `INSERT INTO capture_connection (provider, user_id, status, credential_ref, account_label)
		VALUES ('gmail', $1, 'connected', 'vault:test', 'other@scope.test')`, e.Rep3)
	mark := func(user ids.UUID, label string) string {
		var out string
		if err := e.Owner.QueryRow(context.Background(), `SELECT `+storekit.MailboxMarkSQL("c")+` FROM capture_connection c WHERE user_id=$1 AND account_label=$2`, user, label).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	first, second := mark(e.Rep1, "first@scope.test"), mark(e.Rep1, "second@scope.test")

	type planted struct {
		name      string
		seat      ids.UUID
		delivered []string
		entity    *ids.UUID
		want      *ids.UUID
	}
	cases := []planted{
		{name: "second mailbox", seat: e.Rep1, delivered: []string{second}, want: &c.defaultEntity},
		{name: "first mailbox", seat: e.Rep1, delivered: []string{first}, want: &c.branch},
		{name: "both mailboxes of two companies", seat: e.Rep1, delivered: []string{first, second}},
		{name: "mailbox with no company", seat: e.Rep3, delivered: []string{mark(e.Rep3, "other@scope.test")}},
		{name: "already scoped", seat: e.Rep1, delivered: []string{second}, entity: &c.branch, want: &c.branch},
	}
	for _, p := range cases {
		activity := ids.NewV7()
		exec(t, e, `INSERT INTO activity(id,kind,subject,occurred_at,source,captured_by) VALUES($1,'email',$2,'2026-10-01T09:00:00Z','gmail','human:x')`, activity, p.name)
		exec(t, e, `INSERT INTO capture_import(activity_id,user_id,delivered_by,legal_entity_id) VALUES($1,$2,$3,$4)`, activity, p.seat, p.delivered, p.entity)
	}

	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{importscopes.DataTask()}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		outcome, err := engine.Execute(ctx)
		if err != nil {
			t.Fatal(err)
		}
		now = now.Add(outcome.Wait)
	}
	for _, p := range cases {
		var got *ids.UUID
		if err := e.Owner.QueryRow(context.Background(), `SELECT i.legal_entity_id FROM capture_import i JOIN activity a ON a.id = i.activity_id WHERE a.subject = $1`, p.name).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if (p.want == nil) != (got == nil) || (got != nil && *got != *p.want) {
			t.Errorf("%s: company %v, want %v", p.name, got, p.want)
		}
	}
}
