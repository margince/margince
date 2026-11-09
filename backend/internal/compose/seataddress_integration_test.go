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
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
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

// invitedColleague invites address through the real invite writer, after the
// installation describes its company as an invite requires.
func invitedColleague(t *testing.T, e *integration.Env, address string) (*identity.Service, ids.UserID, string) {
	t.Helper()
	users := identity.NewServiceFor(e.DB())
	company := contacts.NewStore(e.DB()).WithInstallationRenamer(installationRenamer(e.Pool))
	if _, err := company.SaveCompany(e.Admin(), contacts.SaveCompanyInput{DisplayName: "Example company"}); err != nil {
		t.Fatal(err)
	}
	identity.NewHandlers(users).WithInstallationDescribed(installationDescribed(company))
	seat, token, err := users.InviteUser(e.Admin(), recoveryAdminIdentity(e), identity.InviteUserInput{
		Email: address, DisplayName: "Invited colleague", Role: "rep",
	})
	if err != nil {
		t.Fatal(err)
	}
	return users, seat, token
}

// isInternal answers whether capture and the counterparty ensure both treat
// address as a seat's, and fails when the two disagree.
func isInternal(t *testing.T, e *integration.Env, address string) bool {
	t.Helper()
	seats, _ := seatAddressesNow(t, e)
	err := ensureFromMailbox(t, e, e.Rep1, address, strings.SplitN(address, "@", 2)[1])
	refused := errors.Is(err, contacts.ErrCounterpartyIsSeat)
	if err != nil && !refused {
		t.Fatalf("ensuring %s as a counterparty: %v", address, err)
	}
	if refused != seats.Covers(address, ids.Nil) {
		t.Fatalf("the seat set says internal=%v for %s and the counterparty ensure says %v", seats.Covers(address, ids.Nil), address, refused)
	}
	return refused
}

// An invited colleague's login is internal from the invitation on. The
// invitation grants no stored mailbox history; redeeming it does.
func TestAnInvitedColleaguesLoginIsInternalBeforeAndAfterActivation(t *testing.T) {
	e := integration.Setup(t)
	const address = "invited@colleague.example"
	users, seat, token := invitedColleague(t, e, address)
	seats, history := seatAddressesNow(t, e)
	if !isInternal(t, e, address) || !seats.IsLogin(address) {
		t.Fatal("an invited colleague's login was classified as external")
	}
	if len(history[seat.String()]) != 0 {
		t.Fatal("the invitation granted access to stored mailbox history")
	}
	if len(answeringSeats(t, e)[seat.String()]) != 0 {
		t.Fatal("the invitation made the colleague responsible for answering mail")
	}
	if err := users.RedeemPasswordReset(e.Admin(), token, "Invited-colleague-strong-password-42!"); err != nil {
		t.Fatal(err)
	}
	_, history = seatAddressesNow(t, e)
	if !isInternal(t, e, address) || len(history[seat.String()]) == 0 {
		t.Fatal("activation changed the internal identity or lost its live mailbox identity")
	}
}

// An invited login reserves its address. Another seat's typed alias for it no
// longer makes that seat answer its mail, and the invitation owes no answer.
func TestAnInvitedLoginReservesItsAddressFromAnotherSeatsAlias(t *testing.T) {
	e := integration.Setup(t)
	const address = "shared@colleague.example"
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := capture.NewOwnerIdentityStore(e.DB()).Add(rep, capture.IdentityKindAddress, address); err != nil {
		t.Fatal(err)
	}
	if !seatAnswers(answeringSeats(t, e), e.Rep1.String(), address) {
		t.Fatal("the alias did not make its seat answer the address; this case proves nothing")
	}
	invitedColleague(t, e, address)
	for seat, addresses := range answeringSeats(t, e) {
		if seatAnswers(map[string][]string{seat: addresses}, seat, address) {
			t.Fatalf("seat %s answers %s after it became an invited login", seat, address)
		}
	}
}

func seatAnswers(seats map[string][]string, seat, address string) bool {
	for _, held := range seats[seat] {
		if strings.EqualFold(held, address) {
			return true
		}
	}
	return false
}

// A revoked invitation makes the address an outside party again.
func TestARevokedInvitationMakesTheAddressOutsideAgain(t *testing.T) {
	e := integration.Setup(t)
	const address = "revoked@colleague.example"
	users, seat, _ := invitedColleague(t, e, address)
	if !isInternal(t, e, address) {
		t.Fatal("the invitation did not make the address internal; this case proves nothing")
	}
	if err := users.DeactivateUser(e.Admin(), recoveryAdminIdentity(e), identity.DeactivateUserInput{UserID: seat}); err != nil {
		t.Fatal(err)
	}
	if isInternal(t, e, address) {
		t.Fatal("a revoked invitation still counts its address as internal")
	}
}

// An invitation whose email is corrected before redemption moves the internal
// address with it.
func TestAnInvitationsChangedEmailMovesTheInternalAddress(t *testing.T) {
	e := integration.Setup(t)
	const before, after = "typo@colleague.example", "fixed@colleague.example"
	users, seat, _ := invitedColleague(t, e, before)
	if err := users.UpdateMember(e.Admin(), recoveryAdminIdentity(e), seat, identity.MemberEdit{
		Name: "Invited colleague", Email: after, ExpectedUpdatedAt: memberUpdatedAt(t, e, seat),
	}); err != nil {
		t.Fatal(err)
	}
	if isInternal(t, e, before) || !isInternal(t, e, after) {
		t.Fatal("the internal address did not move with the invitation's email")
	}
	// The email change ends the first invitation's token, so a new one is sent.
	token, err := users.ResendInvite(e.Admin(), recoveryAdminIdentity(e), seat)
	if err != nil {
		t.Fatal(err)
	}
	if err := users.RedeemPasswordReset(e.Admin(), token, "Invited-colleague-strong-password-42!"); err != nil {
		t.Fatal(err)
	}
	if !isInternal(t, e, after) {
		t.Fatal("redeeming the corrected invitation lost its internal address")
	}
}

// answeringSeats is the set that decides who owes an answer to a mail.
func answeringSeats(t *testing.T, e *integration.Env) map[string][]string {
	t.Helper()
	var seats map[string][]string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		seats, err = storekit.UnambiguousMailSeatIdentities(context.Background(), tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return seats
}

func memberUpdatedAt(t *testing.T, e *integration.Env, member ids.UserID) time.Time {
	t.Helper()
	var at time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT updated_at FROM app_user WHERE id = $1`, member).Scan(&at)
	}); err != nil {
		t.Fatal(err)
	}
	return at
}

// Each writer that adds, moves or withdraws an invited login waits for a seat
// check of that address already in flight. The check never sees half of it.
func TestInvitationWritersWaitForASeatCheckInFlight(t *testing.T) {
	e := integration.Setup(t)
	users, seat, _ := invitedColleague(t, e, "held@colleague.example")
	admin := recoveryAdminIdentity(e)
	manager := memberManagerFor(t, e)
	var second ids.UserID
	// In order: each case leaves the seat in the state the next one needs.
	for _, write := range []struct {
		name, address string
		prepare, run  func() error
	}{
		{"invite", "second@colleague.example", nil, func() error {
			var err error
			second, _, err = users.InviteUser(e.Admin(), admin, identity.InviteUserInput{Email: "second@colleague.example", DisplayName: "Second", Role: "rep"})
			return err
		}},
		{"email change, new address", "interim@colleague.example", nil, func() error {
			return users.UpdateMember(e.Admin(), admin, seat, identity.MemberEdit{
				Name: "Invited colleague", Email: "interim@colleague.example", ExpectedUpdatedAt: memberUpdatedAt(t, e, seat),
			})
		}},
		{"email change, old address", "interim@colleague.example", nil, func() error {
			return users.UpdateMember(e.Admin(), admin, seat, identity.MemberEdit{
				Name: "Invited colleague", Email: "moved@colleague.example", ExpectedUpdatedAt: memberUpdatedAt(t, e, seat),
			})
		}},
		{"revoke", "moved@colleague.example", nil, func() error {
			return users.DeactivateUser(e.Admin(), admin, identity.DeactivateUserInput{UserID: seat})
		}},
		{"reactivate", "second@colleague.example", func() error {
			return users.DeactivateUser(e.Admin(), admin, identity.DeactivateUserInput{UserID: second})
		}, func() error {
			return users.ReactivateUser(e.Admin(), admin, second)
		}},
		{"retire", "moved@colleague.example", nil, func() error {
			return retireMember(e, manager, seat)
		}},
	} {
		t.Run(write.name, func(t *testing.T) {
			if write.prepare != nil {
				if err := write.prepare(); err != nil {
					t.Fatal(err)
				}
			}
			writerWaitsForCheck(t, e, write.address, write.run)
		})
	}
}

// retireMember removes the member through the composed retirement, after the
// preview its confirmation is staged from.
func retireMember(e *integration.Env, manager memberManager, member ids.UserID) error {
	ctx := e.As(e.AdminUser, nil, integration.AdminWithSignals)
	preview, err := manager.Preview(ctx, member.UUID, nil, false)
	if err != nil {
		return err
	}
	actor := identity.Identity{UserID: ids.From[ids.UserKind](e.AdminUser), Roles: []string{"admin"}, SeatType: "full", Permissions: integration.AdminWithSignals}
	return manager.service.RetireMember(ctx, actor, member, nil, preview.Token, false)
}

// writerWaitsForCheck holds a seat check of address open, runs write, and
// insists it queues behind the check and completes once the check ends.
func writerWaitsForCheck(t *testing.T, e *integration.Env, address string, write func() error) {
	t.Helper()
	ctx := context.Background()
	conn, err := e.Pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Release)
	check, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := check.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("ending the seat check: %v", err)
		}
	})
	if err := storekit.LockSeatAddressChecks(ctx, check, address); err != nil {
		t.Fatal(err)
	}
	holder := conn.Conn().PgConn().PID()
	probe, err := e.Pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(probe.Release)
	done, returned := make(chan error, 1), make(chan struct{})
	go func() { defer close(returned); done <- write() }()
	testdb.WaitForContention(t, returned, "the writer finished while the seat check held its address",
		"the writer never queued behind the seat check", func(ctx context.Context) (bool, error) {
			// pg_stat_activity is cached per transaction until cleared.
			if _, err := probe.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
				return false, err
			}
			var queued bool
			err := probe.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity a WHERE $1 = ANY (pg_blocking_pids(a.pid)))`, holder).Scan(&queued)
			return queued, err
		})
	if err := check.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("the writer failed once the check ended: %v", err)
	}
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
	svc := NewApprovalsServiceWithEffects(e.Pool)
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
