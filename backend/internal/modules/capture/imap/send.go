// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imap

// Sending from an IMAP mailbox: the member's own SMTP server carries the
// message, and a copy goes into the mailbox's Sent folder over IMAP. Sending
// is opt-in per connection. A connection whose credential holds no SMTP
// settings is read-only, and GrantedScopes reports no send scope for it.

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"strings"

	imapv2 "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/platform/mailwire"
	"github.com/margince/margince/backend/internal/platform/netguard"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/pkg/extension"
)

// SendScope is the authority a connection holds once its owner turned sending
// on and the SMTP login passed. There is no OAuth grant behind it: the sealed
// credential is the grant, so GrantedScopes derives the scope from it.
const SendScope = "urn:margince:imap:send"

// The two ways an SMTP submission port is secured.
const (
	SMTPSecurityTLS      = "tls"
	SMTPSecurityStartTLS = "starttls"
)

const (
	defaultSMTPPortTLS      = 465
	defaultSMTPPortStartTLS = 587

	// maxSendableFiles is the contract's attachment_ids cap, as the other mail
	// connectors declare it.
	maxSendableFiles = 10
)

// ErrSendingOff marks a send through a connection whose owner never turned
// sending on. The dispatcher's scope gate parks such a delivery first; this
// is the connector's own refusal behind it.
var ErrSendingOff = fmt.Errorf("imap: sending is not turned on for this mailbox: %w", connector.ErrAuthRejected)

// ErrSendLoginRejected marks an SMTP server that refused the mailbox's login.
var ErrSendLoginRejected = fmt.Errorf("imap: the outgoing mail server rejected these credentials: %w", connector.ErrAuthRejected)

// ErrSendUnreachable marks an SMTP server that could not be reached, secured
// or asked, and took nothing.
var ErrSendUnreachable = fmt.Errorf("imap: could not reach the outgoing mail server: %w", connector.ErrUnreachable)

var (
	_ connector.EmailSender        = (*Connector)(nil)
	_ connector.EmailHandoffSender = (*Connector)(nil)
	_ connector.AttachmentCarrier  = (*Connector)(nil)
	_ connector.GrantedScoper      = (*Connector)(nil)
)

// SMTPSettings is where the mailbox sends from. It logs in with the IMAP
// login and password.
type SMTPSettings struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"`
}

// normalizeSMTP applies the defaults: the IMAP host, implicit TLS, and the
// port that goes with the security.
func normalizeSMTP(smtp SMTPSettings, imapHost string) (SMTPSettings, error) {
	smtp.Host = strings.TrimSpace(smtp.Host)
	if smtp.Host == "" {
		smtp.Host = imapHost
	}
	switch smtp.Security {
	case "", SMTPSecurityTLS:
		smtp.Security = SMTPSecurityTLS
		if smtp.Port == 0 {
			smtp.Port = defaultSMTPPortTLS
		}
	case SMTPSecurityStartTLS:
		if smtp.Port == 0 {
			smtp.Port = defaultSMTPPortStartTLS
		}
	default:
		return SMTPSettings{}, fmt.Errorf("imap: SMTP security %q is neither %q nor %q: %w",
			smtp.Security, SMTPSecurityTLS, SMTPSecurityStartTLS, ErrSendLoginRejected)
	}
	if smtp.Port < 1 || smtp.Port > 65535 {
		return SMTPSettings{}, fmt.Errorf("imap: SMTP port %d is outside 1..65535: %w", smtp.Port, ErrSendLoginRejected)
	}
	return smtp, nil
}

// WithSMTPTransport returns a copy that dials SMTP with dial and trusts roots.
// It exists for tests that run an SMTP server on loopback, which the
// production dialer refuses.
func (c *Connector) WithSMTPTransport(dial func(ctx context.Context, network, addr string) (net.Conn, error), roots *x509.CertPool) *Connector {
	copied := *c
	copied.smtpDial = dial
	copied.smtpRoots = roots
	return &copied
}

// submission is the SMTP exchange for creds. The dialer refuses private and
// loopback addresses, because the host is one the member typed.
func (c *Connector) submission(creds Credentials) mailer.Submission {
	dial := c.smtpDial
	if dial == nil {
		dial = (&net.Dialer{Timeout: dialTimeout, Control: netguard.RefusePrivate}).DialContext
	}
	_, domain, _ := strings.Cut(creds.Email, "@")
	return mailer.Submission{
		Host: creds.SMTP.Host, Port: creds.SMTP.Port,
		Username: creds.Email, Password: creds.Password,
		ImplicitTLS: creds.SMTP.Security == SMTPSecurityTLS,
		HelloName:   domain, Dial: dial, RootCAs: c.smtpRoots,
	}
}

// checkSMTP logs in to the SMTP server and ends the session. The error never
// carries the server's own text, which may echo what was typed.
func (c *Connector) checkSMTP(ctx context.Context, creds Credentials) error {
	err := c.submission(creds).Check(ctx)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, mailer.ErrCredentialsRefused):
		return ErrSendLoginRejected
	default:
		slog.WarnContext(ctx, "imap: the SMTP check did not reach a login", "smtp_host", creds.SMTP.Host, "smtp_port", creds.SMTP.Port, "cause", err)
		return ErrSendUnreachable
	}
}

// GrantedScopes reports SendScope when the sealed credential carries SMTP
// settings. Authenticate seals them only after the SMTP login passed.
func (c *Connector) GrantedScopes(auth connector.Auth) ([]string, error) {
	var creds Credentials
	if err := json.Unmarshal(auth, &creds); err != nil {
		return nil, fmt.Errorf("imap: malformed auth bundle: %w", err)
	}
	if creds.SMTP == nil {
		return []string{}, nil
	}
	return []string{SendScope}, nil
}

// Carriage declares what this connector transmits. The files ride in the
// multipart/mixed message mailwire.Build renders, and the bound is mail's own
// inbound one read in the other direction.
func (c *Connector) Carriage() connector.Carriage {
	return connector.Carriage{
		Carries:         true,
		MaxBytesPerFile: extension.MaxInboundFileBytes,
		MaxFiles:        maxSendableFiles,
	}
}

// SendEmail transmits msg over SMTP and files a copy in the Sent folder.
func (c *Connector) SendEmail(ctx context.Context, auth connector.Auth, msg connector.EmailMessage) (connector.SendReceipt, error) {
	return c.SendEmailWithHandoff(ctx, auth, msg, func(ctx context.Context, send func(context.Context) (connector.SendReceipt, error)) (connector.SendReceipt, error) {
		return send(ctx)
	})
}

// SendEmailWithHandoff keeps the prior-send lookup and the Sent copy outside
// the handoff, which brackets only the SMTP exchange.
//
// The message goes out under msg.MessageID, which SMTP does not rewrite. So
// the receipt reports no other identity. The Sent copy that capture reads back
// lands on the sent row by that key.
func (c *Connector) SendEmailWithHandoff(ctx context.Context, auth connector.Auth, msg connector.EmailMessage, handoff connector.SendHandoff) (connector.SendReceipt, error) {
	if err := msg.Validate(); err != nil {
		return connector.SendReceipt{}, err
	}
	var creds Credentials
	if err := json.Unmarshal(auth, &creds); err != nil {
		return connector.SendReceipt{}, fmt.Errorf("imap: malformed auth bundle: %w", err)
	}
	creds, err := normalizeCredentials(creds)
	if err != nil {
		return connector.SendReceipt{}, err
	}
	if creds.SMTP == nil {
		return connector.SendReceipt{}, ErrSendingOff
	}
	if msg.Attempt > 0 {
		filed, err := c.filedInSent(ctx, creds, msg.MessageID)
		if err != nil {
			return connector.SendReceipt{}, err
		}
		if filed {
			return connector.SendReceipt{}, nil
		}
	}
	// SMTP addresses the blind copies in the envelope, and no server is
	// bound to strip a Bcc header, so the wire copy carries none. The Sent
	// copy keeps it, as a mail client's own does, for the owner to read.
	rcpts := slices.Concat(msg.To, msg.Cc, msg.Bcc)
	wireMsg := msg
	wireMsg.Bcc = nil
	wire := []byte(mailwire.Build(creds.Email, wireMsg))
	if _, err := handoff(ctx, func(ctx context.Context) (connector.SendReceipt, error) {
		return connector.SendReceipt{}, submitError(c.submission(creds).Submit(ctx, creds.Email, rcpts, wire))
	}); err != nil {
		return connector.SendReceipt{}, err
	}
	c.fileSentCopy(ctx, creds, msg.MessageID, []byte(mailwire.Build(creds.Email, msg)))
	return connector.SendReceipt{}, nil
}

// submitError maps an SMTP failure onto the shared vocabulary. A failure
// before the message's final line took nothing, so it may be retried. One at
// or after that line may have been taken. A retry could then mail the
// recipient twice, so it is reported as an unknown outcome.
func submitError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, mailer.ErrCredentialsRefused):
		return ErrSendLoginRejected
	case mailer.NotTaken(err):
		return errors.Join(ErrSendUnreachable, err)
	default:
		return errors.Join(connector.ErrSendOutcomeUnknown, err)
	}
}

// filedInSent reports whether the Sent folder already holds a message with
// this identity. It is the retry lookup the EmailSender contract asks for.
func (c *Connector) filedInSent(ctx context.Context, creds Credentials, messageID string) (bool, error) {
	client, _, err := c.dial(ctx, creds)
	if err != nil {
		return false, err
	}
	//craft:ignore swallowed-errors best-effort close of the lookup session — the search below answered the question
	defer func() { _ = client.Close() }()
	sent, err := sentFolder(client)
	if err != nil || sent == "" {
		return false, err
	}
	return holdsMessage(client, sent, messageID)
}

// fileSentCopy appends the message to the Sent folder, unless the server
// filed it itself. The message is already sent, so a failure here is only
// logged. An error would hand the delivery to a retry, which mails it again.
func (c *Connector) fileSentCopy(ctx context.Context, creds Credentials, messageID string, raw []byte) {
	if err := c.appendSent(ctx, creds, messageID, raw); err != nil {
		slog.WarnContext(ctx, "imap: the message was sent but no copy was filed in the Sent folder", "err", err)
	}
}

func (c *Connector) appendSent(ctx context.Context, creds Credentials, messageID string, raw []byte) error {
	client, _, err := c.dial(ctx, creds)
	if err != nil {
		return err
	}
	//craft:ignore swallowed-errors best-effort close of the filing session — the APPEND's own answer is the result
	defer func() { _ = client.Close() }()
	sent, err := sentFolder(client)
	if err != nil {
		return err
	}
	if sent == "" {
		return errors.New("imap: the mailbox has no Sent folder")
	}
	filed, err := holdsMessage(client, sent, messageID)
	if err != nil || filed {
		return err
	}
	cmd := client.Append(sent, int64(len(raw)), &imapv2.AppendOptions{Flags: []imapv2.Flag{imapv2.FlagSeen}})
	if _, err := cmd.Write(raw); err != nil {
		return fmt.Errorf("imap: writing the Sent copy: %w", err)
	}
	if err := cmd.Close(); err != nil {
		return fmt.Errorf("imap: writing the Sent copy: %w", err)
	}
	if _, err := cmd.Wait(); err != nil {
		return fmt.Errorf("imap: filing the Sent copy: %w", err)
	}
	return nil
}

// sentFolder names the folder sent mail goes in: the one the server marks
// \Sent, else one named "Sent". Empty means the account has neither.
func sentFolder(client *imapclient.Client) (string, error) {
	opts := &imapv2.ListOptions{ReturnSpecialUse: client.Caps().Has(imapv2.CapSpecialUse)}
	entries, err := client.List("", "*", opts).Collect()
	if err != nil {
		return "", fmt.Errorf("imap: listing the account's mailboxes: %w", errors.Join(ErrUnreachable, err))
	}
	return pickSentFolder(entries), nil
}

func pickSentFolder(entries []*imapv2.ListData) string {
	for _, entry := range entries {
		if slices.Contains(entry.Attrs, imapv2.MailboxAttrSent) {
			return entry.Mailbox
		}
	}
	for _, entry := range entries {
		if strings.EqualFold(lastSegment(entry.Mailbox, entry.Delim), "Sent") {
			return entry.Mailbox
		}
	}
	return ""
}

func lastSegment(mailbox string, delim rune) string {
	if delim == 0 {
		return mailbox
	}
	return mailbox[strings.LastIndex(mailbox, string(delim))+1:]
}

// holdsMessage reports whether mailbox holds a message whose Message-ID
// header carries messageID.
func holdsMessage(client *imapclient.Client, mailbox, messageID string) (bool, error) {
	if _, err := client.Select(mailbox, &imapv2.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return false, fmt.Errorf("imap: opening %q: %w", mailbox, errors.Join(ErrUnreachable, err))
	}
	found, err := client.UIDSearch(&imapv2.SearchCriteria{
		Header: []imapv2.SearchCriteriaHeaderField{{Key: "Message-ID", Value: "<" + messageID + ">"}},
	}, nil).Wait()
	if err != nil {
		return false, fmt.Errorf("imap: searching %q: %w", mailbox, errors.Join(ErrUnreachable, err))
	}
	return len(found.AllUIDs()) > 0, nil
}
