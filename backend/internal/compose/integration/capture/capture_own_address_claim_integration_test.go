// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A member claims, as their own, an address capture already made a contact for.
// The contact is archived and its mail stays the member's own correspondence.
//
// Every mail goes through the production mailmap and sink. Every claim goes
// through the store the app builds, ownaddress.NewStore.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/ownaddress"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	claimedFormer = "founder@previous-employer.example"
	claimedBuyer  = "buyer@acme.example"
)

func claimCount(t *testing.T, e *integration.SearchEnv, query string, args ...any) int {
	t.Helper()
	var n int
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, args...).Scan(&n)
	})
	if err != nil {
		t.Fatalf("counting %q: %v", query, err)
	}
	return n
}

func claimText(t *testing.T, e *integration.SearchEnv, query string, args ...any) string {
	t.Helper()
	var out string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, args...).Scan(&out)
	})
	if err != nil {
		t.Fatalf("reading %q: %v", query, err)
	}
	return out
}

func claimExec(t *testing.T, e *integration.SearchEnv, query string, args ...any) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), query, args...)
		return err
	})
	if err != nil {
		t.Fatalf("running %q: %v", query, err)
	}
}

// claimStore is the owner-identity store as the app builds it.
func claimStore(e *integration.SearchEnv) *capturemod.OwnerIdentityStore {
	return ownaddress.NewStore(e.DB(), contacts.NewStore(e.DB()))
}

// claimSeat is the member as the app sees them: a human seat holding the
// grants the claim needs to archive a contact it may write.
func claimSeat(e *integration.SearchEnv, user ids.UUID) context.Context {
	return principal.WithActor(seatContext(e, user), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType: principal.SeatFull, Permissions: integration.AdminPerms,
	})
}

// claimAsOwn declares the address as the user's own, answering the question the
// product asks first about an address a contact already holds.
func claimAsOwn(t *testing.T, e *integration.SearchEnv, user ids.UUID, address string) {
	t.Helper()
	if _, err := claimStore(e).AddConfirmed(claimSeat(e, user), capturemod.IdentityKindAddress, address, true); err != nil {
		t.Fatalf("claiming %s: %v", address, err)
	}
}

// claimMintFormerContact lands the exchange that makes capture file a contact.
// The former address writes, the member answers, and the provider attests the
// answer as sent. A third message copies a buyer, a real other party.
func claimMintFormerContact(t *testing.T, env captureEnv) ids.UUID {
	t.Helper()
	env.syncSent(t, map[string]bool{"f2@myco.example": true},
		email(claimedFormer, "Founder Former", captureOwner, "f1@previous-employer.example", ""),
		email(captureOwner, "", claimedFormer, "f2@myco.example", "f1@previous-employer.example"),
		emailCC(claimedFormer, "Founder Former", captureOwner, claimedBuyer, "f3@previous-employer.example"),
	)
	n := claimCount(t, env.e, `
		SELECT count(*) FROM contact c JOIN contact_email ce ON ce.contact_id = c.id
		 WHERE ce.email = $1 AND c.archived_at IS NULL AND c.owner_id = $2`, claimedFormer, env.e.Rep1)
	if n != 1 {
		t.Fatalf("%d live contacts for %s owned by the member, want 1: the fixture no longer exercises a minted contact", n, claimedFormer)
	}
	var id ids.UUID
	err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT c.id FROM contact c JOIN contact_email ce ON ce.contact_id = c.id WHERE ce.email = $1`,
			claimedFormer).Scan(&id)
	})
	if err != nil {
		t.Fatalf("finding the minted contact: %v", err)
	}
	return id
}

func contactLinkCount(t *testing.T, e *integration.SearchEnv, contact ids.UUID) int {
	t.Helper()
	return claimCount(t, e, `SELECT count(*) FROM activity_link WHERE contact_id = $1`, contact)
}

func claimContactLive(t *testing.T, e *integration.SearchEnv, contact ids.UUID) bool {
	t.Helper()
	return claimCount(t, e, `SELECT count(*) FROM contact WHERE id = $1 AND archived_at IS NULL`, contact) == 1
}

func claimBuyerRows(t *testing.T, e *integration.SearchEnv) int {
	t.Helper()
	return claimCount(t, e, `SELECT count(*) FROM activity_participant WHERE address = $1`, claimedBuyer)
}

func TestClaimingAnAddressArchivesItsMintedContactAndRefilesItsMail(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	before := contactLinkCount(t, e, contact)
	buyerBefore := claimBuyerRows(t, e)
	if before == 0 || buyerBefore == 0 {
		t.Fatalf("the fixture linked %d messages to the contact and stamped the buyer %d times; "+
			"it proves nothing about refiling", before, buyerBefore)
	}

	claimAsOwn(t, e, e.Rep1, claimedFormer)

	if claimContactLive(t, e, contact) {
		t.Error("the contact capture made for the claimed address is still live")
	}
	if n := claimCount(t, e, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND entity_id = $1 AND action = 'archive'`, contact); n != 1 {
		t.Errorf("%d archive audit rows for the contact, want 1", n)
	}
	if n := contactLinkCount(t, e, contact); n != 0 {
		t.Errorf("%d messages are still linked to the archived contact", n)
	}
	if n := claimCount(t, e, `SELECT count(*) FROM activity_participant WHERE contact_id = $1`, contact); n != 0 {
		t.Errorf("%d participant rows still name the archived contact", n)
	}
	if n := claimCount(t, e, `SELECT count(*) FROM activity WHERE kind = 'email' AND archived_at IS NULL`); n != 3 {
		t.Errorf("%d live messages, want all 3: refiling archives or deletes no mail", n)
	}
	if n := claimCount(t, e, `
		SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND action = 'activity_relink'
		   AND evidence->>'reason' = 'own_address_claimed'`); n != before {
		t.Errorf("%d audited unlinks, want one per message taken off the contact (%d)", n, before)
	}
	if n := claimBuyerRows(t, e); n != buyerBefore {
		t.Errorf("the buyer is on %d participant rows, was on %d: mail that involves another party must keep that party", n, buyerBefore)
	}
}

func TestRefilingKeepsWhoMayReadTheMail(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	claimMintFormerContact(t, env)
	const readers = `
		SELECT coalesce(string_agg(a.id::text || ':' || a.audience || ':' || coalesce(a.restricted_at::text, ''), ',' ORDER BY a.id), '')
		  FROM activity a WHERE a.kind = 'email'`
	before := claimText(t, e, readers)
	members := `SELECT count(*) FROM activity_participant WHERE user_id IS NOT NULL AND user_id <> $1`
	othersBefore := claimCount(t, e, members, e.Rep1)

	claimAsOwn(t, e, e.Rep1, claimedFormer)

	if after := claimText(t, e, readers); after != before {
		t.Errorf("refiling changed who may read the mail:\n before %s\n after  %s", before, after)
	}
	if n := claimCount(t, e, members, e.Rep1); n != othersBefore {
		t.Errorf("%d participant rows name another member, was %d: the mail must not gain a colleague", n, othersBefore)
	}
	if n := claimCount(t, e, `
		SELECT count(*) FROM activity a WHERE a.kind = 'email' AND split_part(a.captured_by, ':', 3) <> $1::text`, e.Rep1); n != 0 {
		t.Errorf("%d messages are no longer stamped as the member's own capture", n)
	}
}

func TestAContactAColleagueOwnsIsLeftWhenAnotherSeatClaimsItsAddress(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	var theirs ids.UUID
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `
			INSERT INTO contact (owner_id, full_name, source, captured_by, visibility)
			VALUES ($1, 'Founder Former', 'gmail', 'connector:gmail', 'owner') RETURNING id`, e.Rep3).Scan(&theirs); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
			VALUES ($1, $2, true, 'gmail', 'connector:gmail')`, theirs, claimedFormer)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the colleague's contact: %v", err)
	}

	claimAsOwn(t, e, e.Rep1, claimedFormer)

	if !claimContactLive(t, e, theirs) {
		t.Error("the member's claim archived a contact their colleague owns")
	}
}

func TestAContactTwoSeatsClaimIsLeft(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	claimAsOwn(t, e, e.Rep3, claimedFormer)

	claimAsOwn(t, e, e.Rep1, claimedFormer)

	if !claimContactLive(t, e, contact) {
		t.Error("an address two seats claim is not one member's alone, yet its contact was archived")
	}
}

func TestAContactWithAColleaguesOrHandLoggedMailIsLeftWhole(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	before := contactLinkCount(t, e, contact)
	claimExec(t, e, `
		UPDATE activity SET captured_by = 'human:' || $1::text
		 WHERE id = (SELECT activity_id FROM activity_link WHERE contact_id = $2 ORDER BY activity_id LIMIT 1)`,
		e.Rep3, contact)

	claimAsOwn(t, e, e.Rep1, claimedFormer)

	if !claimContactLive(t, e, contact) {
		t.Error("the contact was archived although one message linked to it is not the member's own capture")
	}
	if n := contactLinkCount(t, e, contact); n != before {
		t.Errorf("%d links to the left contact, want the %d it had: the savepoint must undo the refile", n, before)
	}
	if n := claimCount(t, e, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND entity_id = $1 AND action = 'archive'`, contact); n != 0 {
		t.Errorf("%d archive audit rows for a contact that was left", n)
	}
}

func TestClaimingTheSameAddressAgainChangesNothing(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	claimAsOwn(t, e, e.Rep1, claimedFormer)
	const trail = `SELECT count(*) FROM audit_log WHERE entity_type IN ('contact', 'activity') AND action IN ('archive', 'activity_relink')`
	first := claimCount(t, e, trail)

	claimAsOwn(t, e, e.Rep1, claimedFormer)

	if second := claimCount(t, e, trail); second != first {
		t.Errorf("a second claim wrote %d more audit rows", second-first)
	}
	if claimContactLive(t, e, contact) {
		t.Error("the contact came back to life")
	}
}

func TestAContactAUserEditedIsLeftWhole(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	claimExec(t, e, `
		INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
		VALUES ('human', $1, 'update', 'contact', $2)`, e.Rep1.String(), contact)

	claimAsOwn(t, e, e.Rep1, claimedFormer)

	if !claimContactLive(t, e, contact) {
		t.Error("a contact a user had worked on was archived by the claim")
	}
	if n := contactLinkCount(t, e, contact); n == 0 {
		t.Error("the left contact lost its links")
	}
}

func TestAContactPublishedToTheWorkspaceIsLeftWhole(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	claimExec(t, e, `UPDATE contact SET visibility = 'workspace', narrowing_reason = NULL WHERE id = $1`, contact)

	claimAsOwn(t, e, e.Rep1, claimedFormer)

	if !claimContactLive(t, e, contact) {
		t.Error("a contact the whole workspace can see was archived on one member's claim")
	}
}
