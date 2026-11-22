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
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/ownaddress"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	return compose.NewOwnerIdentityStore(e.Pool, contacts.NewStore(e.DB()))
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
func claimAsOwn(t *testing.T, e *integration.SearchEnv, user ids.UUID, address string) capturemod.OwnerIdentity {
	t.Helper()
	identity, err := claimStore(e).AddConfirmed(claimSeat(e, user), capturemod.IdentityKindAddress, address, true)
	if err != nil {
		t.Fatalf("claiming %s: %v", address, err)
	}
	return identity
}

// claimProve connects a mailbox for the address, which is what proves it is the
// member's own. A typed claim proves nothing.
func claimProve(t *testing.T, e *integration.SearchEnv, user ids.UUID, address string) {
	t.Helper()
	claimExec(t, e, `INSERT INTO capture_connection (provider, user_id, status, account_label) VALUES ('graph', $1, 'connected', $2)`, user, address)
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

// claimProvenFormer proves the address and claims it, the way a member who
// connected the mailbox does.
func claimProvenFormer(t *testing.T, e *integration.SearchEnv) capturemod.OwnerIdentity {
	t.Helper()
	claimProve(t, e, e.Rep1, claimedFormer)
	return claimAsOwn(t, e, e.Rep1, claimedFormer)
}

func archiveRows(t *testing.T, e *integration.SearchEnv, contact ids.UUID) int {
	t.Helper()
	return claimCount(t, e, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND entity_id = $1 AND action = 'archive'`, contact)
}

func TestClaimingAProvenAddressArchivesItsMintedContactAndRefilesItsMail(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	before := contactLinkCount(t, e, contact)
	buyerBefore := claimBuyerRows(t, e)
	if before == 0 || buyerBefore == 0 {
		t.Fatalf("the fixture linked %d messages to the contact and stamped the buyer %d times; "+
			"it proves nothing about refiling", before, buyerBefore)
	}

	claimProvenFormer(t, e)

	if claimContactLive(t, e, contact) {
		t.Error("the contact capture made for the claimed address is still live")
	}
	if n := archiveRows(t, e, contact); n != 1 {
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

// A typed address proves nothing: a member could type a customer's address.
func TestATypedClaimOfACustomersAddressArchivesNothing(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	before := contactLinkCount(t, e, contact)

	claimAsOwn(t, e, e.Rep1, claimedFormer)

	if !claimContactLive(t, e, contact) {
		t.Error("an unproven claim archived the contact of the address it named")
	}
	if n := contactLinkCount(t, e, contact); n != before {
		t.Errorf("%d links left on the contact, want the %d it had: an unproven claim refiles nothing", n, before)
	}
	if n := archiveRows(t, e, contact); n != 0 {
		t.Errorf("%d archive audit rows for a contact that was left", n)
	}
}

func readableBy(t *testing.T, e *integration.SearchEnv, reader context.Context) []bool {
	t.Helper()
	var ids_ []ids.UUID
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT id FROM activity WHERE kind = 'email' ORDER BY id`)
		if err != nil {
			return err
		}
		ids_, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	if err != nil {
		t.Fatalf("listing the mail: %v", err)
	}
	var out []bool
	store := activities.NewStore(e.DB())
	for _, id := range ids_ {
		_, err := store.GetActivityContent(reader, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
		out = append(out, err == nil)
	}
	return out
}

func TestRefilingKeepsWhoMayReadTheMail(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	claimMintFormerContact(t, env)
	colleague := principal.WithActor(seatContext(e, e.Rep3), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep3.String(), UserID: e.Rep3,
		SeatType: principal.SeatFull, Permissions: integration.AdminPerms,
	})
	const readers = `
		SELECT coalesce(string_agg(a.id::text || ':' || a.audience || ':' || coalesce(a.restricted_at::text, ''), ',' ORDER BY a.id), '')
		  FROM activity a WHERE a.kind = 'email'`
	before := claimText(t, e, readers)
	seenBefore := readableBy(t, e, colleague)
	const members = `SELECT count(*) FROM activity_participant WHERE user_id IS NOT NULL AND user_id <> $1`
	othersBefore := claimCount(t, e, members, e.Rep1)

	claimProvenFormer(t, e)

	if after := claimText(t, e, readers); after != before {
		t.Errorf("refiling changed the audience of the mail:\n before %s\n after  %s", before, after)
	}
	seenAfter := readableBy(t, e, colleague)
	for i := range seenBefore {
		if seenAfter[i] && !seenBefore[i] {
			t.Errorf("message %d became readable to a colleague after the refile", i)
		}
	}
	if n := claimCount(t, e, members, e.Rep1); n != othersBefore {
		t.Errorf("%d participant rows name another member, was %d: the mail must not gain a colleague", n, othersBefore)
	}
}

// The claim hook runs as the system, which sees every contact. The two contacts
// then differ only in who owns them.
func TestTheHookLeavesAContactAColleagueOwns(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	mine := claimMintFormerContact(t, env)
	const colleaguesAddress = "colleagues-contact@previous-employer.example"
	var theirs ids.UUID
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `
			INSERT INTO contact (owner_id, full_name, source, captured_by, visibility)
			VALUES ($1, 'Founder Former', 'gmail', 'connector:gmail', 'owner') RETURNING id`, e.Rep3).Scan(&theirs); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
			VALUES ($1, $2, true, 'gmail', 'connector:gmail')`, theirs, colleaguesAddress)
		return err
	})
	if err != nil {
		t.Fatalf("seeding the colleague's contact: %v", err)
	}
	hook := ownaddress.Retire(contacts.NewStore(e.DB()), slog.Default())
	system := principal.SystemActing(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7()), "system:test")
	run := func(address string) {
		err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
			return hook(system, tx, capturemod.ClaimedAddress{ID: ids.NewV7(), User: e.Rep1, Address: address, Proven: true})
		})
		if err != nil {
			t.Fatalf("running the hook for %s: %v", address, err)
		}
	}

	run(claimedFormer)
	run(colleaguesAddress)

	if claimContactLive(t, e, mine) {
		t.Error("the control failed: the claimant's own contact was not archived, so the test proves nothing")
	}
	if !claimContactLive(t, e, theirs) {
		t.Error("the hook archived a contact a colleague owns")
	}
}

func TestAnAddressAColleagueOwnsAMailboxForIsLeftToThem(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	claimProve(t, e, e.Rep3, claimedFormer)

	claimProvenFormer(t, e)

	if !claimContactLive(t, e, contact) {
		t.Error("an address two seats connected was treated as one member's own")
	}
}

func TestAContactTwoSeatsClaimIsLeft(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	claimAsOwn(t, e, e.Rep3, claimedFormer)

	claimProvenFormer(t, e)

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

	claimProvenFormer(t, e)

	if !claimContactLive(t, e, contact) {
		t.Error("the contact was archived although one message linked to it is not the member's own capture")
	}
	if n := contactLinkCount(t, e, contact); n != before {
		t.Errorf("%d links to the left contact, want the %d it had: the savepoint must undo the refile", n, before)
	}
	if n := archiveRows(t, e, contact); n != 0 {
		t.Errorf("%d archive audit rows for a contact that was left", n)
	}
}

func TestAContactWithANonMailActivityIsLeftWhole(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	claimExec(t, e, `
		UPDATE activity SET kind = 'note'
		 WHERE id = (SELECT activity_id FROM activity_link WHERE contact_id = $1 ORDER BY activity_id LIMIT 1)`, contact)

	claimProvenFormer(t, e)

	if !claimContactLive(t, e, contact) {
		t.Error("the contact was archived although a note is linked to it")
	}
}

// A held message is archived with its hold, so the refile never reads it. Its
// link stays where the hold put it, and the rest of the contact's mail moves.
func TestAHeldMessageKeepsItsLinkWhenTheContactIsArchived(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	const first = `(SELECT activity_id FROM activity_link WHERE contact_id = $1 ORDER BY activity_id LIMIT 1)`
	claimExec(t, e, `INSERT INTO activity_retention_evidence (activity_id, basis, qualified_at, decided_by_name, reason)
		VALUES (`+first+`, 'controller_pin', now(), 'Datenschutz', 'litigation hold')`, contact)
	claimExec(t, e, `UPDATE activity SET restricted_at = now(), archived_at = now(), restricted_reason = 'litigation hold',
		restricted_until = now() + interval '1 year', retention_class = 'commercial_correspondence', retention_class_at = now()
		WHERE id = `+first, contact)

	claimProvenFormer(t, e)

	if claimContactLive(t, e, contact) {
		t.Error("the contact stayed live although only a held message was in the way")
	}
	if n := contactLinkCount(t, e, contact); n != 1 {
		t.Errorf("%d links left on the archived contact, want only the held message's", n)
	}
}

func TestAContactWithAMemberOnItsMailIsLeftWhole(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	claimExec(t, e, `
		INSERT INTO activity_participant (activity_id, user_id, role)
		SELECT activity_id, $2, 'cc' FROM activity_link WHERE contact_id = $1 ORDER BY activity_id LIMIT 1`, contact, e.Rep3)

	claimProvenFormer(t, e)

	if !claimContactLive(t, e, contact) {
		t.Error("the contact was archived although a colleague is a participant on its mail")
	}
}

func TestAContactInADealOrARelationshipIsLeftWhole(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	claimExec(t, e, `
		INSERT INTO relationship (kind, contact_id, company_id, role, source, captured_by)
		VALUES ('billing_contact', $1, (SELECT id FROM company LIMIT 1), 'recipient', 'manual', 'human:'||$2::text)`, contact, e.Rep1)

	claimProvenFormer(t, e)

	if !claimContactLive(t, e, contact) {
		t.Error("the contact was archived although a user gave it a relationship")
	}
}

func TestAContactAUserEditedIsLeftWhole(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	claimExec(t, e, `
		INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
		VALUES ('human', $1, 'update', 'contact', $2)`, e.Rep1.String(), contact)

	claimProvenFormer(t, e)

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

	claimProvenFormer(t, e)

	if !claimContactLive(t, e, contact) {
		t.Error("a contact the whole workspace can see was archived on one member's claim")
	}
}

// seedBulkMail links n more of the member's own captured messages to the contact.
func seedBulkMail(t *testing.T, e *integration.SearchEnv, contact ids.UUID, n int) {
	t.Helper()
	claimExec(t, e, `
		INSERT INTO activity (kind, subject, body, raw, direction, source_system, source_id, source, captured_by, counterparty_email)
		SELECT 'email', 'bulk', 'body', '{"headers":"x"}'::jsonb, 'inbound', 'gmail', 'bulk-' || g, 'gmail:bulk-' || g,
		       'connector:gmail:' || $1::text, $2
		  FROM generate_series(1, $3) g`, e.Rep1, claimedFormer, n)
	claimExec(t, e, `
		INSERT INTO activity_link (activity_id, entity_type, contact_id)
		SELECT id, 'contact', $1 FROM activity WHERE source_id LIKE 'bulk-%'`, contact)
}

func TestAContactWithExactlyTheLimitOfMailIsRefiled(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	seedBulkMail(t, e, contact, ownaddress.RefileLimit-contactLinkCount(t, e, contact))

	claimProvenFormer(t, e)

	if claimContactLive(t, e, contact) || contactLinkCount(t, e, contact) != 0 {
		t.Errorf("a contact with %d messages, the limit, was not refiled", ownaddress.RefileLimit)
	}
}

func TestAContactOverTheLimitOfMailIsLeftWhole(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	seedBulkMail(t, e, contact, ownaddress.RefileLimit+1-contactLinkCount(t, e, contact))
	before := contactLinkCount(t, e, contact)

	claimProvenFormer(t, e)

	if !claimContactLive(t, e, contact) || contactLinkCount(t, e, contact) != before {
		t.Errorf("a contact with %d messages, past the limit, was changed", before)
	}
}

func TestClaimingTheSameAddressAgainChangesNothing(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	claimProvenFormer(t, e)
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

func TestWithdrawingAClaimPutsTheContactAndItsMailBack(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	links := contactLinkCount(t, e, contact)
	participants := claimCount(t, e, `SELECT count(*) FROM activity_participant WHERE contact_id = $1`, contact)
	const audiences = `SELECT coalesce(string_agg(id::text || a.audience, ',' ORDER BY id), '') FROM activity a`
	audience := claimText(t, e, audiences)
	identity := claimProvenFormer(t, e)

	if err := claimStore(e).Remove(claimSeat(e, e.Rep1), identity.ID); err != nil {
		t.Fatalf("withdrawing the claim: %v", err)
	}

	if !claimContactLive(t, e, contact) {
		t.Error("withdrawing the claim left the contact archived")
	}
	if n := contactLinkCount(t, e, contact); n != links {
		t.Errorf("%d links after the withdrawal, want the %d it had", n, links)
	}
	if n := claimCount(t, e, `SELECT count(*) FROM activity_participant WHERE contact_id = $1`, contact); n != participants {
		t.Errorf("%d participant rows after the withdrawal, want %d", n, participants)
	}
	if got := claimText(t, e, audiences); got != audience {
		t.Errorf("the withdrawal changed who may read the mail:\n before %s\n after  %s", audience, got)
	}
}

// Putting a withdrawn claim back is judged like declaring it. The contact is
// live again, so the product asks first and changes nothing until it is answered.
func TestPuttingAWithdrawnClaimBackAsksAboutTheLiveContact(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	identity := claimProvenFormer(t, e)
	store := claimStore(e)
	if err := store.Remove(claimSeat(e, e.Rep1), identity.ID); err != nil {
		t.Fatalf("withdrawing the claim: %v", err)
	}
	links := contactLinkCount(t, e, contact)

	if _, err := store.RestoreRemoval(claimSeat(e, e.Rep1), identity.ID); err == nil {
		t.Fatal("putting the claim back did not ask about the contact that holds the address")
	}

	if !claimContactLive(t, e, contact) || contactLinkCount(t, e, contact) != links {
		t.Error("a refused restore changed the contact or its mail")
	}
}

// A contact a verdict retracts is capture's own: its mint row names the mailbox
// owner in on_behalf_of and must not count as somebody's work.
func TestACaptureMintedContactIsRetractableByAVerdict(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	contact := claimMintFormerContact(t, env)
	var retracted bool
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		retracted, err = contacts.NewStore(e.DB()).RetractCaptureOnlyContactTx(principal.WithCorrelationID(e.Admin(), ids.NewV7()), tx, ids.From[ids.ContactKind](contact), e.Rep1)
		return err
	})
	if err != nil {
		t.Fatalf("retracting: %v", err)
	}
	if !retracted {
		t.Error("a contact capture minted could not be retracted: its own audit row vetoed it")
	}
}
