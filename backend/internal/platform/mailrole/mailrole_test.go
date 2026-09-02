// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailrole_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/mailrole"
)

// The addresses a founder found in his own CRM, each having become a "contact"
// with a department for a name. They are the specification: whatever else this
// package does, it refuses these.
func TestEveryObservedRoleAddressIsRefused(t *testing.T) {
	t.Parallel()
	for _, address := range []string{
		"support+idy4dl62-9rnjp@getmyinvoices.zendesk.com",
		"billing_apac@habyt.com",
		"hello.events@thesentry.com.vn",
		"asia-accounting@nfq.com",
	} {
		if _, role := mailrole.Match(address); !role {
			t.Errorf("%s: wanted a role mailbox, got a person", address)
		}
	}
}

// The limit of a deterministic list, stated as a test so nobody assumes a
// coverage this package does not have.
//
// `singapur@advantageaustria.org` is a trade office's shared mailbox and
// `hello@thesentry.com.vn` signs itself with a company name — both became
// "contacts" in the incident this package answers. Neither carries a role WORD:
// one is a city, the other a business. Recognising them needs to know what the
// organization is, which is the AI verdict's question, and a list that guessed
// at city and company names would refuse people called Paris and Mercer.
//
// So this package answers no here, deliberately, and the verdict lane owns the
// case. If that lane ever stops covering it, this test says where to look.
func TestACityOrCompanyMailboxIsBeyondADeterministicList(t *testing.T) {
	t.Parallel()
	if _, role := mailrole.Match("singapur@advantageaustria.org"); role {
		t.Error("the list has started guessing at place names")
	}
}

// The other half of the same rule: a person whose name or address happens to
// contain a role word is still a person. A gate that cannot tell these apart
// trades one silent failure for a louder one.
func TestAPersonIsNotARoleMailbox(t *testing.T) {
	t.Parallel()
	for _, address := range []string{
		"anna.weber@acme.com",
		"lars@gradion.com",
		"bill.mccarthy@acme.com",   // "bill" is a name, not "billing"
		"supporter@charity.org",    // a word CONTAINING a role word
		"marketingsolutions@x.com", // one long word, not a role field
		"jan.newsome@acme.com",     // "newsome" is not "news"
		"connor.eply@acme.com",     // not "noreply"
	} {
		if token, role := mailrole.Match(address); role {
			t.Errorf("%s: wanted a person, got role mailbox %q", address, token)
		}
	}
}

// Plus-addressing is a routing tag the mailbox owner appended. Reading it as
// part of the identity makes one support queue look like thousands of senders,
// which is how a ticketing address reached the model as a first-time stranger.
func TestPlusAddressingIsStrippedBeforeTheRoleIsRead(t *testing.T) {
	t.Parallel()
	token, role := mailrole.Match("support+ticket-9931@acme.com")
	if !role || token != "support" {
		t.Fatalf("wanted the support role, got %q role=%v", token, role)
	}
	if _, role := mailrole.Match("anna.weber+crm@acme.com"); role {
		t.Error("a person's address with a routing tag is still a person")
	}
}

// A helpdesk vendor answers on its customer's behalf, and its routing address
// carries no role word at all.
func TestAHelpdeskVendorIsARoleMailboxWhateverTheLocalPart(t *testing.T) {
	t.Parallel()
	token, role := mailrole.Match("idy4dl62-9rnjp@acme.zendesk.com")
	if !role || token != "helpdesk:zendesk.com" {
		t.Fatalf("wanted the zendesk vendor, got %q role=%v", token, role)
	}
	// A lookalike registration must not borrow the rule.
	if _, role := mailrole.Match("anna@notzendesk.com"); role {
		t.Error("a lookalike domain must not match the vendor rule")
	}
}

// A display name made only of department words invents a person when stored as
// a full name. One that names somebody does not.
func TestADepartmentDisplayNameNamesNobody(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Billing", "APAC Billing", "Support Team", "Sales Department"} {
		if !mailrole.DisplayName(name) {
			t.Errorf("%q: wanted a department, got a person's name", name)
		}
	}
	// "Events The Sentry" is NOT here: "Sentry" is a company name, and a display
	// name carrying one is beyond a word list for the same reason the address is.
	for _, name := range []string{"Anna Weber", "Anna from Billing", "Lars Jankowfsky", "APAC"} {
		if mailrole.DisplayName(name) {
			t.Errorf("%q: wanted a person's name, got a department", name)
		}
	}
}

// The gate that forbids a second spelling of this list derives its corpus from
// Tokens(), so an empty or unsorted answer would silently retire it.
func TestTokensAreTheVocabularyAndAreSorted(t *testing.T) {
	t.Parallel()
	tokens := mailrole.Tokens()
	if len(tokens) < 50 {
		t.Fatalf("the vocabulary looks truncated: %d tokens", len(tokens))
	}
	for i := 1; i < len(tokens); i++ {
		if tokens[i] <= tokens[i-1] {
			t.Fatalf("Tokens() is not sorted at %d: %q then %q", i, tokens[i-1], tokens[i])
		}
	}
}

// An address is not a role mailbox because it is malformed. Answering yes here
// would refuse a record on a parse failure.
func TestAnUnreadableAddressIsNotARoleMailbox(t *testing.T) {
	t.Parallel()
	for _, address := range []string{"", "   ", "@acme.com", "support@", "support", "+tag@acme.com"} {
		if _, role := mailrole.Match(address); role {
			t.Errorf("%q: an unreadable address must not read as a role mailbox", address)
		}
	}
}

// The AI verdict's prompt names example role words, and it is a second
// statement of the same rule this package holds. The two were written
// independently and disagreed about `service@`: the prompt called it a role
// mailbox, the list did not, so an address the model would have refused was
// created as a contact by the tier ladder before the model ever saw it.
//
// Every example the prompt gives must be a word this list actually matches.
func TestThePromptsRoleExamplesAreAllRoleTokens(t *testing.T) {
	t.Parallel()
	for _, word := range mailrole.PromptExamples() {
		if !mailrole.IsRoleLocalPart(word) {
			t.Errorf("the prompt offers %q as a role mailbox and this list does not match it", word)
		}
		if _, role := mailrole.Match(word + "@acme.example"); !role {
			t.Errorf("%q@ is a prompt example that Match refuses", word)
		}
	}
}
