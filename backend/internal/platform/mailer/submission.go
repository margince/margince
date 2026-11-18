// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailer

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/smtp"
	"strconv"
	"time"

	"github.com/margince/margince/backend/internal/platform/outbound"
)

// ErrCredentialsRefused marks a server that answered the login with a refusal,
// as opposed to one that could not be reached or secured.
var ErrCredentialsRefused = errors.New("mailer: the server refused the credentials")

// Submission is one SMTP exchange with a submission server: dial, secure,
// log in, hand over one message. The operator relay and a member's own
// mailbox both send through it, so the two cannot differ on TLS or deadlines.
type Submission struct {
	Host     string
	Port     int
	Username string
	Password string
	// ImplicitTLS opens TLS before the greeting (port 465). Without it the
	// exchange upgrades with STARTTLS, and a server that offers none is
	// refused unless it is loopback.
	ImplicitTLS bool
	// HelloName is the name the exchange announces in EHLO.
	HelloName string
	// Dial opens the connection; nil is a plain dialer. A caller sending to a
	// host a tenant typed passes one that refuses private addresses.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
	// RootCAs verifies the server's certificate; nil is the system pool.
	RootCAs *x509.CertPool
}

// Submit hands msg to the server for every recipient in rcpts.
//
// Once the server accepts the DATA terminator the message is sent, so a QUIT
// that fails after it is logged and the send still succeeds. Reporting it
// would make the caller retry and mail a second copy. Every failure before
// the final line is a NotTakenError.
func (s Submission) Submit(ctx context.Context, from string, rcpts []string, msg []byte) error {
	client, w, err := s.openMessage(ctx, from, rcpts)
	if err != nil {
		return &NotTakenError{Err: err}
	}
	defer closeQuietly(client)
	if _, err := w.Write(msg); err != nil {
		return &NotTakenError{Err: fmt.Errorf("mailer: relay %s dropped mid-message: %w", s.addr(), err)}
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mailer: relay %s did not accept the message: %w", s.addr(), err)
	}
	if err := client.Quit(); err != nil {
		slog.WarnContext(ctx, "the relay accepted the message and then failed the QUIT; the message is sent",
			"relay", s.addr(), "cause", err)
	}
	return nil
}

// Check runs the exchange up to a successful login and ends it, the test a
// connect form runs before it stores the settings.
func (s Submission) Check(ctx context.Context) error {
	client, err := s.connect(ctx)
	if err != nil {
		return err
	}
	defer closeQuietly(client)
	if err := client.Quit(); err != nil {
		slog.WarnContext(ctx, "the relay took the login and then failed the QUIT", "relay", s.addr(), "cause", err)
	}
	return nil
}

func (s Submission) addr() string { return net.JoinHostPort(s.Host, strconv.Itoa(s.Port)) }

// openMessage runs the exchange up to the message body. On an error the
// server holds nothing of this message.
func (s Submission) openMessage(ctx context.Context, from string, rcpts []string) (*smtp.Client, io.WriteCloser, error) {
	client, err := s.connect(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := client.Mail(from); err != nil {
		closeQuietly(client)
		return nil, nil, fmt.Errorf("mailer: relay %s refused the sender: %w", s.addr(), err)
	}
	for _, rcpt := range rcpts {
		if err := client.Rcpt(rcpt); err != nil {
			closeQuietly(client)
			return nil, nil, fmt.Errorf("mailer: relay %s refused the recipient: %w", s.addr(), err)
		}
	}
	w, err := client.Data()
	if err != nil {
		closeQuietly(client)
		return nil, nil, fmt.Errorf("mailer: relay %s refused the message: %w", s.addr(), err)
	}
	return client, w, nil
}

// connect dials the server, bounds the whole exchange with one deadline,
// secures the channel, and logs in. The deadline keeps a stalling server from
// pinning a goroutine and a socket per send.
func (s Submission) connect(ctx context.Context) (*smtp.Client, error) {
	conn, err := s.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("mailer: relay %s unreachable: %w", s.addr(), err)
	}
	if err := conn.SetDeadline(exchangeDeadline(ctx)); err != nil {
		closeQuietly(conn)
		return nil, fmt.Errorf("mailer: relay %s deadline setup failed: %w", s.addr(), err)
	}
	tlsConfig := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12, RootCAs: s.RootCAs}
	if s.ImplicitTLS {
		secured := tls.Client(conn, tlsConfig)
		if err := secured.HandshakeContext(ctx); err != nil {
			closeQuietly(conn)
			return nil, fmt.Errorf("mailer: relay %s TLS handshake failed: %w", s.addr(), err)
		}
		conn = secured
	}
	client, err := smtp.NewClient(conn, s.Host) // NOSONAR(go:S5332) -- secured right below: implicit TLS above, else STARTTLS; a server with neither is refused unless it is loopback
	if err != nil {
		closeQuietly(conn)
		return nil, fmt.Errorf("mailer: relay %s greeting failed: %w", s.addr(), err)
	}
	if err := s.secure(client, tlsConfig); err != nil {
		closeQuietly(client)
		return nil, err
	}
	if s.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			closeQuietly(client)
			return nil, fmt.Errorf("mailer: relay %s: %w: %w", s.addr(), ErrCredentialsRefused, err)
		}
	}
	return client, nil
}

// secure announces the sender and upgrades a cleartext channel.
//
// The name goes out before STARTTLS and AUTH, the only place SMTP takes it.
// Without it net/smtp says `localhost` for every installation, a false claim.
//
// TLS is required, because the exchange carries a password. A server without
// STARTTLS is refused, except on loopback, where the wire stays on the machine.
func (s Submission) secure(client *smtp.Client, tlsConfig *tls.Config) error {
	name := s.HelloName
	if name == "" {
		name = outbound.MailProduct
	}
	if err := client.Hello(name); err != nil {
		return fmt.Errorf("mailer: relay %s refused the greeting: %w", s.addr(), err)
	}
	if s.ImplicitTLS {
		return nil
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("mailer: relay %s STARTTLS failed: %w", s.addr(), err)
		}
		return nil
	}
	if !isLoopback(s.Host) {
		return fmt.Errorf("mailer: relay %s offers no STARTTLS — refusing to send a credential-bearing mail in cleartext", s.addr())
	}
	return nil
}

func (s Submission) dial(ctx context.Context) (net.Conn, error) {
	if s.Dial != nil {
		return s.Dial(ctx, "tcp", s.addr())
	}
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	return dialer.DialContext(ctx, "tcp", s.addr())
}

// exchangeDeadline is when the relay exchange must end: 30 seconds from now,
// or the caller's own deadline when that comes first. A caller that stops
// waiting for the relay also stops the exchange. So the relay cannot take the
// message after the caller reported it as not taken.
func exchangeDeadline(ctx context.Context) time.Time {
	deadline := time.Now().Add(30 * time.Second)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		return callerDeadline
	}
	return deadline
}

// isLoopback reports whether the relay lives on this machine, the one place
// a missing STARTTLS is acceptable.
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// closeQuietly releases a transport on an error path where the send's
// own error is the one that matters.
func closeQuietly(c io.Closer) {
	//craft:ignore swallowed-errors error-path transport cleanup — the send's own error is already on its way to the caller
	_ = c.Close()
}
