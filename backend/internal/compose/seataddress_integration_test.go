// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// One rule decides whether an address is a workspace seat's
// (storekit.SeatAddresses). These tests drive it through the writers that act
// on it: the counterparty ensure, the review-queue accept, and an alias claim
// racing a check.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func ensureFromMailbox(t *testing.T, e *integration.Env, owner ids.UUID, address, domain string) error {
	t.Helper()
	mail := seedCapturedMail(t, e, address, "hello")
	_, err := e.Contacts.EnsureCounterparty(e.ConnectorOwnerCtx(owner, "gmail"), contacts.EnsureCounterpartyInput{
		Email: address, DisplayName: "Sender", Domain: domain, OwnerID: owner,
		ActivityID: ids.From[ids.ActivityKind](mail), Source: "gmail:" + mail.String(), CapturedBy: "connector:gmail",
	})
	return err
}

func seatAddressesNow(t *testing.T, e *integration.Env) (storekit.SeatAddresses, map[string][]string) {
	t.Helper()
	var seats storekit.SeatAddresses
	var history map[string][]string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		if seats, err = storekit.WorkspaceSeatAddresses(context.Background(), tx); err != nil {
			return err
		}
		history, err = storekit.MailSeatIdentities(context.Background(), tx)
		return err
	}); err != nil {
		t.Fatalf("reading the seat addresses: %v", err)
	}
	return seats, history
}

// A disconnected mailbox's label proves nothing for a decision made now, for
// any seat, while the history reader still knows the mail it received.
func TestAWithdrawnMailboxLabelDecidesNothingNewAndKeepsItsHistory(t *testing.T) {
	e := integration.Setup(t)
	const address = "rep2.old@mailbox.example"
	connectSeatMailbox(t, e, e.Rep2, address, "shared")
	if err := ensureFromMailbox(t, e, e.Rep1, address, "mailbox.example"); !errors.Is(err, contacts.ErrCounterpartyIsSeat) {
		t.Fatalf("a connected mailbox's address from a colleague's mailbox = %v, want ErrCounterpartyIsSeat", err)
	}

	e.WsExec(t, `UPDATE capture_connection SET status = 'disconnected', auth = NULL WHERE user_id = $1`, e.Rep2)

	seats, history := seatAddressesNow(t, e)
	if seats.Covers(address, ids.Nil) || seats.Covers(address, e.Rep2) {
		t.Fatal("a withdrawn mailbox's label still counts as a seat address for a new decision")
	}
	found := false
	for _, held := range history[e.Rep2.String()] {
		found = found || held == address
	}
	if !found {
		t.Fatalf("the history reader lost the withdrawn mailbox's label: %v", history[e.Rep2.String()])
	}
	if err := ensureFromMailbox(t, e, e.Rep1, address, "mailbox.example"); err != nil {
		t.Fatalf("the address once the mailbox was withdrawn: %v, want an ordinary contact", err)
	}
}

// A mailbox waiting for reauth is not withdrawn: its label stays the seat's
// own address, as a typed one is, and history keeps it.
func TestAMailboxAwaitingReauthKeepsItsLabelAsTheSeatsOwn(t *testing.T) {
	e := integration.Setup(t)
	const address = "rep2.paused@mailbox.example"
	connectSeatMailbox(t, e, e.Rep2, address, "shared")
	e.WsExec(t, `UPDATE capture_connection SET status = 'reauth_required' WHERE user_id = $1`, e.Rep2)

	seats, history := seatAddressesNow(t, e)
	if !seats.Covers(address, e.Rep2) {
		t.Error("a mailbox awaiting reauth lost its label as the seat's own address")
	}
	if seats.Covers(address, ids.Nil) {
		t.Error("a mailbox awaiting reauth still proves its label to every seat")
	}
	if len(history[e.Rep2.String()]) == 0 {
		t.Error("the history reader lost the label of a mailbox awaiting reauth")
	}
}

// The review-queue accept of a seat's address keeps the ledger `real` with the
// seat reason, creates nothing, and releases none of the held mail.
func TestAcceptingASeatAddressInTheReviewQueueCreatesNothingAndReleasesNothing(t *testing.T) {
	e := integration.Setup(t)
	const address = "rep2.private@home.example"
	connectSeatMailbox(t, e, e.Rep2, address, "shared")
	mail := seedCapturedMail(t, e, address, "Wochenende")
	seedPostureHeldImport(t, e, mail, e.Rep1)
	disposition := seedPendingDisposition(t, e, address, "home.example", mail)
	e.WsExec(t, `UPDATE capture_pending_counterparty SET status = 'unsure' WHERE id = $1`, disposition)
	svc := approvalsServiceWithEffects(e.Pool)
	staged, err := stageCounterpartyReview(e.Admin(), svc, approvalSummaryByLang["en"], capture.PendingCounterparty{
		ID: disposition, Email: address, Domain: "home.example", ActivityID: mail, OwnerID: e.Rep1,
	})
	if err != nil {
		t.Fatalf("staging the card: %v", err)
	}
	card := stagedSenderCard{Env: e, svc: svc, approval: ids.From[ids.ApprovalKind](staged), mail: mail}

	if _, err := svc.Decide(card.decider(), card.approval, true, nil); err != nil {
		t.Fatalf("accepting the card: %v", err)
	}
	if status := dispositionStatus(t, e, disposition); status != capture.PendingStatusReal {
		t.Errorf("disposition = %q, want real", status)
	}
	if reason := e.WsScalar(t, `SELECT coalesce(disposition_reason, '') FROM capture_pending_counterparty WHERE id = $1`, disposition); reason != seatAddressReason {
		t.Errorf("disposition reason = %q, want the seat reason", reason)
	}
	if n := contactsAt(t, e, address); n != 0 {
		t.Errorf("%d contacts for a seat's address, want 0", n)
	}
	if posture := importPosture(t, e, mail); posture != "classified" {
		t.Errorf("the held import moved to %q: the accept released a seat's mail as a cleared sender's", posture)
	}
}

// An alias claim of an address is held after its lock. A counterparty check of
// that address must wait for it and then refuse. Without the shared lock the
// check mints the contact first.
func TestACounterpartyCheckWaitsForAnAliasClaimInFlight(t *testing.T) {
	for name, spelling := range map[string]struct{ claimed, checked string }{
		"one spelling":          {"rep1.second@home.example", "rep1.second@home.example"},
		"another case":          {"Rep1.Second@Home.Example", "rep1.second@home.example"},
		"ascii against unicode": {"rep1@xn--bcher-kva.example", "rep1@bücher.example"},
	} {
		t.Run(name, func(t *testing.T) { raceAliasClaimAndCheck(t, spelling.claimed, spelling.checked) })
	}
}

func raceAliasClaimAndCheck(t *testing.T, claimed, checked string) {
	e := integration.Setup(t)
	ctx := context.Background()
	stored, err := capture.ValidExclusionValue(capture.IdentityKindAddress, claimed)
	if err != nil {
		t.Fatalf("the claimed spelling %q is no address: %v", claimed, err)
	}
	mail := seedCapturedMail(t, e, checked, "hello")
	owner := integration.OwnerConn(t)
	gate, err := owner.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the holding transaction: %v", err)
	}
	defer func() {
		if rollbackErr := gate.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			t.Errorf("closing the holding transaction: %v", rollbackErr)
		}
	}()
	// The same claim, uncommitted: the real claim takes its lock and then
	// waits on this row's unique key.
	if _, err := gate.Exec(ctx, `
		INSERT INTO capture_owner_identity (user_id, kind, value, source, created_by)
		VALUES ($1, 'address', $2, 'user', 'human:holder')`, e.Rep1, stored); err != nil {
		t.Fatalf("holding the claim: %v", err)
	}

	claimErr, claimDone := make(chan error, 1), make(chan struct{})
	go func() {
		_, err := capture.NewOwnerIdentityStore(e.DB()).Add(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms),
			capture.IdentityKindAddress, claimed)
		claimErr <- err
		close(claimDone)
	}()
	waitForLockWaiter(t, claimDone, `'transactionid'`, "the alias claim finished without reaching the held row")

	ensureErr, ensureDone := make(chan error, 1), make(chan struct{})
	go func() {
		_, err := e.Contacts.EnsureCounterparty(e.ConnectorOwnerCtx(e.Rep1, "gmail"), contacts.EnsureCounterpartyInput{
			Email: checked, DisplayName: "Rep One", Domain: checked[strings.LastIndex(checked, "@")+1:], OwnerID: e.Rep1,
			ActivityID: ids.From[ids.ActivityKind](mail), Source: "gmail:" + mail.String(), CapturedBy: "connector:gmail",
		})
		ensureErr <- err
		close(ensureDone)
	}()
	waitForLockWaiter(t, ensureDone, `'advisory'`,
		"the counterparty check finished without waiting for the alias claim in flight, so it decided on a stale seat set")

	if err := gate.Rollback(ctx); err != nil {
		t.Fatalf("releasing the held claim: %v", err)
	}
	<-claimDone
	if err := <-claimErr; err != nil {
		t.Fatalf("the alias claim failed: %v", err)
	}
	<-ensureDone
	if err := <-ensureErr; !errors.Is(err, contacts.ErrCounterpartyIsSeat) {
		t.Fatalf("the check after the claim committed = %v, want ErrCounterpartyIsSeat", err)
	}
	if n := contactsAt(t, e, checked); n != 0 {
		t.Fatalf("%d contacts for the alias claimed in flight, want 0", n)
	}
}
