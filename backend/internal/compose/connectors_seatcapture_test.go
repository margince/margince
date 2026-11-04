// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// readOnlyCtx is a signed-in member whose role reads but records nothing, as
// the shipped Read-only role does.
func readOnlyCtx() context.Context {
	ctx := humanCtx()
	actor, _ := principal.Actor(ctx)
	actor.Permissions = principal.Permissions{Objects: map[string]principal.ObjectGrant{
		"activity": {Read: true}, "contact": {Read: true}, "lead": {Read: true},
	}}
	return principal.WithActor(ctx, actor)
}

// Every connect branch refuses a role that cannot capture with its own code.
// It does so before anything is probed or stored: the nil-pool registry here
// would fail any persistence with a 500.
func TestConnectRefusesARoleThatCannotCaptureOnEveryBranch(t *testing.T) {
	for _, provider := range []crmcontracts.CaptureProvider{"gmail", "gcal", "imap", "test_mailbox"} {
		t.Run(string(provider), func(t *testing.T) {
			h := wiredHandlers()
			rec := httptest.NewRecorder()
			body := bytes.NewBufferString(`{"imap":{"host":"mail.example","username":"a@b.example","secret":"s"}}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/connectors/"+string(provider)+"/connect", body).
				WithContext(readOnlyCtx())

			h.ConnectConnector(rec, req, provider)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
			}
			var problem struct{ Code string }
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Code != capture.CodeRoleCannotCapture {
				t.Fatalf("code = %q (%v), want %q", problem.Code, err, capture.CodeRoleCannotCapture)
			}
		})
	}
}

// The connect screen hears the same answer before the click: one seat-level
// verdict, and every provider on the roster reporting the role as the reason.
func TestTheRosterReportsARoleThatCannotCapture(t *testing.T) {
	h := wiredHandlers()
	ctx := readOnlyCtx()
	seat := seatCapture(ctx)
	if seat.Allowed || seat.Reason == nil || *seat.Reason != crmcontracts.SeatCaptureReasonRoleCannotCapture ||
		seat.Missing == nil || *seat.Missing != crmcontracts.SeatCaptureMissingActivity {
		t.Fatalf("seat_capture = %+v, want refused for role_cannot_capture, missing activity", seat)
	}
	for _, p := range *h.providerAvailability(ctx) {
		if p.Reason != crmcontracts.CaptureProviderAvailabilityReasonRoleCannotCapture {
			t.Errorf("%s reason = %s, want role_cannot_capture", p.Provider, p.Reason)
		}
	}
	if seat := seatCapture(humanCtx()); !seat.Allowed {
		t.Fatalf("a capturing member's seat_capture = %+v, want allowed", seat)
	}
}

// readOnlyAuthority is identity answering for a member whose role was moved to
// one that cannot capture while their consent was out at the provider.
type readOnlyAuthority struct{ liveAuthority }

func (readOnlyAuthority) EffectiveRBAC(context.Context, ids.UUID, ids.UUID) (authz.RBAC, error) {
	return authz.RBAC{}, nil
}

func TestCallbackRefusesARoleThatCannotCaptureBeforeExchangingTheCode(t *testing.T) {
	oauth := &recordingOAuth{}
	h := wiredHandlers()
	h.authority, h.oauth, h.gmailAPI = readOnlyAuthority{}, oauth, stubGmailAPI{}
	const nonce = "csrf-nonce-value"
	state := h.signer.sign(connectState{
		Workspace: ids.MustParse("11111111-1111-1111-1111-111111111111"),
		User:      ids.MustParse("22222222-2222-2222-2222-222222222222"),
		Provider:  providerGmail, Nonce: nonce, Version: stateVersionNamespacedCSRF,
	}, time.Now().Add(connectStateTTL))
	code := "the-code"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/connectors/gmail/callback", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName(providerGmail), Value: nonce})

	h.ConnectorOAuthCallback(rec, req, "gmail", crmcontracts.ConnectorOAuthCallbackParams{State: state, Code: &code})

	if oauth.exchanged {
		t.Error("the code was spent for a member whose role cannot capture")
	}
	if got, want := rec.Header().Get("Location"), "https://app.test/#/onboarding/connect/role_cannot_capture/gmail"; rec.Code != http.StatusFound || got != want {
		t.Fatalf("landed %d on %q, want 302 to %q", rec.Code, got, want)
	}
}

// A history import is refused before its estimate asks the provider: the
// nil-pool registry here would fail any read of the connection row.
func TestABackfillPreviewAndStartRefuseARoleThatCannotCapture(t *testing.T) {
	h := backfillHandlers{registry: capture.NewRegistry(nil, nil, readOnlyAuthority{}, nil)}
	for op, invoke := range map[string]func(http.ResponseWriter, *http.Request){
		"preview": func(w http.ResponseWriter, r *http.Request) { h.PreviewConnectorBackfill(w, r, "gmail") },
		"start":   func(w http.ResponseWriter, r *http.Request) { h.StartConnectorBackfill(w, r, "gmail") },
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/connectors/gmail/backfill",
			bytes.NewBufferString(`{"window":"3m"}`)).WithContext(humanCtx())
		req.Header.Set("Content-Type", "application/json")

		invoke(rec, req)

		if rec.Code != http.StatusForbidden || refusalCode(t, rec) != capture.CodeRoleCannotCapture {
			t.Errorf("%s = %d %s, want 403 role_cannot_capture", op, rec.Code, rec.Body.String())
		}
	}
}
