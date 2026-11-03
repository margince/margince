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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailer"
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
	if row = rosterRow(t, e, working, email); !mailState(row, inviteMailSent) || offersResend(row) {
		t.Fatalf("after a resend went out, the roster shows state=%v actions=%v; want sent and no resend",
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
