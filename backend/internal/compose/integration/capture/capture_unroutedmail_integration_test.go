// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Mail in a member's own mailbox that was sent to addresses that are not theirs.
// Only the mailbox owner sees it, counted per address. "I receive this address"
// is a claim that the stranded-mail sweep then picks up. "Not mine" hides it.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	unroutedInfo     = "info@colleague.example"
	unroutedSecurity = "security@colleague.example"
)

// mailboxOwnerCtx is the connector principal that syncs the seat's mailbox.
func mailboxOwnerCtx(e *integration.Env, owner ids.UUID) context.Context {
	ctx := context.Background()
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail",
		UserID: owner, OnBehalfOf: owner,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true},
				"contact":  {Create: true, Read: true, Update: true},
				"company":  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// seatCtx is the signed-in member: a full seat acting for themselves.
func seatCtx(e *integration.Env, user ids.UUID) context.Context {
	ctx := context.Background()
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true, Update: true, Delete: true},
				"contact":  {Read: true, Create: true, Update: true, Delete: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func addParty(t *testing.T, e *integration.Env, activityID ids.UUID, address, role string) {
	t.Helper()
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, $3)`, activityID, address, role)
}

// unroutedMailbox connects a classified mailbox for the seat, so adopted mail
// opens a confidentiality question.
func unroutedMailbox(t *testing.T, e *integration.Env, seat ids.UUID, address string) {
	t.Helper()
	e.GrantRole(t, seat, "rep")
	e.WsExec(t, `INSERT INTO capture_connection (user_id, provider, status, credential_ref, mail_posture, account_label)
		VALUES (@owner, 'gmail', 'connected', 'vault:test', 'classified', @address)`, pgx.NamedArgs{"owner": seat, "address": address})
}

// captureUnrouted lands one inbound mail through the real sink under the
// seat's connector. It names the given recipients and none of the seat's own
// addresses, so the sink stores it and writes no import row.
func captureUnrouted(t *testing.T, e *integration.Env, seat ids.UUID, sourceID string, recipients ...string) ids.UUID {
	t.Helper()
	const sender = "pat@counterparty.example"
	parties := make([]connector.MessageParticipant, 0, len(recipients))
	for _, address := range recipients {
		parties = append(parties, connector.MessageParticipant{Email: address, Role: "to"})
	}
	ref, err := capturemod.NewSink(e.DB()).Upsert(mailboxOwnerCtx(e, seat), connector.NormalizedRecord{
		EntityType:   "activity",
		NaturalKey:   connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: sourceID},
		Counterparty: connector.Counterparty{Email: sender, DisplayName: "Pat", Direction: connector.DirectionInbound},
		Fields: capturemod.ActivityFields{
			Kind: "email", Subject: "Angebot", Body: "Anbei.", Direction: connector.DirectionInbound,
		},
		ThreadKey:    "thread-" + sourceID,
		Addresses:    append([]string{sender}, recipients...),
		Participants: parties,
		Source:       "gmail:" + sourceID, CapturedBy: "connector:gmail",
	})
	if err != nil {
		t.Fatalf("capturing %s: %v", sourceID, err)
	}
	return ref.ID
}

func unroutedStore(e *integration.Env) *capturemod.OwnerIdentityStore {
	return capturemod.NewOwnerIdentityStore(compose.InstallationDB(e.Pool))
}

func unroutedFor(t *testing.T, e *integration.Env, seat ids.UUID) capturemod.UnroutedMail {
	t.Helper()
	mail, err := unroutedStore(e).UnroutedMail(seatCtx(e, seat))
	if err != nil {
		t.Fatalf("listing unrouted mail for %s: %v", seat, err)
	}
	return mail
}

func unroutedCounts(mail capturemod.UnroutedMail) map[string]int {
	out := map[string]int{}
	for _, row := range mail.Addresses {
		out[row.Address] = row.Messages
	}
	return out
}

func sweepStrandedMail(t *testing.T, e *integration.Env) {
	t.Helper()
	ctx := principal.SystemActing(context.Background(), "link-reconcile")
	if _, err := capturemod.NewPendingStore(compose.InstallationDB(e.Pool)).AdoptStrandedMail(ctx, activities.RecomputeAudienceTx, 200); err != nil {
		t.Fatalf("stranded-mail sweep: %v", err)
	}
}

func seatImportCount(t *testing.T, e *integration.Env, activity, seat ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id = $2`, activity, seat)
}

func TestTheMailboxOwnerSeesTheirMailPerAddressAndNobodyElseDoes(t *testing.T) {
	e := integration.Setup(t)
	unroutedMailbox(t, e, e.Rep1, "own1@authz.test")
	unroutedMailbox(t, e, e.Rep2, "own2@authz.test")
	captureUnrouted(t, e, e.Rep1, "u-1", unroutedInfo)
	captureUnrouted(t, e, e.Rep1, "u-2", unroutedInfo, unroutedSecurity)
	captureUnrouted(t, e, e.Rep2, "u-3", "billing@other.example")

	mine := unroutedFor(t, e, e.Rep1)
	if mine.Messages != 2 {
		t.Errorf("messages = %d, want 2 (each message once)", mine.Messages)
	}
	if got := unroutedCounts(mine); len(got) != 2 || got[unroutedInfo] != 2 || got[unroutedSecurity] != 1 {
		t.Errorf("per address = %v, want %s:2 and %s:1", got, unroutedInfo, unroutedSecurity)
	}
	if got := unroutedCounts(unroutedFor(t, e, e.Rep2)); len(got) != 1 || got["billing@other.example"] != 1 {
		t.Errorf("the second member sees %v, want only their own mailbox's address", got)
	}
	if admin, err := unroutedStore(e).UnroutedMail(e.Admin()); err != nil || admin.Messages != 0 {
		t.Errorf("an admin reading the list got %+v, %v; want their own empty list", admin, err)
	}
	if _, err := unroutedStore(e).UnroutedMail(mailboxOwnerCtx(e, e.Rep1)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the mailbox's own connector read the list: %v, want a refusal", err)
	}
}

func TestMailAddressedToTheSeatItselfIsNotCounted(t *testing.T) {
	e := integration.Setup(t)
	unroutedMailbox(t, e, e.Rep1, "own1@authz.test")
	captureUnrouted(t, e, e.Rep1, "u-own", "own1@authz.test", unroutedInfo)

	if mail := unroutedFor(t, e, e.Rep1); mail.Messages != 0 || len(mail.Addresses) != 0 {
		t.Errorf("a message naming the seat's own address is listed: %+v", mail)
	}
}

func TestReceivingAnAddressLetsTheNextSweepJudgeItsMail(t *testing.T) {
	e := integration.Setup(t)
	unroutedMailbox(t, e, e.Rep1, "own1@authz.test")
	unroutedMailbox(t, e, e.Rep2, "own2@authz.test")
	mine := captureUnrouted(t, e, e.Rep1, "u-mine", unroutedInfo)
	other := captureUnrouted(t, e, e.Rep1, "u-other", unroutedSecurity)
	theirs := captureUnrouted(t, e, e.Rep2, "u-theirs", unroutedInfo)

	sweepStrandedMail(t, e)
	if seatImportCount(t, e, mine, e.Rep1) != 0 {
		t.Fatal("mail at an address nobody claimed was imported by the sweep")
	}
	if _, err := unroutedStore(e).AddConfirmed(seatCtx(e, e.Rep1), capturemod.IdentityKindAddress, unroutedInfo, false); err != nil {
		t.Fatalf("claiming %s: %v", unroutedInfo, err)
	}
	if got := unroutedCounts(unroutedFor(t, e, e.Rep1)); got[unroutedInfo] != 0 || got[unroutedSecurity] != 1 {
		t.Errorf("after the claim the list is %v, want only %s", got, unroutedSecurity)
	}

	sweepStrandedMail(t, e)
	if seatImportCount(t, e, mine, e.Rep1) != 1 {
		t.Error("the claimed address's mail was not imported for its seat")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_thread_verdict v JOIN activity a ON a.thread_key = v.thread_key
		 WHERE v.user_id = $1 AND a.id = $2`, e.Rep1, mine); got != 1 {
		t.Errorf("%d confidentiality questions opened for the claimed mail, want 1", got)
	}
	if seatImportCount(t, e, other, e.Rep1) != 0 {
		t.Error("mail at an address that was not claimed was imported")
	}
	if seatImportCount(t, e, theirs, e.Rep1) != 0 {
		t.Error("the claim imported mail that sits in another member's mailbox")
	}
	if seatImportCount(t, e, theirs, e.Rep2) != 0 || len(unroutedFor(t, e, e.Rep2).Addresses) != 1 {
		t.Error("a claim by one member changed the other member's mail")
	}
}

func TestWithdrawingTheClaimListsTheUnjudgedMailAgain(t *testing.T) {
	e := integration.Setup(t)
	unroutedMailbox(t, e, e.Rep1, "own1@authz.test")
	captureUnrouted(t, e, e.Rep1, "u-undo", unroutedInfo)
	identity, err := unroutedStore(e).AddConfirmed(seatCtx(e, e.Rep1), capturemod.IdentityKindAddress, unroutedInfo, false)
	if err != nil {
		t.Fatalf("claiming %s: %v", unroutedInfo, err)
	}
	if err := unroutedStore(e).Remove(seatCtx(e, e.Rep1), identity.ID); err != nil {
		t.Fatalf("withdrawing the claim: %v", err)
	}
	if got := unroutedCounts(unroutedFor(t, e, e.Rep1)); got[unroutedInfo] != 1 {
		t.Errorf("after the withdrawal the list is %v, want %s back", got, unroutedInfo)
	}
}

func TestNotMineHidesTheAddressAndKeepsItsMailUnjudged(t *testing.T) {
	e := integration.Setup(t)
	unroutedMailbox(t, e, e.Rep1, "own1@authz.test")
	unroutedMailbox(t, e, e.Rep2, "own2@authz.test")
	mail := captureUnrouted(t, e, e.Rep1, "u-no", unroutedInfo)
	captureUnrouted(t, e, e.Rep1, "u-no2", unroutedSecurity)
	captureUnrouted(t, e, e.Rep2, "u-no3", unroutedInfo)

	if err := unroutedStore(e).RefuseUnroutedAddress(seatCtx(e, e.Rep1), " INFO@Colleague.example "); err != nil {
		t.Fatalf("answering not mine: %v", err)
	}
	listed := unroutedFor(t, e, e.Rep1)
	if got := unroutedCounts(listed); len(got) != 1 || got[unroutedSecurity] != 1 || listed.Messages != 1 {
		t.Errorf("after not mine the list is %+v, want only %s", listed, unroutedSecurity)
	}
	sweepStrandedMail(t, e)
	if seatImportCount(t, e, mail, e.Rep1) != 0 {
		t.Error("mail at a refused address was imported")
	}
	if got := unroutedCounts(unroutedFor(t, e, e.Rep2)); got[unroutedInfo] != 1 {
		t.Errorf("one member's not mine changed the other member's list: %v", got)
	}

	if err := unroutedStore(e).RestoreUnroutedAddress(seatCtx(e, e.Rep1), unroutedInfo); err != nil {
		t.Fatalf("taking back not mine: %v", err)
	}
	if got := unroutedCounts(unroutedFor(t, e, e.Rep1)); got[unroutedInfo] != 1 {
		t.Errorf("after the undo the list is %v, want %s counted again", got, unroutedInfo)
	}
}

// importedAtHeldAddress reports whether the sweep imported Rep1's mail at an
// address a colleague holds, even though Rep1's claim of it is on file. The
// claim is written directly, as one that predates the colleague's hold. A
// control address Rep1 holds alone is claimed the same way, so a sweep that
// imports nothing at all cannot pass the test.
func importedAtHeldAddress(t *testing.T, e *integration.Env, address string) bool {
	t.Helper()
	const free = "free@colleague.example"
	held := captureUnrouted(t, e, e.Rep1, "u-held", address)
	control := captureUnrouted(t, e, e.Rep1, "u-control", free)
	for _, claimed := range []string{address, free} {
		e.WsExec(t, `INSERT INTO capture_owner_identity (user_id, kind, value, source, created_by)
			VALUES ($1, 'address', $2, 'user', $3)`, e.Rep1, claimed, "human:"+e.Rep1.String())
	}
	sweepStrandedMail(t, e)
	if seatImportCount(t, e, control, e.Rep1) != 1 {
		t.Fatal("the sweep did not import the mail at an address nobody else holds")
	}
	return seatImportCount(t, e, held, e.Rep1)+seatImportCount(t, e, held, e.Rep2) != 0
}

func TestAnAddressAnotherMemberClaimsIsNeverAdopted(t *testing.T) {
	e := integration.Setup(t)
	unroutedMailbox(t, e, e.Rep1, "own1@authz.test")
	unroutedMailbox(t, e, e.Rep2, "own2@authz.test")
	if _, err := unroutedStore(e).AddConfirmed(seatCtx(e, e.Rep2), capturemod.IdentityKindAddress, unroutedInfo, false); err != nil {
		t.Fatalf("the colleague claiming %s: %v", unroutedInfo, err)
	}
	if importedAtHeldAddress(t, e, unroutedInfo) {
		t.Error("mail was imported at an address a second member also claims")
	}
}

func TestAColleaguesLoginIsNeverAdopted(t *testing.T) {
	e := integration.Setup(t)
	unroutedMailbox(t, e, e.Rep1, "own1@authz.test")
	login := e.WsScalar(t, `SELECT lower(email) FROM app_user WHERE id = $1`, e.Rep2)
	if importedAtHeldAddress(t, e, login) {
		t.Error("mail was imported at a colleague's login address")
	}
}

func TestAColleaguesMailboxAddressIsNeverAdopted(t *testing.T) {
	e := integration.Setup(t)
	unroutedMailbox(t, e, e.Rep1, "own1@authz.test")
	unroutedMailbox(t, e, e.Rep2, "own2@authz.test")
	if importedAtHeldAddress(t, e, "own2@authz.test") {
		t.Error("mail was imported at the address a colleague's mailbox is connected as")
	}
}

func TestAnAddressOnAContactAsksBeforeItIsClaimed(t *testing.T) {
	e := integration.Setup(t)
	unroutedMailbox(t, e, e.Rep1, "own1@authz.test")
	const customer = "kunde@customer.example"
	contact := e.SeedContact(t, "Kunde", nil)
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, source, captured_by)
		VALUES ($1, $2, 'manual', 'human:x')`, contact, customer)
	mail := captureUnrouted(t, e, e.Rep1, "u-customer", customer)

	_, err := unroutedStore(e).AddConfirmed(seatCtx(e, e.Rep1), capturemod.IdentityKindAddress, customer, false)
	var question *httperr.DetailedError
	if !errors.As(err, &question) || question.Code != capturemod.CodeNeedsConfirmation {
		t.Fatalf("claiming a contact's address answered %v, want the confirmation question", err)
	}
	sweepStrandedMail(t, e)
	if seatImportCount(t, e, mail, e.Rep1) != 0 {
		t.Error("mail was imported for an address that was never confirmed")
	}
	if _, err := unroutedStore(e).AddConfirmed(seatCtx(e, e.Rep1), capturemod.IdentityKindAddress, customer, true); err != nil {
		t.Fatalf("confirming %s: %v", customer, err)
	}
	sweepStrandedMail(t, e)
	if seatImportCount(t, e, mail, e.Rep1) != 1 {
		t.Error("a confirmed claim of a mail in the member's own mailbox was not imported")
	}
}

func TestTheListIsCutAtTheAddressCapAndSaysSo(t *testing.T) {
	e := integration.Setup(t)
	unroutedMailbox(t, e, e.Rep1, "own1@authz.test")
	message := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, audience)
		VALUES ($1, 'email', 'Rundmail', now(), 'gmail', $2, 'participants')`, message, "connector:gmail:"+e.Rep1.String())
	for i := range 51 {
		addParty(t, e, message, fmt.Sprintf("list-%03d@cap.example", i), "to")
	}

	mail := unroutedFor(t, e, e.Rep1)
	if len(mail.Addresses) != 50 || !mail.More || mail.Messages != 1 {
		t.Errorf("got %d addresses, more=%v, messages=%d; want 50, true, 1",
			len(mail.Addresses), mail.More, mail.Messages)
	}
	if short := unroutedFor(t, e, e.Rep2); short.More {
		t.Error("an empty list claims it was cut")
	}
}

func refusalTrail(t *testing.T, e *integration.Env) (events, audits int) {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'capture_settings.updated'`),
		e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'capture_settings'`)
}

func TestARefusalWritesItsAuditRowAndEventOnlyWhenItChangesSomething(t *testing.T) {
	e := integration.Setup(t)
	store := unroutedStore(e)
	ctx := seatCtx(e, e.Rep1)
	events, audits := refusalTrail(t, e)
	step := func(name string, run func() error, wantEvents, wantAudits int) {
		t.Helper()
		if err := run(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		gotEvents, gotAudits := refusalTrail(t, e)
		if gotEvents-events != wantEvents || gotAudits-audits != wantAudits {
			t.Errorf("%s wrote %d events and %d audit rows, want %d and %d",
				name, gotEvents-events, gotAudits-audits, wantEvents, wantAudits)
		}
		events, audits = gotEvents, gotAudits
	}
	refuse := func() error { return store.RefuseUnroutedAddress(ctx, unroutedInfo) }
	restore := func() error { return store.RestoreUnroutedAddress(ctx, unroutedInfo) }
	step("the first refusal", refuse, 1, 1)
	step("the same refusal again", refuse, 0, 0)
	step("taking it back", restore, 1, 1)
	step("taking it back again", restore, 0, 0)
}
