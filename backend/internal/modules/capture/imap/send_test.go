// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imap

// Sending from an IMAP mailbox, against an in-memory IMAP server and an
// in-process SMTP server with TLS and a login. Only the transports are
// replaced. The IMAP dial is a plain loopback pipe. The SMTP dial reaches the
// loopback server under a public name, so the certificate check runs.

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/mail"
	"slices"
	"strconv"
	"strings"
	"testing"

	imapv2 "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/testsmtp"
)

const smtpName = "smtp.ws.example"

// sendRig is one mailbox that can send: its IMAP store, its SMTP server, and
// a connector wired to both.
type sendRig struct {
	user  *imapmemserver.User
	relay *testsmtp.Relay
	conn  *Connector
	creds Credentials
}

// newSendRig serves a mailbox with a Sent folder and an SMTP server that
// demands the mailbox's own login over implicit TLS.
func newSendRig(t *testing.T) sendRig {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser(memUser, memPass)
	for _, box := range []struct {
		name string
		opts *imapv2.CreateOptions
	}{
		{"INBOX", nil},
		// The in-memory server stores no special-use, so this rig finds the
		// folder by name; pickSentFolder's own test covers \Sent.
		{"Sent", nil},
	} {
		if err := user.Create(box.name, box.opts); err != nil {
			t.Fatal(err)
		}
	}
	mem.AddUser(user)
	addr := listenMem(t, mem)

	serverTLS, roots := testsmtp.SelfSigned(t, smtpName)
	relay := testsmtp.StartWith(t, testsmtp.Options{TLS: serverTLS, ImplicitTLS: true, Username: memUser, Password: memPass})
	_, port := relay.Address(t)
	creds, err := normalizeCredentials(Credentials{
		Host: "imap.ws.example", Email: memUser, Password: memPass,
		SMTP: &SMTPSettings{Host: smtpName, Port: port},
	})
	if err != nil {
		t.Fatal(err)
	}
	conn := NewStanding().withDialer(plainDialer(addr)).WithSMTPTransport(loopbackAs(t, relay), roots)
	return sendRig{user: user, relay: relay, conn: conn, creds: creds}
}

// loopbackAs dials relay whatever address the exchange names.
func loopbackAs(t *testing.T, relay *testsmtp.Relay) func(context.Context, string, string) (net.Conn, error) {
	t.Helper()
	host, port := relay.Address(t)
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, net.JoinHostPort(host, strconv.Itoa(port)))
	}
}

func outboundMessage() connector.EmailMessage {
	return connector.EmailMessage{
		To: []string{"alice@acme.test"}, Cc: []string{"bob@acme.test"}, Bcc: []string{"carol@acme.test"},
		Subject: "Offer", Body: "Here it is.", MessageID: "offer-1@ws.example",
	}
}

// sentFolderMessageIDs reads the Message-ID of every message in the Sent folder.
func sentFolderMessageIDs(t *testing.T, rig sendRig) []string {
	t.Helper()
	client, _, err := rig.conn.dial(context.Background(), rig.creds)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		//craft:ignore swallowed-errors test session teardown; the assertions read the result already
		_ = client.Close()
	})
	if _, err := client.Select("Sent", nil).Wait(); err != nil {
		t.Fatal(err)
	}
	msgs, err := client.Fetch(imapv2.SeqSetNum(1, 2, 3), &imapv2.FetchOptions{Envelope: true}).Collect()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, msg := range msgs {
		out = append(out, msg.Envelope.MessageID)
	}
	return out
}

func TestSendingGoesOutOverSMTPUnderOurMessageIDAndIsFiledInSent(t *testing.T) {
	rig := newSendRig(t)

	receipt, err := rig.conn.SendEmail(context.Background(), sealCreds(t, rig.creds), outboundMessage())
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if receipt.RFC822MessageID != "" {
		t.Errorf("receipt re-keys the send to %q; SMTP keeps the identity it was handed", receipt.RFC822MessageID)
	}
	if !rig.relay.Secured() {
		t.Error("the message crossed in cleartext")
	}
	want := []string{"alice@acme.test", "bob@acme.test", "carol@acme.test"}
	if got := rig.relay.Recipients(); !slices.Equal(got, want) {
		t.Errorf("SMTP recipients = %v, want %v (Bcc is addressed, not written into a header)", got, want)
	}
	parsed, err := mail.ReadMessage(strings.NewReader(rig.relay.Received()))
	if err != nil {
		t.Fatalf("the relay received no message: %v", err)
	}
	if got := parsed.Header.Get("Message-ID"); got != "<offer-1@ws.example>" {
		t.Errorf("Message-ID = %q, want ours", got)
	}
	if parsed.Header.Get("Bcc") != "" {
		t.Error("the Bcc addressee is written into the transmitted header")
	}
	if got := sentFolderMessageIDs(t, rig); !slices.Equal(got, []string{"offer-1@ws.example"}) {
		t.Errorf("Sent folder holds %v, want the one copy of the sent message", got)
	}
}

// A server that files sent mail itself keeps one copy, not two.
func TestAServerThatFiledTheMessageItselfGetsNoSecondCopy(t *testing.T) {
	rig := newSendRig(t)
	msg := outboundMessage()
	appendTo(t, rig.user, "Sent", "From: "+memUser+"\r\nTo: alice@acme.test\r\nSubject: Offer\r\n"+
		"Message-ID: <"+msg.MessageID+">\r\n\r\nHere it is.")

	if _, err := rig.conn.SendEmail(context.Background(), sealCreds(t, rig.creds), msg); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if got := sentFolderMessageIDs(t, rig); len(got) != 1 {
		t.Errorf("Sent folder holds %v, want the server's one copy", got)
	}
}

// A retry finds the message already in Sent and does not mail it again.
func TestARetryOfAFiledMessageDoesNotTransmitAgain(t *testing.T) {
	rig := newSendRig(t)
	msg := outboundMessage()
	appendTo(t, rig.user, "Sent", "From: "+memUser+"\r\nSubject: Offer\r\nMessage-ID: <"+msg.MessageID+">\r\n\r\nx")
	msg.Attempt = 1

	if _, err := rig.conn.SendEmail(context.Background(), sealCreds(t, rig.creds), msg); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if got := rig.relay.Recipients(); len(got) != 0 {
		t.Errorf("a retry of a filed message reached SMTP for %v", got)
	}
}

func TestAConnectionWithoutSendingRefusesToSend(t *testing.T) {
	rig := newSendRig(t)
	rig.creds.SMTP = nil

	_, err := rig.conn.SendEmail(context.Background(), sealCreds(t, rig.creds), outboundMessage())
	if !errors.Is(err, ErrSendingOff) || !errors.Is(err, connector.ErrAuthRejected) {
		t.Fatalf("SendEmail = %v, want ErrSendingOff parked as a credential refusal", err)
	}
	if got := rig.relay.Recipients(); len(got) != 0 {
		t.Errorf("a read-only connection reached SMTP for %v", got)
	}
}

// The SMTP host is one the member typed, so the production dialer refuses a
// loopback or private address before any byte goes out.
func TestTheSMTPDialRefusesAPrivateAddress(t *testing.T) {
	rig := newSendRig(t)
	host, port := rig.relay.Address(t)
	creds := rig.creds
	creds.SMTP = &SMTPSettings{Host: host, Port: port, Security: SMTPSecurityTLS}
	guarded := NewStanding()

	if err := guarded.checkSMTP(context.Background(), creds); !errors.Is(err, ErrSendUnreachable) {
		t.Fatalf("checkSMTP to %s = %v, want ErrSendUnreachable", host, err)
	}
	if rig.relay.Secured() {
		t.Error("the guarded dialer reached a loopback SMTP server")
	}
}

// A certificate the trust store does not vouch for is refused before the
// password goes out.
func TestTheSMTPCheckRefusesAnUntrustedCertificate(t *testing.T) {
	rig := newSendRig(t)
	untrusting := rig.conn.WithSMTPTransport(loopbackAs(t, rig.relay), x509.NewCertPool())

	if err := untrusting.checkSMTP(context.Background(), rig.creds); !errors.Is(err, ErrSendUnreachable) {
		t.Fatalf("checkSMTP = %v, want ErrSendUnreachable", err)
	}
}

// Connecting with sending on runs the SMTP login, and seals the settings only
// when it passed. The rejection never carries the password.
func TestConnectingWithSendingOnChecksTheSMTPLogin(t *testing.T) {
	rig := newSendRig(t)
	req, err := AuthRequestFrom(Credentials{
		Host: "imap.ws.example", Email: memUser, Password: memPass,
		SMTP: &SMTPSettings{Host: smtpName, Port: rig.creds.SMTP.Port},
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := rig.conn.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if scopes, _ := rig.conn.GrantedScopes(auth); !slices.Equal(scopes, []string{SendScope}) {
		t.Errorf("granted scopes = %v, want the send scope once the SMTP login passed", scopes)
	}

	refusingTLS, refusingRoots := testsmtp.SelfSigned(t, smtpName)
	refusing := testsmtp.StartWith(t, testsmtp.Options{TLS: refusingTLS, ImplicitTLS: true, Username: memUser, Password: "not-" + memPass})
	wrongPassword := rig.conn.WithSMTPTransport(loopbackAs(t, refusing), refusingRoots)
	_, err = wrongPassword.Authenticate(context.Background(), req)
	if !errors.Is(err, ErrSendLoginRejected) {
		t.Fatalf("Authenticate against a refusing SMTP server = %v, want ErrSendLoginRejected", err)
	}
	if strings.Contains(err.Error(), memPass) {
		t.Error("the refusal carries the password")
	}
}

func TestAReadOnlyConnectionHoldsNoSendScope(t *testing.T) {
	auth, err := json.Marshal(Credentials{Host: "imap.ws.example", Email: memUser, Password: memPass})
	if err != nil {
		t.Fatal(err)
	}
	scopes, err := NewStanding().GrantedScopes(auth)
	if err != nil || len(scopes) != 0 {
		t.Fatalf("GrantedScopes = %v, %v; want none for a connection whose owner never turned sending on", scopes, err)
	}
}

func TestSMTPDefaultsFollowTheSecurity(t *testing.T) {
	for _, tc := range []struct {
		in       SMTPSettings
		wantPort int
		wantSec  string
	}{
		{SMTPSettings{}, 465, SMTPSecurityTLS},
		{SMTPSettings{Security: SMTPSecurityStartTLS}, 587, SMTPSecurityStartTLS},
		{SMTPSettings{Host: "smtp.other.example", Port: 2525, Security: SMTPSecurityStartTLS}, 2525, SMTPSecurityStartTLS},
	} {
		got, err := normalizeSMTP(tc.in, "imap.ws.example")
		if err != nil {
			t.Fatalf("normalizeSMTP(%+v): %v", tc.in, err)
		}
		wantHost := tc.in.Host
		if wantHost == "" {
			wantHost = "imap.ws.example"
		}
		if got.Host != wantHost || got.Port != tc.wantPort || got.Security != tc.wantSec {
			t.Errorf("normalizeSMTP(%+v) = %+v, want %s:%d %s", tc.in, got, wantHost, tc.wantPort, tc.wantSec)
		}
	}
	if _, err := normalizeSMTP(SMTPSettings{Security: "ssl3"}, "imap.ws.example"); err == nil {
		t.Error("an unknown security setting was accepted")
	}
}

// The folder the server marks \Sent wins over one merely named so, and a
// name is read after the server's own delimiter.
func TestTheSentCopyGoesToTheFolderTheServerMarksSent(t *testing.T) {
	marked := []*imapv2.ListData{
		{Mailbox: "INBOX.Sent", Delim: '.'},
		{Mailbox: "Gesendete Elemente", Attrs: []imapv2.MailboxAttr{imapv2.MailboxAttrSent}},
	}
	if got := pickSentFolder(marked); got != "Gesendete Elemente" {
		t.Errorf("pickSentFolder = %q, want the \\Sent folder", got)
	}
	named := []*imapv2.ListData{{Mailbox: "INBOX"}, {Mailbox: "INBOX.sent", Delim: '.'}}
	if got := pickSentFolder(named); got != "INBOX.sent" {
		t.Errorf("pickSentFolder = %q, want the folder named Sent", got)
	}
	if got := pickSentFolder([]*imapv2.ListData{{Mailbox: "INBOX"}, {Mailbox: "Sentiments"}}); got != "" {
		t.Errorf("pickSentFolder = %q, want none", got)
	}
}
