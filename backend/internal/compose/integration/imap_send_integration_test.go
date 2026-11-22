// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Sending from an IMAP mailbox over the real composition. The pre-flight lets
// a send through only when the member's IMAP connection holds the send scope,
// and needs no app the deployment configured. A delivery whose mailbox holds
// no send scope when it is dispatched parks with the reason that names
// reconnecting, and transmits nothing.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/capture/imap"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// imapBundle is the sealed credential as the IMAP connect writes it, with or
// without the SMTP settings that turn sending on.
func imapBundle(t *testing.T, sending bool) connector.Auth {
	t.Helper()
	creds := imap.Credentials{Host: "imap.preflight.test", Port: 993, Email: "sender@fable.test", Password: "app", Mailbox: "INBOX", MaxMessages: 50}
	if sending {
		creds.SMTP = &imap.SMTPSettings{Host: "smtp.preflight.test", Port: 465, Security: imap.SMTPSecurityTLS}
	}
	bundle, err := json.Marshal(creds)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

// connectIMAP writes the acting member's IMAP connection with the scopes the
// connector itself reports for bundle.
func (p *preflightEnv) connectIMAP(t *testing.T, bundle connector.Auth) {
	t.Helper()
	scopes, err := imap.NewStanding().GrantedScopes(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (provider, user_id, status, auth, provider_scopes)
			VALUES ('imap', $1, 'connected', $2, $3)
			ON CONFLICT (user_id, provider, lower(account_label)) WHERE user_id IS NOT NULL
			DO UPDATE SET status = 'connected', provider_scopes = EXCLUDED.provider_scopes`,
			p.user, []byte(bundle), scopes)
		return err
	}); err != nil {
		t.Fatalf("seeding the imap connection: %v", err)
	}
}

func TestAnIMAPMailboxSendsOnlyOnceSendingIsTurnedOn(t *testing.T) {
	p := setupPreflightWithoutGoogleApp(t)
	p.connectIMAP(t, imapBundle(t, false))

	status, code, _ := p.send(t)
	if status != http.StatusUnprocessableEntity || code != "mailbox_not_send_capable" {
		t.Fatalf("send through a read-only IMAP mailbox → %d %q, want 422 mailbox_not_send_capable", status, code)
	}

	p.connectIMAP(t, imapBundle(t, true))
	p.stakeADeal(t)
	if status, code, _ := p.send(t); status != http.StatusAccepted {
		t.Fatalf("send through an IMAP mailbox with sending on → %d %q, want 202", status, code)
	}
	var provider string
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT provider FROM comms_outbound`).Scan(&provider)
	}); err != nil {
		t.Fatal(err)
	}
	if provider != "imap" {
		t.Errorf("the delivery was staged for %q, want imap", provider)
	}
}

// imapResolver hands the dispatcher the real IMAP connector and the scopes it
// reports for the credential, as the capture registry does.
type imapResolver struct{ auth connector.Auth }

func (r imapResolver) Resolve(context.Context, ids.UserID, string, *ids.UUID) (connector.EmailSender, connector.Auth, []string, error) {
	scopes, err := imap.NewStanding().GrantedScopes(r.auth)
	return imap.NewStanding(), r.auth, scopes, err
}

func (imapResolver) ResolveChannel(context.Context, ids.UserID, string) (connector.MessageSender, connector.Auth, error) {
	return nil, nil, errors.New("imapResolver: this suite stages mail deliveries only")
}

// The owner reconnected the mailbox without sending after the mail was staged.
func TestAnIMAPDeliveryWhoseMailboxNoLongerSendsParks(t *testing.T) {
	p := setupPreflightWithoutGoogleApp(t)
	p.connectIMAP(t, imapBundle(t, true))
	p.stakeADeal(t)
	activityID := p.sendExpectingAcceptance(t, "transactional", "Re: Inbound question", "answer")
	deliveryID, _ := p.deliveryFor(t, activityID)

	dispatcher := comms.NewDispatcher(
		compose.NewMailDeliveryStore(p.Pool, nil),
		imapResolver{auth: imapBundle(t, false)},
		compose.NewSendSeatAuthority(p.Pool),
		compose.NewSendAttachmentAuthority(p.Pool, nil),
		consent.NewGate(consent.NewStore(compose.InstallationDB(p.Pool))),
		nil, func() time.Time { return clocktest.Now(t) }, 24*time.Hour, 10,
	)
	outcome, _, err := dispatcher.DispatchWithWait(compose.SendWorkerContext(context.Background(), p.workspaceID(t)), deliveryID)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if outcome != comms.OutcomeParked {
		t.Fatalf("dispatch → %q, want parked", outcome)
	}
	if reason := p.deliveryReason(t, deliveryID); !strings.Contains(reason, "not granted the send scope") {
		t.Errorf("parked reason = %q, want the send-scope reason", reason)
	}
}
