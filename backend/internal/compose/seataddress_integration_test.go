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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
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
	mail := seedCapturedMail(t, e, checked, "hello")
	release, claimErr := claimInFlight(t, e, claimed)

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

	release()
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

// claimInFlight starts Rep1's alias claim of address and holds it after its
// lock. An uncommitted copy of the row makes the claim wait on its unique key.
// release lets it commit.
func claimInFlight(t *testing.T, e *integration.Env, address string) (release func(), claimed <-chan error) {
	t.Helper()
	ctx := context.Background()
	gate, err := integration.OwnerConn(t).Begin(ctx)
	if err != nil {
		t.Fatalf("opening the holding transaction: %v", err)
	}
	t.Cleanup(func() {
		if rollbackErr := gate.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			t.Errorf("closing the holding transaction: %v", rollbackErr)
		}
	})
	stored, err := capture.ValidExclusionValue(capture.IdentityKindAddress, address)
	if err != nil {
		t.Fatalf("the claimed spelling %q is no address: %v", address, err)
	}
	if _, err := gate.Exec(ctx, `
		INSERT INTO capture_owner_identity (user_id, kind, value, source, created_by)
		VALUES ($1, 'address', $2, 'user', 'human:holder')`, e.Rep1, stored); err != nil {
		t.Fatalf("holding the claim: %v", err)
	}
	result, done := make(chan error, 1), make(chan struct{})
	go func() {
		_, err := capture.NewOwnerIdentityStore(e.DB()).Add(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms),
			capture.IdentityKindAddress, address)
		result <- err
		close(done)
	}()
	waitForLockWaiter(t, done, `'transactionid'`, "the alias claim finished without reaching the held row")
	return func() {
		if err := gate.Rollback(ctx); err != nil {
			t.Fatalf("releasing the held claim: %v", err)
		}
	}, result
}

// A mail captured while its sender's address is being claimed as Rep1's own
// waits for the claim. The ledger then asks nothing about the address, and no
// participant row names it as an outside party.
func TestACaptureWaitsForAnAliasClaimOfItsSender(t *testing.T) {
	e := setupWithOwnMailbox(t)
	const alias = "rep1.home@home.example"
	release, claimed := claimInFlight(t, e, alias)

	captured, done := make(chan ids.UUID, 1), make(chan struct{})
	go func() {
		defer close(done)
		ref, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(mailboxOwnerCtx(e, e.Rep1), connector.NormalizedRecord{
			EntityType: "activity",
			NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "alias-race-1"},
			Counterparty: connector.Counterparty{
				Email: alias, DisplayName: "Rep One", Domain: "home.example", Direction: connector.DirectionInbound,
			},
			Fields:    capture.ActivityFields{Kind: "email", Subject: "Angebot", Body: "Anbei.", Direction: connector.DirectionInbound},
			ThreadKey: "thread-alias-race-1",
			Addresses: []string{alias, partnerBuyer, "a@authz.test"},
			Source:    "gmail:alias-race-1", CapturedBy: "connector:gmail",
		})
		if err != nil {
			t.Errorf("capturing the mail: %v", err)
		}
		captured <- ref.ID
	}()
	waitForLockWaiter(t, done, `'advisory'`, "the capture finished without waiting for the alias claim of its sender")

	release()
	if err := <-claimed; err != nil {
		t.Fatalf("the alias claim failed: %v", err)
	}
	<-done
	mail := <-captured
	if n := e.WsCount(t, `SELECT count(*) FROM capture_pending_counterparty WHERE email = $1`, alias); n != 0 {
		t.Errorf("the ledger holds %d question(s) about the seat's own alias, want none", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND address = $2`, mail, alias); n != 0 {
		t.Errorf("the alias was stamped %d time(s) as an outside party, want 0", n)
	}
	if n := contactsAt(t, e, alias); n != 0 {
		t.Errorf("%d contacts for the seat's own alias, want 0", n)
	}
}

// A ledger row retires to `unsure` while its address is claimed. The retire
// takes the address's check lock first, so the claim waits for it. The claim
// then settles the row instead of leaving an open review behind.
func TestAnAliasClaimSettlesARowRetiringToUnsure(t *testing.T) {
	e := integration.Setup(t)
	ctx := context.Background()
	const alias = "rep1.retiring@home.example"
	mail := seedCapturedMail(t, e, alias, "hello")
	row := seedPendingDisposition(t, e, alias, "home.example", mail)
	lease := ids.NewV7()
	e.WsExec(t, `UPDATE capture_pending_counterparty SET claimed_by = $2, claimed_until = now() + interval '1 hour' WHERE id = $1`, row, lease)

	gate, err := integration.OwnerConn(t).Begin(ctx)
	if err != nil {
		t.Fatalf("opening the holding transaction: %v", err)
	}
	defer func() {
		if rollbackErr := gate.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			t.Errorf("closing the holding transaction: %v", rollbackErr)
		}
	}()
	if _, err := gate.Exec(ctx, `SELECT id FROM capture_pending_counterparty WHERE id = $1 FOR UPDATE`, row); err != nil {
		t.Fatalf("holding the row: %v", err)
	}
	retired, retireDone := make(chan error, 1), make(chan struct{})
	go func() {
		retired <- capture.NewPendingStore(InstallationDB(e.Pool)).Retire(principal.WithWorkspaceID(ctx, e.WS),
			capture.PendingCounterparty{ID: row, Email: alias, Claim: lease}, "never cleared the floor", capture.VerdictMeasurement{})
		close(retireDone)
	}()
	waitForLockWaiter(t, retireDone, `'tuple', 'transactionid'`, "the retire finished without reaching the held row")

	claimed, claimDone := make(chan error, 1), make(chan struct{})
	go func() {
		_, err := capture.NewOwnerIdentityStore(e.DB()).Add(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms),
			capture.IdentityKindAddress, alias)
		claimed <- err
		close(claimDone)
	}()
	waitForLockWaiter(t, claimDone, `'advisory'`, "the alias claim finished without waiting for the retire's check lock")

	if err := gate.Commit(ctx); err != nil {
		t.Fatalf("releasing the row: %v", err)
	}
	if err := <-retired; err != nil {
		t.Fatalf("the retire failed: %v", err)
	}
	if err := <-claimed; err != nil {
		t.Fatalf("the alias claim failed: %v", err)
	}
	if status := dispositionStatus(t, e, row); status != "suppressed" {
		t.Fatalf("the row ended %q, want suppressed: the claim left an open review behind", status)
	}
}
