// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	close(relay.release)
	<-sent
}
