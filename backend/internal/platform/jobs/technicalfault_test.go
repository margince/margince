// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

// The classifier reads the cause and publishes none of it.
//
// Every case here starts from an error in the shape a real provider failure
// arrives in, and makes two assertions: the authored sentence comes out, and NO
// PART OF THE INPUT DOES. The second is the one that holds the invariant
// river_job.errors depends on — that column has no workspace and no row-level
// protection, and a provider's prose routinely names the address or the record
// it refused.

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// secretish are the fragments a real cause carries that must never reach the
// stored sentence: the host somebody is syncing, the mailbox, the record id.
var secretish = []string{
	"mail.acme-customer.example",
	"anna.weber@acme-customer.example",
	"018f3a1b-0000-7000-8000-0000000000c1",
}

func TestATechnicalCauseIsNamedWithoutBeingQuoted(t *testing.T) {
	for _, c := range []struct {
		name  string
		cause error
		class string
	}{{
		name: "a host that does not resolve",
		cause: &net.DNSError{
			Err: "no such host", Name: secretish[0], IsNotFound: true,
		},
		class: "host_unresolved",
	}, {
		name:  "a certificate the trust store refuses",
		cause: &tls.CertificateVerificationError{Err: errors.New("x509: certificate signed by unknown authority")},
		class: "tls_refused",
	}, {
		name: "a refused dial",
		cause: &net.OpError{
			Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 443},
			Err: syscall.ECONNREFUSED,
		},
		class: "connection_refused",
	}, {
		name:  "a deadline the caller set",
		cause: context.DeadlineExceeded,
		class: "provider_timed_out",
	}, {
		name:  "a read that outlived its deadline",
		cause: os.ErrDeadlineExceeded,
		class: "provider_timed_out",
	}, {
		name: "a credential the provider rejected",
		cause: &connector.ProviderError{
			Op: "/users/" + secretish[1] + "/messages", Status: 401,
			Reason: "invalid_grant", Class: errors.New("auth"),
		},
		class: "provider_rejected_credential",
	}, {
		name: "the provider's own fault",
		cause: &connector.ProviderError{
			Op: "/records/" + secretish[2], Status: 503, Class: errors.New("upstream"),
		},
		class: "provider_server_error",
	}} {
		t.Run(c.name, func(t *testing.T) {
			got := Fault(c.cause)
			want := SentenceForClass(c.class)
			if want == "" {
				t.Fatalf("no core sentence for class %q, so this case asserts nothing", c.class)
			}
			if got.Error() != want {
				t.Errorf("stored sentence = %q, want the authored %q", got.Error(), want)
			}
			// The whole point: the cause is reachable through errors.Is and
			// unreachable through the column.
			if !errors.Is(got, c.cause) {
				t.Error("the cause is no longer reachable, so nothing downstream can classify on it")
			}
			for _, fragment := range secretish {
				if strings.Contains(got.Error(), fragment) {
					t.Errorf("the stored sentence carries %q out of the cause — river_job.errors has no "+
						"workspace column and no row-level protection, so this is fleet-visible", fragment)
				}
			}
			// And the stronger form of the same property, which does not depend
			// on this case having planted the right fragments: what is stored
			// is EXACTLY a member of the authored set. A sentence assembled
			// from the cause could not be, however careful the assembly —
			// which is why the classifier chooses rather than formats.
			if !isAuthoredSentence(got.Error()) {
				t.Errorf("the stored sentence %q is not one of the authored set, so something was built from the cause", got.Error())
			}
		})
	}
}

// A cause nobody enumerated gets NOTHING, which is the default this classifier
// is safe because of: a sentence chosen by resemblance sends an operator after
// the wrong thing with the same confidence as a true one.
func TestACauseNobodyEnumeratedIsNotGuessedAt(t *testing.T) {
	got := Fault(errors.New("the provider said something nobody has classified"))
	if detail, ok := VettedFailure("", got.Error()); ok {
		t.Errorf("an unrecognised cause was classified as %q — the classifier guessed", detail.Class)
	}
	if !strings.Contains(got.Error(), "could not classify") {
		t.Errorf("stored sentence = %q, want the unclassified substitute", got.Error())
	}
}

// A SENTINEL still wins. "The record this job names no longer exists" tells an
// operator what to do; "the provider did not answer in time" tells them where it
// happened, and the first is the more useful of two true statements.
func TestASentinelOutranksTheCausesShape(t *testing.T) {
	// A timeout the caller already classified as a budget refusal.
	wrapped := errors.Join(apperrors.ErrBudgetExceeded, context.DeadlineExceeded)
	got := Fault(wrapped)
	if got.Error() == SentenceForClass("provider_timed_out") {
		t.Error("the cause's shape outranked the sentinel the caller set, losing the more useful statement")
	}
}

// Every class this file publishes is readable back, or the screen shows a
// sentence with no class beside it and the metric counts it as unclassified.
func TestEveryTechnicalSentenceReadsBackToItsClass(t *testing.T) {
	for _, technical := range technicalFaults {
		detail, ok := VettedFailure("", technical.sentence)
		if !ok {
			t.Errorf("the sentence for %q does not read back, so it publishes with no class", technical.class)
			continue
		}
		if detail.Class != technical.class || detail.Remedy != technical.remedy {
			t.Errorf("%q reads back as %q/%q", technical.class, detail.Class, detail.Remedy)
		}
	}
}

// isAuthoredSentence answers whether text is, exactly, one the product wrote.
func isAuthoredSentence(text string) bool {
	for _, technical := range technicalFaults {
		if text == technical.sentence {
			return true
		}
	}
	for _, known := range vocabulary {
		if text == known.sentence {
			return true
		}
	}
	return false
}
