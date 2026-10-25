// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

type refusingMail struct{}

func (refusingMail) Send(context.Context, string, string, string) error {
	return errors.New("relay refused the recipient: 550")
}

func inviteVia(t *testing.T, e *revocationEnv, h Handlers, email string) *httptest.ResponseRecorder {
	t.Helper()
	ctx := withIdentity(e.wsCtx(e.admin), e.admin)
	body := `{"email":"` + email + `","display_name":"New Member","role":"rep"}`
	rec := httptest.NewRecorder()
	h.InviteUser(rec, httptest.NewRequest(http.MethodPost, "/v1/users", strings.NewReader(body)).WithContext(ctx))
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite status = %d, want 201: %s", rec.Code, rec.Body)
	}
	return rec
}

func TestInviteAnswersWhileTheRelayIsStillSending(t *testing.T) {
	e := setupRevocationEnv(t, "invite-stalled")
	relay := stalledMail{release: make(chan struct{})}
	h := NewHandlers(e.svc).WithPasswordReset(relay).WithPasswordLinkBase("https://crm.example.test")
	sent := make(chan struct{})
	h.resetSendStarted = func() { close(sent) }

	answered := make(chan struct{})
	go func() {
		inviteVia(t, e, h, "slow@acme.test")
		close(answered)
	}()
	select {
	case <-answered:
	case <-time.After(10 * time.Second):
		t.Fatal("invite did not answer while the relay was still holding the send")
	}
	close(relay.release)
	<-sent
}

func TestRefusedInviteMailShowsOnTheRosterRow(t *testing.T) {
	e := setupRevocationEnv(t, "invite-refused")
	h := NewHandlers(e.svc).WithPasswordReset(refusingMail{}).WithPasswordLinkBase("https://crm.example.test")
	sent := make(chan struct{})
	h.resetSendStarted = func() { close(sent) }

	rec := inviteVia(t, e, h, "refused@acme.test")
	<-sent

	limit := 100
	page, err := e.svc.ListUsers(withIdentity(e.wsCtx(e.admin), e.admin),
		ListUsersInput{IncludeInactive: true, Limit: &limit, Actor: &e.admin})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	failed := map[string]bool{}
	for _, u := range page.Users {
		if u.InviteEmailFailed == nil {
			t.Fatalf("management view dropped invite_email_failed for %s", u.Email)
		}
		failed[u.Email] = *u.InviteEmailFailed
	}
	if !failed["refused@acme.test"] {
		t.Errorf("roster flags = %v, want refused@acme.test flagged; invite answer: %s", failed, rec.Body)
	}
	if failed[e.member.Email] {
		t.Errorf("an entered member %s is flagged as having a failed invite", e.member.Email)
	}
}
