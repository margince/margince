// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// The send-capability answer, and the recipient list the suppression gate is
// handed. Both are read by the dispatcher on every attempt, and both fail
// silently in the same direction if they are wrong: a delivery that parks with a
// reason describing a limitation that does not exist, or a default-deny gate
// asked about nobody.

import (
	"maps"
	"os"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A bot token holds no OAuth scope and still sends. Read through the old
// two-valued answer, "no scope" and "cannot send" were the same reply, so
// Telegram would be classified capture-only and every reply would park.
func TestSendScopeForReportsTelegramSendsWithoutAScope(t *testing.T) {
	scope, capability := SendScopeFor("telegram", PreRegistryChannelSending)
	if capability != SendsWithoutScope {
		t.Fatalf("SendScopeFor(\"telegram\") capability = %v, want SendsWithoutScope", capability)
	}
	if scope != "" {
		t.Errorf("SendScopeFor(\"telegram\") scope = %q, want none: a bot token has no grant to intersect", scope)
	}
}

// The other two states, kept beside the one above because the THREE-way split is
// the point: a provider nobody configured must still refuse, and Gmail must
// still be scope-checked.
func TestSendScopeForSeparatesScopedSendersFromProvidersThatCannotSend(t *testing.T) {
	scope, capability := SendScopeFor("gmail", PreRegistryChannelSending)
	if capability != SendsWithScope || scope == "" {
		t.Fatalf("SendScopeFor(\"gmail\") = (%q, %v), want a named scope with SendsWithScope", scope, capability)
	}
	if _, capability := SendScopeFor("carrier-pigeon", PreRegistryChannelSending); capability != CannotSend {
		t.Fatalf("SendScopeFor on an unknown provider = %v, want CannotSend", capability)
	}
	// The zero value must be the refusal, so a capability nobody answered
	// cannot read as permission.
	var unanswered SendCapability
	if unanswered != CannotSend {
		t.Fatal("the zero SendCapability is not CannotSend; an unset capability would authorize a send")
	}
}

// The channel arm answers from the composition's registered transports: a
// provider it sends on answers SendsWithoutScope, and one it does not falls back
// to CannotSend. The mail arm is untouched by either answer.
func TestSendScopeForChannelArmAnswersFromTheRegisteredTransports(t *testing.T) {
	t.Parallel()
	registered := func(provider string) bool { return provider == "telegram" || provider == "fake-unit-provider" }
	if _, capability := SendScopeFor("fake-unit-provider", registered); capability != SendsWithoutScope {
		t.Fatalf("SendScopeFor(\"fake-unit-provider\") capability = %v, want SendsWithoutScope", capability)
	}
	if _, capability := SendScopeFor("fake-unit-provider", PreRegistryChannelSending); capability != CannotSend {
		t.Fatalf("SendScopeFor(\"fake-unit-provider\") capability = %v, want CannotSend when not registered", capability)
	}
	if scope, capability := SendScopeFor("gmail", registered); capability != SendsWithScope || scope == "" {
		t.Fatalf("SendScopeFor(\"gmail\") = (%q, %v), want an unchanged SendsWithScope with a real scope", scope, capability)
	}
}

// The gate's subject list is derived from the delivery's SHAPE, and a channel
// delivery has to arrive as a channel recipient. Flattened to an address list it
// would arrive empty, and a default-deny gate asked about nobody refuses nobody.
func TestConsentRecipientsCarryTheDeliverysOwnShape(t *testing.T) {
	mail := consentRecipients(liveDelivery())
	if len(mail) != 2 || mail[0].Email != "buyer@example.com" || mail[1].Email != "cc@example.com" {
		t.Fatalf("mail recipients = %+v, want the To and Cc addresses as mail recipients", mail)
	}

	account := "7788"
	channel := Delivery{Provider: "telegram", ChannelUserID: &account, ConsentPurpose: "transactional"}
	got := consentRecipients(channel)
	if len(got) != 1 {
		t.Fatalf("channel recipients = %+v, want exactly one", got)
	}
	if got[0].Channel == nil || got[0].Channel.Provider != "telegram" || got[0].Channel.ChannelUserID != account {
		t.Fatalf("channel recipient = %+v, want the delivery's provider and account id", got[0])
	}
	if err := got[0].Validate(); err != nil {
		t.Fatalf("the recipient handed to the gate does not validate: %v", err)
	}
}

// The discriminator answers on the row's own NULL-ness, not on emptiness: a
// privacy scrub empties a channel recipient in place, and a delivery that then
// read as mail would be a channel row asking the mail gate about no addressees.
func TestIsChannelReadsTheRowsShapeNotItsEmptiness(t *testing.T) {
	if liveDelivery().IsChannel() {
		t.Error("a mail delivery reports IsChannel")
	}
	scrubbed := ""
	erased := Delivery{ID: ids.NewV7(), Provider: "telegram", ChannelUserID: &scrubbed}
	if !erased.IsChannel() {
		t.Error("a channel delivery whose recipient was scrubbed no longer reports IsChannel")
	}
	if erased.ChannelRecipient() != "" {
		t.Error("a scrubbed recipient reads back as an account id")
	}
	// And nothing may transmit to it: the gate refuses the recipient outright.
	if err := consentRecipients(erased)[0].Validate(); err == nil {
		t.Error("a scrubbed channel recipient validated; an erased subject could still be messaged")
	}
}

// The connections screen badges a mailbox that can send from its own copy of
// mailSendScopes (frontend/src/lib/connector-status.ts). A provider missing
// there is never offered as a sender, and a wrong scope disagrees with the
// pre-flight. test_mailbox is a QC-only connector the screen never shows.
func TestTheFrontendSendScopeTableMirrorsMailSendScopes(t *testing.T) {
	source, err := os.ReadFile("../../../../frontend/src/lib/connector-status.ts")
	if err != nil {
		t.Fatal(err)
	}
	table := regexp.MustCompile(`(?s)const SEND_SCOPES[^=]*= \{(.*?)\};`).FindSubmatch(source)
	if len(table) != 2 {
		t.Fatal("SEND_SCOPES missing from connector-status.ts")
	}
	got := map[string]string{}
	for _, entry := range regexp.MustCompile(`(?m)^\s*([a-z_]+):\s*"([^"]+)",?\s*$`).FindAllSubmatch(table[1], -1) {
		got[string(entry[1])] = string(entry[2])
	}
	want := maps.Clone(mailSendScopes)
	delete(want, "test_mailbox")
	if !maps.Equal(got, want) {
		t.Errorf("connector-status.ts SEND_SCOPES = %v, want %v", got, want)
	}
}
