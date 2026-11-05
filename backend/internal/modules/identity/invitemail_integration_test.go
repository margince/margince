// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stalledMail struct{ release chan struct{} }

func (m stalledMail) Send(ctx context.Context, _, _, _ string) error {
	select {
	case <-m.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// The relay holds the send until the test releases it, so the handler can
// return only if the send runs off the request.
func TestInviteAnswersWhileTheRelayIsStillSending(t *testing.T) {
	e := setupRevocationEnv(t, "invite-stalled")
	relay := stalledMail{release: make(chan struct{})}
	h := NewHandlers(e.svc).WithPasswordReset(relay).WithPasswordLinkBase("https://crm.example.test")
	sent := make(chan struct{})
	h.resetSendStarted = func() { close(sent) }

	ctx := withIdentity(e.wsCtx(e.admin), e.admin)
	body := `{"email":"slow@acme.test","display_name":"Slow Relay","role":"rep"}`
	rec := httptest.NewRecorder()
	h.InviteUser(rec, httptest.NewRequest(http.MethodPost, "/v1/users", strings.NewReader(body)).WithContext(ctx))
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite status = %d, want 201: %s", rec.Code, rec.Body)
	}
	select {
	case <-sent:
		t.Fatal("the send finished before the relay released it")
	default:
	}
	if row := rosterRow(t, e, h, "slow@acme.test"); !mailState(row, inviteMailSending) {
		t.Errorf("while the relay holds the mail the roster reads %v, want sending", row.InviteEmailState)
	}
	close(relay.release)
	<-sent
	if row := rosterRow(t, e, h, "slow@acme.test"); !mailState(row, inviteMailSent) {
		t.Errorf("after the relay took the mail the roster reads %v, want sent", row.InviteEmailState)
	}
}

// errRelayRefused is what a relay answers for an address it does not know.
var errRelayRefused = errors.New("mailer: relay refused the recipient: 550 5.1.1 no such user")

// refusingMail is a relay that refuses every recipient, keeping the last body
// it was handed.
type refusingMail struct{ body string }

func (m *refusingMail) Send(_ context.Context, _, _, body string) error {
	m.body = body
	return errRelayRefused
}

// mailingHandlers wires mailer, and returns a channel each detached send
// signals once it is done.
func mailingHandlers(e *revocationEnv, relay mailer.Mailer) (Handlers, chan struct{}) {
	h := NewHandlers(e.svc).WithPasswordReset(relay).WithPasswordLinkBase("https://crm.example.test")
	done := make(chan struct{}, 4)
	h.resetSendStarted = func() { done <- struct{}{} }
	return h, done
}

// decodeInvited reads the invitation mail's handoff off a member answer.
func decodeInvited(t *testing.T, rec *httptest.ResponseRecorder) crmcontracts.InviteEmailHandoff {
	t.Helper()
	var out crmcontracts.User
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.InviteEmail == nil {
		t.Fatalf("the answer carries no invite_email: %s", rec.Body)
	}
	return *out.InviteEmail
}

// rosterRow reads one member off the management roster, as the admin.
func rosterRow(t *testing.T, e *revocationEnv, h Handlers, email string) crmcontracts.User {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/users?include_inactive=true", nil).
		WithContext(withIdentity(e.wsCtx(e.admin), e.admin))
	inactive, limit := true, 200
	h.ListUsers(rec, req, crmcontracts.ListUsersParams{IncludeInactive: &inactive, Limit: &limit})
	var page crmcontracts.UserListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	for _, user := range page.Data {
		if string(user.Email) == email {
			return user
		}
	}
	t.Fatalf("%s is not on the roster", email)
	return crmcontracts.User{}
}

func offersResend(user crmcontracts.User) bool {
	return user.AllowedActions != nil &&
		slices.Contains(*user.AllowedActions, crmcontracts.UserAllowedActions(actionResendInvite))
}

func TestARefusedInviteMailShowsOnTheRosterUntilAResendGoesOut(t *testing.T) {
	e := setupRevocationEnv(t, "invite-refused")
	relay := &refusingMail{}
	refusing, refused := mailingHandlers(e, relay)
	const email = "refuse@acme.test"

	ctx := withIdentity(e.wsCtx(e.admin), e.admin)
	rec := httptest.NewRecorder()
	body := `{"email":"` + email + `","display_name":"New Member","role":"rep"}`
	refusing.InviteUser(rec, httptest.NewRequest(http.MethodPost, "/v1/users", strings.NewReader(body)).WithContext(ctx))
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite status = %d, want 201: %s", rec.Code, rec.Body)
	}
	if invited := decodeInvited(t, rec); !invited.Sendable || !invited.Queued {
		t.Fatalf("the invite answered sendable=%v queued=%v, want both true", invited.Sendable, invited.Queued)
	}
	<-refused
	row := rosterRow(t, e, refusing, email)
	if !mailState(row, inviteMailFailed) || !offersResend(row) {
		t.Fatalf("after the relay refused, the roster shows state=%v actions=%v; want failed and a resend",
			row.InviteEmailState, row.AllowedActions)
	}
	firstLink := resetLinkToken.FindStringSubmatch(relay.body)

	mail := &capturedMail{}
	working, sent := mailingHandlers(e, mail)
	rec = httptest.NewRecorder()
	working.ResendUserInvite(rec, httptest.NewRequest(http.MethodPost, "/v1/users/"+row.Id.String()+"/resend-invite", nil).
		WithContext(ctx), row.Id)
	if rec.Code != http.StatusOK {
		t.Fatalf("resend status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if resent := decodeInvited(t, rec); !resent.Queued {
		t.Fatal("the resend answered queued=false on an installation that can send")
	}
	<-sent
	if mail.sent != 1 || mail.to != email || !resetLinkToken.MatchString(mail.body) {
		t.Fatalf("the resend mailed %d message(s) to %q, want one link to %s", mail.sent, mail.to, email)
	}
	if row = rosterRow(t, e, working, email); !mailState(row, inviteMailSent) || !offersResend(row) {
		t.Fatalf("after a resend went out, the roster shows state=%v actions=%v; want sent, and a resend still offered",
			row.InviteEmailState, row.AllowedActions)
	}

	// A late refusal about the first mail lands on a superseded link.
	if len(firstLink) != 2 {
		t.Fatalf("the refused mail carried no link: %q", relay.body)
	}
	if err := e.svc.RecordInviteMailOutcome(e.wsCtx(e.admin), e.admin, firstLink[1], errRelayRefused); err != nil {
		t.Fatal(err)
	}
	if row = rosterRow(t, e, working, email); !mailState(row, inviteMailSent) {
		t.Fatalf("a late refusal of the superseded mail turned the roster to %v, want sent", row.InviteEmailState)
	}
}

func mailState(user crmcontracts.User, want string) bool {
	return user.InviteEmailState != nil && string(*user.InviteEmailState) == want
}

func TestAResendIsRefusedWhereNoInvitationIsWaiting(t *testing.T) {
	e := setupRevocationEnv(t, "invite-resend-refused")
	ctx := withIdentity(e.wsCtx(e.admin), e.admin)
	member := crmcontracts.Id(e.member.UserID.UUID)
	for name, tc := range map[string]struct {
		h    Handlers
		code string
	}{
		"a member who set a password": {
			h:    NewHandlers(e.svc).WithPasswordReset(&capturedMail{}).WithPasswordLinkBase("https://crm.example.test"),
			code: "member_not_invited",
		},
		"an installation that cannot mail": {
			h:    NewHandlers(e.svc).WithPasswordLinkBase("https://crm.example.test"),
			code: "mail_unavailable",
		},
	} {
		rec := httptest.NewRecorder()
		tc.h.ResendUserInvite(rec, httptest.NewRequest(http.MethodPost, "/v1/users/x/resend-invite", nil).WithContext(ctx), member)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), tc.code) {
			t.Errorf("%s: resend answered %d %s, want 409 %s", name, rec.Code, rec.Body, tc.code)
		}
	}
}

// inviteThrough invites email through h, as the admin, and answers its id.
func inviteThrough(t *testing.T, e *revocationEnv, h Handlers, email string) crmcontracts.Id {
	t.Helper()
	rec := httptest.NewRecorder()
	body := `{"email":"` + email + `","display_name":"New Member","role":"rep"}`
	h.InviteUser(rec, httptest.NewRequest(http.MethodPost, "/v1/users", strings.NewReader(body)).
		WithContext(withIdentity(e.wsCtx(e.admin), e.admin)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite status = %d, want 201: %s", rec.Code, rec.Body)
	}
	var user crmcontracts.User
	if err := json.Unmarshal(rec.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	return user.Id
}

// Every invited member can be sent a new invitation, whatever became of the
// last one: a link that expired, a mail that went out but never arrived, a
// member never mailed at all. The new link is the only one left that opens
// the account.
func TestEveryInvitedMemberCanBeSentANewInvitation(t *testing.T) {
	e := setupRevocationEnv(t, "invite-resend-state")
	sent, sentDone := mailingHandlers(e, &capturedMail{})
	sentID := inviteThrough(t, e, sent, "sent@acme.test")
	<-sentDone
	expiredID := inviteThrough(t, e, sent, "expired@acme.test")
	<-sentDone
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE auth_token SET expires_at = now() - interval '1 hour' WHERE user_id = $1`, expiredID); err != nil {
		t.Fatal(err)
	}
	unmailed := NewHandlers(e.svc).WithPasswordLinkBase("https://crm.example.test")
	unmailedID := inviteThrough(t, e, unmailed, "unmailed@acme.test")

	ctx := withIdentity(e.wsCtx(e.admin), e.admin)
	for state, member := range map[string]struct {
		id    crmcontracts.Id
		email string
	}{
		"sent":         {sentID, "sent@acme.test"},
		"expired":      {expiredID, "expired@acme.test"},
		"never mailed": {unmailedID, "unmailed@acme.test"},
	} {
		t.Run(state, func(t *testing.T) {
			mail := &capturedMail{}
			resend, done := mailingHandlers(e, mail)
			if row := rosterRow(t, e, resend, member.email); !offersResend(row) {
				t.Fatalf("the roster offers %v for a %s invitation, want resend_invite", row.AllowedActions, state)
			}
			rec := httptest.NewRecorder()
			resend.ResendUserInvite(rec, httptest.NewRequest(http.MethodPost, "/v1/users/x/resend-invite", nil).
				WithContext(ctx), member.id)
			if rec.Code != http.StatusOK {
				t.Fatalf("a resend of a %s invitation answered %d %s, want 200", state, rec.Code, rec.Body)
			}
			<-done
			link := resetLinkToken.FindStringSubmatch(mail.body)
			if mail.sent != 1 || mail.to != member.email || len(link) != 2 {
				t.Fatalf("the resend mailed %d message(s) to %q, want one link to %s", mail.sent, mail.to, member.email)
			}
			var live int
			var newest bool
			if err := e.owner.QueryRow(context.Background(),
				`SELECT count(*), bool_and(token_hash = $2) FROM auth_token
				  WHERE user_id = $1 AND purpose = 'password_reset' AND used_at IS NULL AND expires_at > now()`,
				member.id, hashToken(link[1])).Scan(&live, &newest); err != nil {
				t.Fatal(err)
			}
			if live != 1 || !newest {
				t.Fatalf("after the resend %d link(s) are live, newest=%v; want only the mailed one", live, newest)
			}
		})
	}
}

// A read that fails before the send leaves the mail unsent, and the admin was
// already told it went out. The link says so, which is what offers the resend.
func TestAnInvitationThatNeverReachedTheRelayIsRecordedAsFailed(t *testing.T) {
	e := setupRevocationEnv(t, "invite-unread")
	h, done := mailingHandlers(e, stalledMail{release: make(chan struct{})})
	h.sendBudget = time.Nanosecond

	inviteThrough(t, e, h, "unread@acme.test")
	<-done
	if row := rosterRow(t, e, h, "unread@acme.test"); !mailState(row, inviteMailFailed) || !offersResend(row) {
		t.Fatalf("after the read before the send failed the roster shows %v %v, want failed and a resend",
			row.InviteEmailState, row.AllowedActions)
	}
}

// A relay that holds the mail past the send's budget fails it. The refusal
// still lands on the link, although the send's own context is spent.
func TestARelayTimeoutStillRecordsTheMailAsFailed(t *testing.T) {
	e := setupRevocationEnv(t, "invite-timeout")
	stalled := stalledMail{release: make(chan struct{})}
	t.Cleanup(func() { close(stalled.release) })
	h, done := mailingHandlers(e, stalled)
	h.sendBudget = 50 * time.Millisecond

	inviteThrough(t, e, h, "slowrelay@acme.test")
	<-done
	if row := rosterRow(t, e, h, "slowrelay@acme.test"); !mailState(row, inviteMailFailed) || !offersResend(row) {
		t.Fatalf("after the send ran out of time the roster shows %v %v, want failed and a resend",
			row.InviteEmailState, row.AllowedActions)
	}
}

// The detached send asks again just before it mails. A link superseded by a
// newer one, and a member deactivated after the link was minted, get no mail;
// the live link goes to the member's current address.
func TestAnInvitationIsMailedOnlyWhileItsLinkAndMemberStillStand(t *testing.T) {
	e := setupRevocationEnv(t, "invite-send-recheck")
	unmailed := NewHandlers(e.svc).WithPasswordLinkBase("https://crm.example.test")
	id := ids.From[ids.UserKind](ids.UUID(inviteThrough(t, e, unmailed, "recheck@acme.test")))
	ctx := e.wsCtx(e.admin)
	mint := func() string {
		t.Helper()
		raw, err := e.svc.ResendInvite(ctx, e.admin, id)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	mail := &capturedMail{}
	h, done := mailingHandlers(e, mail)
	send := func(raw string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/", nil).WithContext(withIdentity(ctx, e.admin))
		if !h.sendInvite(req, "stale@acme.test", raw, e.admin) {
			t.Fatal("the send was not handed over on an installation that can mail")
		}
		<-done
	}

	superseded := mint()
	live := mint()
	send(superseded)
	if mail.sent != 0 {
		t.Fatalf("a superseded link was mailed (%d message(s))", mail.sent)
	}
	send(live)
	if mail.sent != 1 || mail.to != "recheck@acme.test" {
		t.Fatalf("the live link mailed %d message(s) to %q, want one to the member's current address", mail.sent, mail.to)
	}

	minted := mint()
	if err := e.svc.DeactivateUser(ctx, e.admin, DeactivateUserInput{UserID: id}); err != nil {
		t.Fatal(err)
	}
	send(minted)
	if mail.sent != 1 {
		t.Fatalf("a link minted before the member was deactivated was mailed (%d message(s) in all)", mail.sent)
	}
}

// A resend that waits on a redemption in flight reads the member after it.
// The redemption holds the invitation's token and makes the member active;
// the resend, queued behind it, then finds no invitation to send and leaves
// no live link for an active member.
func TestAResendQueuedBehindARedemptionMintsNothing(t *testing.T) {
	e := setupRevocationEnv(t, "invite-resend-race")
	unmailed := NewHandlers(e.svc).WithPasswordLinkBase("https://crm.example.test")
	id := ids.From[ids.UserKind](ids.UUID(inviteThrough(t, e, unmailed, "racer@acme.test")))

	redemption, err := e.owner.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := redemption.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	if _, err := redemption.Exec(context.Background(),
		`SELECT id FROM auth_token WHERE user_id = $1 AND used_at IS NULL FOR UPDATE`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := redemption.Exec(context.Background(),
		`UPDATE app_user SET status = 'active' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := redemption.Exec(context.Background(),
		`UPDATE auth_token SET used_at = now() WHERE user_id = $1 AND used_at IS NULL`, id); err != nil {
		t.Fatal(err)
	}

	resent := make(chan error, 1)
	go func() {
		_, err := e.svc.ResendInvite(e.wsCtx(e.admin), e.admin, id)
		resent <- err
	}()
	waitForABlockedBackend(t, redemption, `locktype = 'transactionid'`)
	if err := redemption.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-resent; !errors.Is(err, errMemberNotInvited) {
		t.Fatalf("a resend queued behind the redemption answered %v, want the member no longer invited", err)
	}
	var live int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM auth_token WHERE user_id = $1 AND used_at IS NULL AND expires_at > now()`, id).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 0 {
		t.Fatalf("an active member holds %d live set-password link(s) after the race", live)
	}
}
