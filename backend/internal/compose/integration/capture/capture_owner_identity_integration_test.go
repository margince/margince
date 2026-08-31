// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A seat's OTHER addresses, through the production sink.
//
// The connector decides direction by comparing the From header against the one
// address the grant was made for, so a message sent from an alias arrives
// looking like inbound mail from a stranger — and the creation ladder then
// decides whether to mint a record for the mailbox owner. These prove the sink
// corrects that, and that a seat's claim binds their own mail only.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownerAlias is the founder's own private domain in these fixtures — the shape
// that actually produced a contact record: a second address the same person
// reads, on a domain the workspace never registered as its own.
const (
	ownerAlias       = "lars@private.example"
	ownerAliasDomain = "private.example"
)

// declareIdentity records one of the acting seat's own addresses through the
// real store, so the fixture exercises the gate the product ships rather than
// a row hand-inserted in a shape no writer produces.
func declareIdentity(t *testing.T, e *integration.SearchEnv, user ids.UUID, kind, value string) {
	t.Helper()
	store := capturemod.NewOwnerIdentityStore(e.DB())
	if _, err := store.Add(seatContext(e, user), kind, value); err != nil {
		t.Fatalf("declaring %s %q: %v", kind, value, err)
	}
}

// seatContext is one human seat, which is all declaring your own address takes.
func seatContext(e *integration.SearchEnv, user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType: principal.SeatFull,
	})
}

func TestMailAmongTheOwnersOwnAddressesLeavesNoRow(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	declareIdentity(t, e, e.Rep1, capturemod.IdentityKindAddress, ownerAlias)

	// Both legs: the owner writing to their own alias, and the alias writing
	// back. The second is the one that used to mint a person — it reads as
	// inbound mail from a stranger, because the connector knows one address.
	sync(t,
		email(captureOwner, "", ownerAlias, "self1@myco.example", ""),
		email(ownerAlias, "Lars Private", captureOwner, "self2@private.example", "self1@myco.example"),
	)

	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = '`+ownerAlias+`'`); n != 0 {
		t.Errorf("%d person(s) for the owner's own alias, want 0 — an alias is not a contact", n)
	}
	if n := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'email'`); n != 0 {
		t.Errorf("%d activity row(s) for mail among the owner's own addresses, want 0 — "+
			"one person talking to themselves is not correspondence the CRM was asked to hold", n)
	}
}

func TestAnOwnersAliasCopiedOnACustomerThreadStillCapturesTheCustomer(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	declareIdentity(t, e, e.Rep1, capturemod.IdentityKindAddress, ownerAlias)

	// The alias is a party, and a customer is too. The message is external and
	// the customer is who the ladder is about — the alias must not stand in for
	// them, and must not be minted beside them.
	sync(t, emailCC("alice@acme.example", "Alice Example", captureOwner,
		ownerAlias, "cc1@acme.example"))

	if n := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'email'`); n != 1 {
		t.Fatalf("%d activity row(s), want 1 — a message naming a customer is not internal", n)
	}
	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = '`+ownerAlias+`'`); n != 0 {
		t.Errorf("%d person(s) for the owner's alias, want 0", n)
	}
}

func TestAnOwnersOwnDomainCoversTheAddressesOnIt(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	// A domain claim, not an address claim: a founder reads several addresses
	// on their own domain and cannot list them one by one.
	declareIdentity(t, e, e.Rep1, capturemod.IdentityKindDomain, ownerAliasDomain)

	sync(t, email("family@"+ownerAliasDomain, "Family", captureOwner, "dom1@private.example", ""))

	if n := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'email'`); n != 0 {
		t.Errorf("%d activity row(s) from the owner's own domain, want 0 — "+
			"a declared domain covers the addresses on it, the way the workspace's own domains do", n)
	}
}

func TestAColleaguesIdentityIsNotMine(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	// Rep3 claims the address; Rep1 is the seat whose mailbox syncs. One seat's
	// claim says nothing about another seat's mail — otherwise anyone could
	// silence a colleague's counterparty by claiming their address.
	declareIdentity(t, e, e.Rep3, capturemod.IdentityKindAddress, "claimed@acme.example")

	// The owner's reply is ATTESTED, which is what makes T1 mint the person:
	// only a provider-vouched send counts as the workspace writing to them.
	// Without it the sender defers to the verdict engine and this test would
	// read a deferral as a colleague's claim taking effect.
	env.syncSent(t, map[string]bool{"other2@myco.example": true},
		email("claimed@acme.example", "Claimed Party", captureOwner, "other1@acme.example", ""),
		email(captureOwner, "", "claimed@acme.example", "other2@myco.example", "other1@acme.example"),
	)

	if n := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'email'`); n != 2 {
		t.Fatalf("%d activity row(s), want 2 — a colleague's claim must not drop this mailbox's mail", n)
	}
	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'claimed@acme.example'`); n != 1 {
		t.Errorf("%d person(s) for the address, want 1 — Rep3's claim binds Rep3's mail, not Rep1's", n)
	}
}

func TestWithdrawingAnIdentityJudgesTheAddressLikeAnyOther(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	store := capturemod.NewOwnerIdentityStore(e.DB())
	seat := seatContext(e, e.Rep1)

	identity, err := store.Add(seat, capturemod.IdentityKindAddress, ownerAlias)
	if err != nil {
		t.Fatalf("declaring the alias: %v", err)
	}
	sync(t, email(ownerAlias, "Lars Private", captureOwner, "with1@private.example", ""))
	if n := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'email'`); n != 0 {
		t.Fatalf("%d activity row(s) while the claim stood, want 0 — the fixture proves nothing about withdrawing it", n)
	}

	if err := store.Remove(seat, identity.ID); err != nil {
		t.Fatalf("withdrawing the alias: %v", err)
	}
	// Not retroactive: the message already dropped stays dropped, and the NEXT
	// one is judged like mail from anybody.
	sync(t, email(ownerAlias, "Lars Private", captureOwner, "with2@private.example", ""))
	if n := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'email'`); n != 1 {
		t.Errorf("%d activity row(s) after withdrawing the claim, want 1 — "+
			"the address is judged like any other sender from the next message on", n)
	}
}

func TestAnotherSeatsIdentityIsNotListedOrRemovable(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	store := capturemod.NewOwnerIdentityStore(e.DB())

	theirs, err := store.Add(seatContext(e, e.Rep3), capturemod.IdentityKindAddress, ownerAlias)
	if err != nil {
		t.Fatalf("Rep3 declaring their own alias: %v", err)
	}
	mine, err := store.List(seatContext(e, e.Rep1))
	if err != nil {
		t.Fatalf("listing Rep1's identities: %v", err)
	}
	if len(mine) != 0 {
		t.Errorf("Rep1 sees %d of Rep3's identities — a colleague's alias says which of THEIR mail is private, "+
			"which is the thing the list exists to protect", len(mine))
	}
	if err := store.Remove(seatContext(e, e.Rep1), theirs.ID); err == nil {
		t.Error("Rep1 withdrew Rep3's claim")
	}
}

// The audit trail records that a claim was made, and never which address. An
// owner identity is always one person's, and its whole purpose is keeping a
// private address out of the CRM — the trail must not put it back in.
func TestTheAuditTrailNamesNoDeclaredAddress(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	store := capturemod.NewOwnerIdentityStore(e.DB())
	if _, err := store.Add(seatContext(e, e.Rep1), capturemod.IdentityKindAddress, ownerAlias); err != nil {
		t.Fatalf("declaring the alias: %v", err)
	}

	var images int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM audit_log
			 WHERE coalesce(before::text, '') || coalesce(after::text, '') LIKE '%' || $1 || '%'`,
			ownerAlias).Scan(&images)
	}); err != nil {
		t.Fatal(err)
	}
	if images != 0 {
		t.Errorf("%d audit image(s) name the declared address — the trail is where nothing erases it "+
			"and every admin reads it", images)
	}
}

// The LADDER's own correction, reached where the drop gate cannot help.
//
// A message the owner sent FROM an alias to a customer is external — it names a
// customer, so it is captured — and the connector still derives the ALIAS as
// its counterparty, because the From header is not the address the grant was
// made for. The creation ladder is then deciding about the mailbox owner.
//
// The assertion is on WHO the ladder is asked about, read from the pending
// ledger it records that in, rather than on a minted person: whether a subject
// becomes a record depends on the tier rules, and this is a claim about the
// subject.
//
// The drop gate cannot answer this one: it refuses a message whose every party
// is internal, and this message has a customer on it.
func TestAThreadTheOwnerWroteFromAnAliasIsAboutTheCustomer(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	declareIdentity(t, e, e.Rep1, capturemod.IdentityKindAddress, ownerAlias)

	env.syncSent(t, map[string]bool{"alias1@private.example": true},
		emailCC(ownerAlias, "Lars Private", "alice@acme.example", captureOwner, "alias1@private.example"),
	)

	if n := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'email'`); n != 1 {
		t.Fatalf("%d activity row(s), want 1 — a message naming a customer is not internal", n)
	}
	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty WHERE email = '`+ownerAlias+`'`); n != 0 {
		t.Errorf("the ladder is deciding about the owner's own alias — it was asked whether to create " +
			"a record for the mailbox owner")
	}
	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = '`+ownerAlias+`'`); n != 0 {
		t.Errorf("%d person(s) for the owner's own alias, want 0", n)
	}
}

// The owner is never stamped at both ends of their own message.
//
// The participant rows are what the interaction graph reads as "who talked to
// whom", and they take the connector's DERIVED counterparty rather than the
// ladder's corrected subject — so a message the owner sent from an alias would
// record the owner as the other end of their own exchange.
//
// Nothing promotes such a row to a person: the promotion needs a person_email,
// and the gates above keep an alias from having one. What this prevents is a
// durable falsehood about the exchange.
func TestTheOwnersOwnAliasIsNeverTheOtherEndOfTheirMessage(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	declareIdentity(t, e, e.Rep1, capturemod.IdentityKindAddress, ownerAlias)

	env.syncSent(t, map[string]bool{"alias2@private.example": true},
		emailCC(ownerAlias, "Lars Private", "alice@acme.example", captureOwner, "alias2@private.example"),
	)

	if n := countRows(t, e, `
		SELECT count(*) FROM activity_participant WHERE address = '`+ownerAlias+`'`); n != 0 {
		t.Errorf("%d participant row(s) name the owner's own alias as a party — "+
			"the owner is recorded at both ends of their own message", n)
	}
	// The colleague's address IS still stamped, so the fixture can tell a
	// working rule from a participant writer that stopped working.
	if n := countRows(t, e, `
		SELECT count(*) FROM activity_participant WHERE address = 'alice@acme.example'`); n != 1 {
		t.Errorf("%d participant row(s) for the customer, want 1 — the rule refuses more than the owner's own addresses", n)
	}
}

// The connected mailbox's OWN address is part of the set, and nobody declares
// it — the grant established it.
//
// Without it the gates protect a seat's aliases and leave their primary address
// exposed. On a consumer mailbox the workspace's own domains do not cover that
// address either, so a message between the owner's two addresses stands the
// OWNER in as their own counterparty and the ladder is asked whether to create
// a record for them.
//
// The fixture's mailbox is on the workspace's own domain, so the case is made
// by claiming the alias and asserting the ladder never reaches the owner: with
// the primary address absent from the set, standInSubject picks it.
func TestTheConnectedMailboxsOwnAddressIsNeverTheLaddersSubject(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	declareIdentity(t, e, e.Rep1, capturemod.IdentityKindAddress, ownerAlias)

	// Alias → alias, with the owner's primary address Cc'd. Every party is the
	// owner; nothing external is left for the ladder to be about.
	env.syncSent(t, map[string]bool{"prim1@private.example": true},
		emailCC(ownerAlias, "Lars Private", ownerAlias, captureOwner, "prim1@private.example"),
	)

	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty WHERE email = '`+captureOwner+`'`); n != 0 {
		t.Errorf("the ladder is deciding about the mailbox's own address — a seat's primary address is " +
			"part of who they are, and nobody declares it")
	}
	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = '`+captureOwner+`'`); n != 0 {
		t.Errorf("%d person(s) for the mailbox's own address, want 0", n)
	}
}

// A claim settles the seat's OWN open questions about that address.
//
// Without this the gates bind only mail that arrives from now on: an address
// deferred moments before the claim keeps its pending row, a verdict lands on
// it afterwards, and the seat's own address becomes a contact through exactly
// the door the claim was made to close.
func TestDeclaringAnIdentitySettlesTheOpenQuestionsAboutIt(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e

	// An unattested first message from the alias defers: T1 has no evidence the
	// workspace wrote to it, so the address waits on a verdict.
	env.sync(t, email(ownerAlias, "Lars Private", captureOwner, "pend1@private.example", ""))
	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		 WHERE email = '`+ownerAlias+`' AND status IN ('pending', 'unsure')`); n != 1 {
		t.Fatalf("%d open ledger row(s) for the alias, want 1 — the fixture never reaches the case under test", n)
	}

	declareIdentity(t, e, e.Rep1, capturemod.IdentityKindAddress, ownerAlias)

	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		 WHERE email = '`+ownerAlias+`' AND status IN ('pending', 'unsure')`); n != 0 {
		t.Errorf("%d open ledger row(s) survive the claim — a verdict landing on one of them mints the "+
			"seat's own address as a contact", n)
	}
	// The row STAYS, settled: the Senders surface reads it, and a decision that
	// vanished from it would be one nobody could see or reverse.
	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		 WHERE email = '`+ownerAlias+`' AND status = 'suppressed'`); n != 1 {
		t.Errorf("%d settled ledger row(s), want 1 — the answer must be visible, not absent", n)
	}
}

// A colleague's open question about the same address is theirs. The ledger is
// owner-scoped, and one seat claiming an address must not answer another seat's
// pending decision about it.
func TestAClaimSettlesNoColleaguesOpenQuestion(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e

	env.sync(t, email(ownerAlias, "Lars Private", captureOwner, "pend2@private.example", ""))
	// Re-own the pending row to a colleague, which is the state two connected
	// mailboxes reach when both hear from the same sender.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_pending_counterparty SET owner_id = $1 WHERE email = $2`, e.Rep3, ownerAlias)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	declareIdentity(t, e, e.Rep1, capturemod.IdentityKindAddress, ownerAlias)

	if n := countRows(t, e, `
		SELECT count(*) FROM capture_pending_counterparty
		 WHERE email = '`+ownerAlias+`' AND status IN ('pending', 'unsure')`); n != 1 {
		t.Errorf("Rep1's claim settled Rep3's open question — one seat's claim about their own address " +
			"must not answer a colleague's pending decision")
	}
}
