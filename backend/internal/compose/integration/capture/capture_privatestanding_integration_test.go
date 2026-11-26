// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A sender the classifier judged personal stays withdrawn for the seat that
// owns the verdict. Correspondence in either direction, over any of that seat's
// mailboxes, creates no contact until the seat lifts the standing decision.

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const privateCorrespondent = "dr.keller@clinic-private.example"

// kindVerdict answers every sender it is asked about with one kind.
type kindVerdict struct{ kind string }

func (v kindVerdict) Complete(_ context.Context, req model.Request) (model.Response, error) {
	marker, ok := promptfence.MarkerIn(req.System)
	if !ok {
		return model.Response{}, errors.New("verdict prompt declared no data boundary")
	}
	results := []map[string]any{}
	for _, part := range strings.Split(req.Messages[0].Content, "<"+marker+` id="`)[1:] {
		ref, _, _ := strings.Cut(part, `"`)
		results = append(results, map[string]any{"id": ref, "verdict": v.kind, "confidence": 0.95})
	}
	payload, err := json.Marshal(map[string]any{"results": results})
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: string(payload)}, nil
}

func answerEverySenderAs(t *testing.T, e *integration.SearchEnv, kind string) {
	t.Helper()
	engine := compose.NewCounterpartyVerdictEngine(e.Pool, kindVerdict{kind: kind}, compose.CaptureConfig{}, quietLog())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
}

var dateHeader = regexp.MustCompile(`(?m)^Date: .*\r$`)

// afterTheRetraction dates a message later than the withdrawal. Withdrawing a
// contact releases its address. Only a send dated after that counts as new
// evidence, so a fixture dated in the past never reaches the bug.
func afterTheRetraction(t *testing.T, raw []byte) []byte {
	later := clocktest.Now(t).Add(time.Hour).Format(time.RFC1123Z)
	return dateHeader.ReplaceAll(raw, []byte("Date: "+later+"\r"))
}

// liveContactsFor counts the contacts a seat could see for the address.
func liveContactsFor(t *testing.T, e *integration.SearchEnv, address string) int {
	t.Helper()
	return countRows(t, e, `SELECT count(*) FROM contact p JOIN contact_email pe ON pe.contact_id = p.id
		WHERE pe.email = '`+address+`' AND p.archived_at IS NULL`)
}

func standingFor(t *testing.T, e *integration.SearchEnv, user ids.UUID, address string) int {
	t.Helper()
	return countRows(t, e, `SELECT count(*) FROM capture_sender_override
		WHERE user_id = '`+user.String()+`' AND address = '`+address+`' AND decision = 'personal'`)
}

// mintedThenJudgedPersonal is the history this suite starts from. The seat
// wrote to a stranger and capture made the contact. The classifier then
// called the sender personal and withdrew it.
func mintedThenJudgedPersonal(t *testing.T) captureEnv {
	t.Helper()
	return mintedThenJudgedPersonalAt(t, captureNow)
}

func mintedThenJudgedPersonalAt(t *testing.T, now time.Time) captureEnv {
	t.Helper()
	env := newCaptureEnvAt(t, now)
	env.syncSent(t, map[string]bool{"pv2@myco.example": true},
		email(privateCorrespondent, "Dr Keller", captureOwner, "pv1@clinic-private.example", ""),
		email(captureOwner, "", privateCorrespondent, "pv2@myco.example", "pv1@clinic-private.example"),
	)
	if n := liveContactsFor(t, env.e, privateCorrespondent); n != 1 {
		t.Fatalf("%d contacts before the verdict, want 1: the case is not set up", n)
	}
	answerEverySenderAs(t, env.e, capturemod.KindPersonal)
	if n := liveContactsFor(t, env.e, privateCorrespondent); n != 0 {
		t.Fatalf("%d live contacts after a personal verdict, want it withdrawn", n)
	}
	if n := standingFor(t, env.e, env.e.Rep1, privateCorrespondent); n != 1 {
		t.Fatalf("%d standing decisions after a personal verdict, want 1", n)
	}
	return env
}

func TestMailTheSeatSendsToAPrivateSenderMakesNoContact(t *testing.T) {
	env := mintedThenJudgedPersonal(t)

	env.syncSent(t, map[string]bool{"pv3@myco.example": true},
		afterTheRetraction(t, email(captureOwner, "", privateCorrespondent, "pv3@myco.example", "pv1@clinic-private.example")))

	if n := liveContactsFor(t, env.e, privateCorrespondent); n != 0 {
		t.Fatalf("%d contacts after the seat wrote to a private sender, want 0", n)
	}
}

func TestAReplyFromAPrivateSenderMakesNoContact(t *testing.T) {
	env := mintedThenJudgedPersonal(t)

	env.syncSent(t, map[string]bool{"pv3@myco.example": true},
		afterTheRetraction(t, email(captureOwner, "", privateCorrespondent, "pv3@myco.example", "pv1@clinic-private.example")),
		afterTheRetraction(t, email(privateCorrespondent, "Dr Keller", captureOwner, "pv4@clinic-private.example", "pv3@myco.example")))

	if n := liveContactsFor(t, env.e, privateCorrespondent); n != 0 {
		t.Fatalf("%d contacts after a private sender replied, want 0", n)
	}
	if n := countRows(t, env.e, `SELECT count(*) FROM activity WHERE source_id = 'pv4@clinic-private.example'`); n != 1 {
		t.Fatalf("%d activities for the reply, want 1: the mail is kept", n)
	}
}

func TestAMeetingWithAPrivateSenderMakesNoContact(t *testing.T) {
	env := mintedThenJudgedPersonal(t)

	// A meeting counts once it has taken place, and only if it is dated after
	// the withdrawal. The withdrawal is moved back so such a meeting can exist.
	if err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE contact_email SET archived_at = now() - interval '2 days'
			WHERE email = $1 AND archived_at IS NOT NULL`, privateCorrespondent)
		return err
	}); err != nil {
		t.Fatalf("dating the withdrawal two days back: %v", err)
	}
	meeting := clocktest.Now(t).Add(-3 * time.Hour)
	syncGcalEventsThrough(t, env.e, env.registry, captureOwner, []map[string]any{{
		"id": "pv5@myco.example", "status": "confirmed", "visibility": "default", "summary": "Follow-up",
		"start":     map[string]string{"dateTime": meeting.Format(time.RFC3339)},
		"end":       map[string]string{"dateTime": meeting.Add(time.Hour).Format(time.RFC3339)},
		"organizer": map[string]any{"email": captureOwner, "self": true},
		"attendees": []map[string]any{{"email": captureOwner, "self": true, "responseStatus": "accepted"}, {"email": privateCorrespondent, "displayName": "Dr Keller", "responseStatus": "accepted"}, {"email": "control@clinic-private.example", "displayName": "Control", "responseStatus": "accepted"}},
	}})
	if n := liveContactsFor(t, env.e, "control@clinic-private.example"); n != 1 {
		t.Fatalf("%d contacts for the control guest, want 1: the meeting does not reach the ladder", n)
	}

	if n := liveContactsFor(t, env.e, privateCorrespondent); n != 0 {
		t.Fatalf("%d contacts after a meeting with a private sender, want 0", n)
	}
}

// connectFor connects one more mailbox for a seat and answers a sync function
// that attests the listed Message-IDs as the seat's own sent mail.
func connectFor(t *testing.T, env captureEnv, user ids.UUID, provider, label, parseAs string) func(*testing.T, map[string]bool, ...[]byte) {
	t.Helper()
	seedCaptureRoleFor(t, env.e, user)
	conn := &mailBatchConnector{accountLabel: label, name: provider, parseAs: parseAs}
	registry := compose.NewCaptureRegistry(env.e.Pool, newTestKeyvault(t, env.e), compose.CaptureConfig{})
	registry.Register(conn)
	id, err := registry.Connect(humanWithScopes(env.e, user, []principal.Scope{principal.ScopeRead}), provider, connector.Auth("refresh"))
	if err != nil {
		t.Fatalf("connecting %s: %v", label, err)
	}
	wsCtx := principal.WithWorkspaceID(context.Background(), env.e.WS)
	return func(t *testing.T, sent map[string]bool, raws ...[]byte) {
		t.Helper()
		conn.raws, conn.sent = raws, sent
		if err := registry.SyncOnce(wsCtx, id); err != nil {
			t.Fatalf("SyncOnce for %s: %v", label, err)
		}
	}
}

func TestTheWithdrawalHoldsForASecondMailboxOfTheSameSeat(t *testing.T) {
	env := mintedThenJudgedPersonal(t)
	second := connectFor(t, env, env.e.Rep1, "graph", "owner.graph@myco.example", captureOwner)

	second(t, map[string]bool{"pv6@myco.example": true},
		afterTheRetraction(t, email(captureOwner, "", privateCorrespondent, "pv6@myco.example", "")),
		afterTheRetraction(t, email(privateCorrespondent, "Dr Keller", captureOwner, "pv7@clinic-private.example", "pv6@myco.example")))

	if n := liveContactsFor(t, env.e, privateCorrespondent); n != 0 {
		t.Fatalf("%d contacts after the second mailbox saw the correspondence, want 0", n)
	}
}

// The decision is the seat's own: a colleague who does business with the same
// address keeps the contact their own correspondence makes.
func TestAnotherSeatsMailboxStillMakesItsContact(t *testing.T) {
	env := mintedThenJudgedPersonal(t)
	other := connectFor(t, env, env.e.Rep3, "graph", secondSeatAddress, secondSeatAddress)

	other(t, map[string]bool{"pv8@myco.example": true},
		afterTheRetraction(t, email(privateCorrespondent, "Dr Keller", secondSeatAddress, "pv9@clinic-private.example", "")),
		afterTheRetraction(t, email(secondSeatAddress, "", privateCorrespondent, "pv8@myco.example", "pv9@clinic-private.example")))

	if n := liveContactsFor(t, env.e, privateCorrespondent); n != 1 {
		t.Fatalf("%d contacts from the other seat's own correspondence, want 1", n)
	}
	if n := standingFor(t, env.e, env.e.Rep3, privateCorrespondent); n != 0 {
		t.Fatalf("the other seat holds %d standing decisions, want none: the verdict was the first seat's", n)
	}
}

func TestLiftingTheStandingDecisionLetsCorrespondenceMakeTheContact(t *testing.T) {
	env := mintedThenJudgedPersonal(t)

	if err := capturemod.NewSenderOverrideStore(env.e.DB()).Remove(ownerDeskCtx(env.e), privateCorrespondent); err != nil {
		t.Fatalf("lifting the standing decision: %v", err)
	}
	env.syncSent(t, map[string]bool{"pv10@myco.example": true},
		afterTheRetraction(t, email(captureOwner, "", privateCorrespondent, "pv10@myco.example", "pv1@clinic-private.example")))

	if n := liveContactsFor(t, env.e, privateCorrespondent); n != 1 {
		t.Fatalf("%d contacts after the lift, want 1: the ladder works as before", n)
	}
}

func TestAnAgentCannotLiftAPrivateSendersStanding(t *testing.T) {
	env := mintedThenJudgedPersonal(t)
	ctx := principal.WithActor(ownerDeskCtx(env.e), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + env.e.Rep1.String(), UserID: env.e.Rep1,
		SeatType: principal.SeatFull,
	})

	err := capturemod.NewSenderOverrideStore(env.e.DB()).Remove(ctx, privateCorrespondent)

	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an agent lifting a private sender got %v, want permission denied", err)
	}
	if n := standingFor(t, env.e, env.e.Rep1, privateCorrespondent); n != 1 {
		t.Fatalf("%d standing decisions after the refused lift, want 1", n)
	}
}

// A decision the seat typed outranks the verdict that comes later.
func TestAPersonalVerdictDoesNotOverwriteTheSeatsOwnDecision(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	if _, err := capturemod.NewSenderOverrideStore(e.DB()).Set(ownerDeskCtx(e), privateCorrespondent, capturemod.OverrideBusiness); err != nil {
		t.Fatalf("recording the seat's decision: %v", err)
	}

	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return capturemod.RecordPersonalStandingTx(e.Admin(), tx, e.Rep1, privateCorrespondent)
	}); err != nil {
		t.Fatalf("recording the verdict's standing: %v", err)
	}

	if n := countRows(t, e, `SELECT count(*) FROM capture_sender_override
		WHERE address = '`+privateCorrespondent+`' AND decision = 'business'`); n != 1 {
		t.Fatalf("%d business decisions kept, want the seat's own to stand", n)
	}
}

// A noise verdict is a stale answer the seat's own correspondence recovers from.
func TestACorrespondedNoiseSenderStillBecomesAContact(t *testing.T) {
	env := newCaptureEnv(t)
	env.syncSent(t, map[string]bool{"pn2@myco.example": true},
		email(privateCorrespondent, "Dr Keller", captureOwner, "pn1@clinic-private.example", ""))
	answerEverySenderAs(t, env.e, capturemod.KindNewsletter)

	env.syncSent(t, map[string]bool{"pn3@myco.example": true},
		afterTheRetraction(t, email(captureOwner, "", privateCorrespondent, "pn3@myco.example", "pn1@clinic-private.example")))

	if n := liveContactsFor(t, env.e, privateCorrespondent); n != 1 {
		t.Fatalf("%d contacts for a sender judged noise that the seat then wrote to, want 1", n)
	}
	if n := countRows(t, env.e, `SELECT count(*) FROM capture_sender_override WHERE address = '`+privateCorrespondent+`'`); n != 0 {
		t.Fatalf("%d standing decisions for a noise verdict, want none", n)
	}
}

func TestThePrivateStandingBackfillCoversVerdictsFromBeforeItWasRecorded(t *testing.T) {
	env := mintedThenJudgedPersonal(t)
	e := env.e
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `DELETE FROM capture_sender_override WHERE address = $1`, privateCorrespondent)
		return err
	}); err != nil {
		t.Fatalf("removing the standing decision to model an older verdict: %v", err)
	}

	for pass, want := range []int{1, 0} {
		var written int
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			var err error
			_, written, err = capturemod.BackfillPersonalStandingsTx(principal.SystemActing(e.Admin(), "private-standing-backfill"), tx, ids.Nil, 50)
			return err
		}); err != nil {
			t.Fatalf("backfill pass %d: %v", pass, err)
		}
		if written != want {
			t.Fatalf("backfill pass %d wrote %d, want %d", pass, written, want)
		}
	}
	if n := standingFor(t, e, e.Rep1, privateCorrespondent); n != 1 {
		t.Fatalf("%d standing decisions after the backfill, want 1", n)
	}
}
