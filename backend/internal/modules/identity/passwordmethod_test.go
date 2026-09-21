// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// silentMailer is a wired transport that sends nothing. It exists so the
// password-door tests can say "this installation HAS recovery machinery" — the
// point being that the machinery is not what decides the answer.
type silentMailer struct{}

func (silentMailer) Send(context.Context, string, string, string) error { return nil }

// capabilitiesOf runs the anonymous probe and reads back the two method flags.
func capabilitiesOf(t *testing.T, h Handlers) (password, passwordReset bool) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.GetAuthCapabilities(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/capabilities", nil))
	var body struct {
		Password      bool `json:"password"`
		PasswordReset bool `json:"password_reset"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return body.Password, body.PasswordReset
}

// An installation that signs its members in through an identity provider closes
// the password door, and the probe the login screen renders from says so. The
// recovery machinery is deliberately wired here and still reports false: a
// deployment with a mail transport and no password method has somewhere to send
// a link and nothing to recover into.
func TestTheProbeReportsAClosedPasswordDoor(t *testing.T) {
	withRecovery := Handlers{}.WithPasswordReset(silentMailer{}).
		WithPasswordLinkBase("https://app.example.com")
	if password, reset := capabilitiesOf(t, withRecovery); !password || !reset {
		t.Fatalf("the fixture reports password=%v reset=%v before the door is closed; "+
			"the assertion below would pass for the wrong reason", password, reset)
	}

	closed := withRecovery.WithPasswordLogin(false)

	password, reset := capabilitiesOf(t, closed)
	if password {
		t.Error("the probe offers the password method at an installation that turned it off — " +
			"the login screen renders a form whose route refuses every submission")
	}
	if reset {
		t.Error("the probe offers self-service password recovery with no password method to recover into")
	}
}

// A handler set nobody told about authentication methods offers the password
// one. Every composition did before the switch existed, and the fail-closed
// default here would be no way in at all rather than a narrower one.
func TestAHandlerSetWithNoAuthWiringStillOffersPassword(t *testing.T) {
	if password, _ := capabilitiesOf(t, Handlers{}); !password {
		t.Error("an unwired handler set withheld the password method")
	}
}

// The route agrees with the probe, and answers before it spends anything: the
// per-IP budget is exhausted first here, so a refusal that ran the throttle
// ahead of the switch would answer 429 and let an installation with no password
// method be pushed into rate-limiting addresses it does not authenticate.
func TestLoginRefusesAtAClosedDoorBeforeItSpendsABudget(t *testing.T) {
	h := NewHandlers(&Service{}).WithPasswordLogin(false)
	for range 40 {
		h.loginPerIP.Allow("192.0.2.9")
	}
	if h.loginPerIP.Allow("192.0.2.9") {
		t.Fatal("the per-IP budget is not exhausted; this test asserts nothing about ordering")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login",
		strings.NewReader(`{"email":"a@b.test","password":"whatever"}`))
	req.RemoteAddr = "192.0.2.9:1234"
	h.Login(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("login answered %d at an installation with no password method, want %d",
			rec.Code, http.StatusNotImplemented)
	}
	if !strings.Contains(rec.Body.String(), "not enabled for this installation") {
		t.Errorf("the refusal does not say why: %s", rec.Body.String())
	}
}

// And self-service recovery, for the same reason: a token minted here would set
// a credential no login route accepts, and it would consume the one recovery
// attempt its owner gets.
func TestForgotPasswordRefusesAtAClosedDoor(t *testing.T) {
	h := NewHandlers(&Service{}).WithPasswordReset(silentMailer{}).
		WithPasswordLinkBase("https://app.example.com").WithPasswordLogin(false)

	rec := httptest.NewRecorder()
	h.RequestPasswordReset(rec, httptest.NewRequest(http.MethodPost, "/v1/auth/forgot-password",
		strings.NewReader(`{"email":"a@b.test"}`)))

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("forgot-password answered %d, want %d — the flow recovers into a method this "+
			"installation does not offer", rec.Code, http.StatusNotImplemented)
	}
}
