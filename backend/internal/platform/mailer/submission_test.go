// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailer

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"slices"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/testsmtp"
)

const submissionHost = "smtp.example.test"

// submissionTo builds a Submission that names submissionHost and reaches relay
// on loopback, so certificate checks run against the name a member typed.
func submissionTo(t *testing.T, relay *testsmtp.Relay, pool *x509.CertPool, implicit bool) Submission {
	t.Helper()
	host, port := relay.Address(t)
	return Submission{
		Host: submissionHost, Port: port, Username: "ada@example.test", Password: "app-password",
		ImplicitTLS: implicit, HelloName: "example.test", RootCAs: pool,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, net.JoinHostPort(host, strconv.Itoa(port)))
		},
	}
}

func TestASubmissionLogsInOverTLSAndAddressesEveryRecipient(t *testing.T) {
	for _, implicit := range []bool{true, false} {
		t.Run("implicit="+strconv.FormatBool(implicit), func(t *testing.T) {
			serverTLS, pool := testsmtp.SelfSigned(t, submissionHost)
			relay := testsmtp.StartWith(t, testsmtp.Options{
				TLS: serverTLS, ImplicitTLS: implicit, Username: "ada@example.test", Password: "app-password",
			})
			s := submissionTo(t, relay, pool, implicit)

			rcpts := []string{"to@example.test", "cc@example.test", "bcc@example.test"}
			if err := s.Submit(context.Background(), "ada@example.test", rcpts, []byte("Subject: hi\r\n\r\nbody\r\n")); err != nil {
				t.Fatalf("Submit: %v", err)
			}
			if !relay.Secured() {
				t.Error("the exchange ran in cleartext")
			}
			if got := relay.Recipients(); !slices.Equal(got, rcpts) {
				t.Errorf("recipients = %v, want %v", got, rcpts)
			}
		})
	}
}

// A certificate no trusted authority signed is refused before the password
// goes out.
func TestASubmissionRefusesAnUntrustedCertificate(t *testing.T) {
	serverTLS, _ := testsmtp.SelfSigned(t, submissionHost)
	relay := testsmtp.StartWith(t, testsmtp.Options{TLS: serverTLS, ImplicitTLS: true, Username: "ada@example.test", Password: "app-password"})
	s := submissionTo(t, relay, x509.NewCertPool(), true)

	err := s.Check(context.Background())
	if _, ok := errors.AsType[x509.UnknownAuthorityError](err); !ok {
		t.Fatalf("Check = %v, want the certificate refused", err)
	}
}

func TestASubmissionReportsARefusedLoginAsSuch(t *testing.T) {
	serverTLS, pool := testsmtp.SelfSigned(t, submissionHost)
	relay := testsmtp.StartWith(t, testsmtp.Options{TLS: serverTLS, Username: "ada@example.test", Password: "another"})
	s := submissionTo(t, relay, pool, false)

	err := s.Check(context.Background())
	if !errors.Is(err, ErrCredentialsRefused) {
		t.Fatalf("Check = %v, want ErrCredentialsRefused", err)
	}
}

// A server reached by name that offers no STARTTLS is refused: the login
// would cross the network in cleartext.
func TestASubmissionRefusesCleartextOffLoopback(t *testing.T) {
	relay := testsmtp.StartWith(t, testsmtp.Options{})
	s := submissionTo(t, relay, nil, false)

	if err := s.Check(context.Background()); err == nil {
		t.Fatal("a server without STARTTLS took a login in cleartext")
	}
}
