// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestTransactionalListSuppress(t *testing.T) {
	// extra adds one infra eSLD; never allowlists a domain that WOULD otherwise
	// be suppressed by a corroborated prefix rule.
	list := NewTransactionalList([]string{"customship.io"}, []string{"news.myrealbrand.com", "myrealbrand.com"})

	cases := []struct {
		name         string
		in           TransactionalInput
		wantSuppress bool
		wantReason   string
	}{
		{
			name:         "exact infra eSLD suppresses standalone, no corroboration needed",
			in:           TransactionalInput{Domain: "eu.docusign.net", Localpart: "dse"},
			wantSuppress: true,
			wantReason:   "transactional_infra:docusign.net",
		},
		{
			name:         "sendgrid relay suppressed",
			in:           TransactionalInput{Domain: "bounces.sendgrid.net", Localpart: "bounce"},
			wantSuppress: true,
			wantReason:   "transactional_infra:sendgrid.net",
		},
		{
			name:         "config-extra infra eSLD suppressed",
			in:           TransactionalInput{Domain: "mail.customship.io", Localpart: "orders"},
			wantSuppress: true,
			wantReason:   "transactional_infra:customship.io",
		},
		{
			name:         "prefix domain WITH List-Unsubscribe corroboration is suppressed",
			in:           TransactionalInput{Domain: "event.gitex.com", Localpart: "hello", ListUnsubscribe: true},
			wantSuppress: true,
			wantReason:   "transactional_prefix:event",
		},
		{
			name:         "prefix domain WITH machine-localpart corroboration is suppressed",
			in:           TransactionalInput{Domain: "news.gitex.com", Localpart: "no-reply"},
			wantSuppress: true,
			wantReason:   "transactional_prefix:news",
		},
		{
			name:         "prefix domain WITHOUT corroboration is NOT suppressed (a real company can live at event.gitex.com)",
			in:           TransactionalInput{Domain: "event.gitex.com", Localpart: "sales"},
			wantSuppress: false,
		},
		{
			name:         "ordinary company mail is never suppressed",
			in:           TransactionalInput{Domain: "mail.acme.com", Localpart: "jane", ListUnsubscribe: true},
			wantSuppress: false,
		},
		{
			name:         "bare registrable domain has no prefix, never suppressed",
			in:           TransactionalInput{Domain: "gitex.com", Localpart: "sales", ListUnsubscribe: true},
			wantSuppress: false,
		},
		{
			name:         "allowlist wins over a corroborated prefix rule",
			in:           TransactionalInput{Domain: "news.myrealbrand.com", Localpart: "no-reply", ListUnsubscribe: true},
			wantSuppress: false,
		},
		{
			name:         "empty domain is a no-op",
			in:           TransactionalInput{Domain: "", Localpart: "x"},
			wantSuppress: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotSuppress, gotReason := list.Suppress(tc.in)
			if gotSuppress != tc.wantSuppress {
				t.Fatalf("Suppress(%+v) = %v, want %v", tc.in, gotSuppress, tc.wantSuppress)
			}
			if tc.wantSuppress && gotReason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", gotReason, tc.wantReason)
			}
			if !tc.wantSuppress && gotReason != "" {
				t.Fatalf("reason = %q, want empty on a non-suppression", gotReason)
			}
		})
	}
}

func TestIsMachineLocalpart(t *testing.T) {
	machine := []string{"no-reply", "noreply", "no.reply", "donotreply", "bounce", "bounces", "notifications", "mailer-daemon", "postmaster", "newsletter"}
	for _, l := range machine {
		if !isMachineLocalpart(l) {
			t.Errorf("isMachineLocalpart(%q) = false, want true", l)
		}
	}
	human := []string{"jane", "jane.doe", "sales", "info", "j.smith"}
	for _, l := range human {
		if isMachineLocalpart(l) {
			t.Errorf("isMachineLocalpart(%q) = true, want false", l)
		}
	}
}

// A queue asking "is a customer waiting?" has an address and no headers, so it
// needs the address half of the rule on its own.
func TestAMachineAddressIsRecognisedWithoutHeaders(t *testing.T) {
	for _, address := range []string{
		"noreply@acme.com", "no-reply@acme.com", "notifications@acme.com",
		"donotreply@acme.com", "bounces@acme.com",
		"anything@sendgrid.net", "hello@mailgun.org",
		// The compound names the real world actually sends from. An exact
		// match missed every one of these, and they filled a rep's day.
		"esignature-noreply@google.com", "calendar-notification@google.com",
		"jira-no-reply@atlassian.net", "automated@billing.example.com",
	} {
		if !IsMachineAddress(address) {
			t.Errorf("%q was taken for a contact", address)
		}
	}
}

// And a contact must not be mistaken for one. Over-recognising hides a customer,
// which the reader cannot recover from; under-recognising costs a row.
func TestAContactIsNeverTakenForAMachine(t *testing.T) {
	for _, address := range []string{
		"anna.weber@acme.com", "lars@gradion.com", "sales@acme.com",
		"info@acme.com", "kontakt@acme.de", "", "not-an-address",
		// Separator-created false positives. Stripping dots and dashes before
		// matching turned a contact's name into a marker: `connor.eply` became
		// `connoreply`, which contains "noreply".
		"connor.eply@customer.example",
		"anna.notify.weber@customer.example",
		"automatedcontrols@buyer.example",
		"reply@customer.example",
	} {
		if IsMachineAddress(address) {
			t.Errorf("%q was taken for a machine", address)
		}
	}
}

func TestRecordWorthyRefusesAnAddressNoContactAnswers(t *testing.T) {
	sink := &Sink{transactional: NewTransactionalList(nil, nil)}
	// T1's evidence is that the workspace WROTE to an address, which is honest
	// evidence of intent and silent about whether a contact is there. A mailbox
	// owner books flights, files expenses and answers their own robots, so
	// before this gate every one of those became a contact.
	unreachable := []string{
		"receipts@expensify.com",
		"plans@tripit.com",
		"noreply@acme.com",
		"calendar-notification@google.com",
		"dse@eu.docusign.net",
		// A human-looking local part on an infrastructure domain is refused too:
		// the domain is the statement, not the name in front of it.
		"support@expensify.com",
	}
	for _, address := range unreachable {
		if sink.recordWorthy(counterpartyOf(address)) {
			t.Errorf("recordWorthy(%q) = true, want false — no contact answers that address", address)
		}
	}
	// The refusal has to stay narrow. A contact at an ordinary company is a
	// contact whatever their employer does, and a rule that swept a domain on
	// suspicion would refuse them.
	reachable := []string{
		"jane@github.com",
		"einkauf@kunde.example",
		"info@acme.com",
		"sales@event.gitex.com",
	}
	for _, address := range reachable {
		if !sink.recordWorthy(counterpartyOf(address)) {
			t.Errorf("recordWorthy(%q) = false, want true", address)
		}
	}
	if sink.recordWorthy(connector.Counterparty{}) {
		t.Error("recordWorthy on an empty address = true, want false")
	}
}

func TestAnOperatorsAllowlistOutranksTheRecordWorthyRefusal(t *testing.T) {
	// capture.transactional_never is the escape hatch for a deployment whose
	// business genuinely sells to one of these companies. It has to win here as
	// it wins in Suppress: a refusal that ignored the declaration would turn a
	// deliberate allowlist into a suppression, which is the exact inversion the
	// config's own contract warns about.
	plain := &Sink{transactional: NewTransactionalList(nil, nil)}
	vouched := &Sink{transactional: NewTransactionalList(nil, []string{"xero.com"})}

	const address = "anna.mueller@xero.com"
	if plain.recordWorthy(counterpartyOf(address)) {
		t.Fatalf("recordWorthy(%q) = true without the allowlist, want false — the test proves nothing otherwise", address)
	}
	if !vouched.recordWorthy(counterpartyOf(address)) {
		t.Errorf("recordWorthy(%q) = false despite transactional_never naming the domain", address)
	}
	// The allowlist is about the DOMAIN, not about robots. A no-reply address on
	// a vouched domain still names nobody.
	if vouched.recordWorthy(counterpartyOf("noreply@xero.com")) {
		t.Error("an allowlisted domain admitted a no-reply address — nobody answers it whoever vouched for the domain")
	}
}

// counterpartyOf builds the ladder's view of a sender from its address alone.
func counterpartyOf(address string) connector.Counterparty {
	_, domain, _ := strings.Cut(address, "@")
	return connector.Counterparty{Email: address, Domain: domain}
}

// The same refusal, asked the way the verdict lane asks it — by address, with no
// Sink to hold the registry.
//
// The lane needs its own entry point because a deferred sender reaches a model,
// and a model's stray answer is enough to create the record: a ten-year import
// produced fifteen `transactional` verdicts for an expense tool's receipts
// address and one `contact` at 0.95, and the one created a contact called
// "Receipts". Nothing re-read the fifteen.
func TestTheVerdictLaneRefusesTheSameAddressesTheLadderDoes(t *testing.T) {
	// A billing product sends under its customer's letterhead, so the display
	// name names a company the owner really deals with while the address belongs
	// to the tool. That pair minted a contact called "BERATUNG JUDITH ANDRESEN".
	for _, address := range []string{
		"receipts@expensify.com",
		"noreply@fastbill.com",
		"invoice-receipts@fastbill.com",
	} {
		if AddressCouldNameAContact(address, domainOf(address), nil) {
			t.Errorf("AddressCouldNameAContact(%q) = true — no human answers it", address)
		}
	}
	// A personal-service domain refuses its named staff too, exactly as it does
	// for the tools already on that list — the owner's traffic with a product
	// they use is the product's, whoever signs it. An operator who genuinely
	// sells to one of these declares it (the allowlist case below), which is the
	// escape hatch that keeps the refusal honest rather than absolute.
	if AddressCouldNameAContact("anna.mueller@fastbill.com", "fastbill.com", nil) {
		t.Error("a personal-service domain admitted a named local part — expensify's support@ is refused the same way")
	}
	// And it gives the Sink's answer rather than a second one.
	sink := &Sink{transactional: NewTransactionalList(nil, nil)}
	for _, address := range []string{
		"receipts@expensify.com", "noreply@acme.com", "jane@github.com", "info@acme.com",
	} {
		want := sink.recordWorthy(counterpartyOf(address))
		if got := AddressCouldNameAContact(address, domainOf(address), nil); got != want {
			t.Errorf("AddressCouldNameAContact(%q) = %v, recordWorthy = %v — two answers to one question", address, got, want)
		}
	}
	// The operator's allowlist still outranks the domain lists, and still says
	// nothing about a robot on a vouched domain.
	vouched := NewTransactionalList(nil, []string{"expensify.com"})
	if !AddressCouldNameAContact("anna@expensify.com", "expensify.com", vouched) {
		t.Error("an allowlisted domain refused a named human")
	}
	if AddressCouldNameAContact("receipts@expensify.com", "expensify.com", vouched) {
		t.Error("an allowlisted domain admitted a machine local part — nobody answers it whoever vouched")
	}
}

func domainOf(address string) string {
	_, domain, _ := strings.Cut(address, "@")
	return domain
}

func TestTheAttentionQueueStillSeesHumansAtPersonalServiceCompanies(t *testing.T) {
	// IsMachineAddress is read by the attention queue to drop rows from "who is
	// waiting on me". Its own contract says over-recognising hides a real
	// customer and that the reader cannot recover from it, because nothing on
	// the page says somebody was hidden.
	//
	// So the personal-service product domains are kept OUT of the baseline this
	// reads. They are companies with salescontacts, not relay infrastructure, and
	// the reason to refuse them a CRM record is about the mailbox owner's own
	// traffic — not about whether a human there is waiting for an answer.
	visible := []string{
		"anna.mueller@xero.com",
		"jane@docusign.com",
		"sales@concur.com",
		"support@expensify.com",
	}
	for _, address := range visible {
		if IsMachineAddress(address) {
			t.Errorf("IsMachineAddress(%q) = true — a named human at that company would vanish from the waiting queue", address)
		}
	}
	// Relay infrastructure stays hidden: nobody is waiting behind it.
	hidden := []string{"bounce@sendgrid.net", "dse@eu.docusign.net", "noreply@xero.com"}
	for _, address := range hidden {
		if !IsMachineAddress(address) {
			t.Errorf("IsMachineAddress(%q) = false, want true", address)
		}
	}
}
