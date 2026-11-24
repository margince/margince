// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imapprobe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture/imap"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func probeWith(t *testing.T, in *crmcontracts.ImapCredentials, authenticate Authenticate) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/connectors/imap/connect", nil)
	Probe(r.Context(), w, r, in, authenticate)
	return w
}

func credentials() *crmcontracts.ImapCredentials {
	secret := "app-password"
	return &crmcontracts.ImapCredentials{Host: "imap.example.test", Username: "ada@example.test", Secret: &secret}
}

// The SMTP settings the form sent reach the connector, which checks the login
// before anything is stored.
func TestTheProbeHandsTheSendSettingsToTheConnector(t *testing.T) {
	in := credentials()
	host, port, security := "smtp.example.test", 587, crmcontracts.ImapSmtpSettingsSecurity("starttls")
	in.Smtp = &crmcontracts.ImapSmtpSettings{Host: &host, Port: &port, Security: &security}
	var got imap.Credentials
	probeWith(t, in, func(_ context.Context, req connector.AuthRequest) (connector.Auth, error) {
		if err := json.Unmarshal(req.Payload, &got); err != nil {
			t.Fatal(err)
		}
		return connector.Auth("{}"), nil
	})
	if got.SMTP == nil || *got.SMTP != (imap.SMTPSettings{Host: host, Port: port, Security: "starttls"}) {
		t.Errorf("connector received SMTP %+v, want the form's settings", got.SMTP)
	}

	probeWith(t, credentials(), func(_ context.Context, req connector.AuthRequest) (connector.Auth, error) {
		got = imap.Credentials{}
		if err := json.Unmarshal(req.Payload, &got); err != nil {
			t.Fatal(err)
		}
		return connector.Auth("{}"), nil
	})
	if got.SMTP != nil {
		t.Errorf("a connect without send settings turned sending on: %+v", got.SMTP)
	}
}

func TestAnSMTPRefusalAnswersItsOwnCode(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"rejected login":       {imap.ErrSendLoginRejected, http.StatusUnprocessableEntity, "smtp_login_rejected"},
		"unreachable server":   {imap.ErrSendUnreachable, http.StatusBadGateway, "smtp_unreachable"},
		"login not an address": {imap.ErrSendAddressInvalid, http.StatusUnprocessableEntity, "smtp_address_invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			w := probeWith(t, credentials(), func(context.Context, connector.AuthRequest) (connector.Auth, error) {
				return nil, tc.err
			})
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
				t.Errorf("got %d %s, want %d %s", w.Code, w.Body.String(), tc.status, tc.code)
			}
			if strings.Contains(w.Body.String(), "app-password") {
				t.Error("the refusal echoes the password")
			}
		})
	}
}
