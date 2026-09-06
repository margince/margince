// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension_test

// The published channel declaration: what a unit may say it transports, and the
// two rules one declaration can settle for itself. Everything else a channel is
// subject to — colliding with another unit's provider, or with a core
// connector's — is a fact about the composed SET, and lives in the boot.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func aSender() extension.MessageSender {
	return func(context.Context, extension.Runtime, extension.OutboundMessage) (extension.Receipt, error) {
		return extension.Receipt{}, nil
	}
}

func aLiveCheck() extension.ConnectionLiveChecker {
	return func(context.Context, extension.Runtime, extension.UserID) (bool, error) { return true, nil }
}

// A transport that can send and a transport that only captures are both
// well-formed, and the second is the one worth pinning: a nil Send is the
// documented capture-only case, not an incomplete declaration.
func TestAWellFormedChannelIsAcceptedSendingOrNot(t *testing.T) {
	for name, ch := range map[string]extension.Channel{
		"a transport that sends": {Provider: "dispact", CredentialModel: extension.CredentialPerMember, Send: aSender(), Live: aLiveCheck()},
		"a capture-only channel": {Provider: "dispact", CredentialModel: extension.CredentialPerMember},
		"digits and underscores": {Provider: "deal_room2", CredentialModel: extension.CredentialPerMember},
		"a company-wide account": {Provider: "official_account", CredentialModel: extension.CredentialWorkspaceBot},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ch.Validate(); err != nil {
				t.Fatalf("a well-formed channel was refused: %v", err)
			}
		})
	}
}

// The grammar is `channel_provider.provider`'s own CHECK, restated so a unit is
// refused at BOOT with an explanation rather than at the first write with a
// constraint name.
//
// The kebab case is the one that matters: DESIGN-SP5 §9 claimed a channel
// provider shares IngressSource.System's grammar, and it does not — ingress is
// kebab, a provider is snake — so `deal-room` is a legal ingress system and an
// illegal provider. A declaration that took the sibling's rule would boot and
// then violate the column.
func TestTheProviderGrammarIsTheRegistryColumnsAndNotTheIngressDeclarations(t *testing.T) {
	for name, provider := range map[string]string{
		"empty":                     "",
		"kebab, which ingress uses": "deal-room",
		"leading digit":             "1chat",
		"leading underscore":        "_chat",
		"upper case":                "Dispact",
		"a dot":                     "dispact.chat",
		"a space":                   "deal room",
		"over the length cap":       strings.Repeat("c", 33),
	} {
		t.Run(name, func(t *testing.T) {
			if err := (extension.Channel{Provider: provider, CredentialModel: extension.CredentialPerMember}).Validate(); err == nil {
				t.Fatalf("the grammar accepted %q, which channel_provider's own CHECK would refuse", provider)
			}
		})
	}
}

// Send and Live travel together, and it is refused at the DECLARATION rather
// than at the send: a transport that can transmit and cannot report its own
// liveness would leave the core choosing between parking a delivery it might
// have sent and retrying one it already did, at the one moment that choice is
// unrecoverable.
func TestATransportThatCanSendMustBeAbleToSayWhetherItStillMay(t *testing.T) {
	err := extension.Channel{Provider: "dispact", CredentialModel: extension.CredentialPerMember, Send: aSender()}.Validate()
	if err == nil {
		t.Fatal("a channel declaring Send without Live was accepted")
	}
	if !strings.Contains(err.Error(), "Live") {
		t.Errorf("the refusal %q does not name the missing half, which is the one thing the author has to add", err)
	}
	// The other way round is legal: a capture-only channel may still answer
	// whether the member's connection exists, and there is nothing unsafe about
	// knowing it.
	if err := (extension.Channel{Provider: "dispact", CredentialModel: extension.CredentialPerMember, Live: aLiveCheck()}).Validate(); err != nil {
		t.Fatalf("a capture-only channel that can report liveness was refused: %v", err)
	}
}

// SuppliesTransport is the declaration's own answer to the question the
// transport directory publishes, so the endpoint and the boot cannot disagree
// about which registered transports a reply can actually leave on.
func TestSuppliesTransportIsTheDeclarationsOwnAnswer(t *testing.T) {
	if (extension.Channel{Provider: "dispact", CredentialModel: extension.CredentialPerMember}).SuppliesTransport() {
		t.Error("a channel with no Send reported that it supplies transport; every reply staged against it would park")
	}
	if !(extension.Channel{Provider: "dispact", CredentialModel: extension.CredentialPerMember, Send: aSender(), Live: aLiveCheck()}).SuppliesTransport() {
		t.Error("a channel with a Send reported that it supplies none")
	}
}

// Whose credential a transport spends has NO default, and a unit that has not
// said so is refused.
//
// The temptation is a default, and both candidates are wrong in the same way.
// Defaulting to per_member puts a company's shared account on the mailbox path
// with whichever admin bound it as the owner of everybody's customer
// correspondence; defaulting to workspace_bot publishes one member's personal
// chats to every colleague. Neither shows up as an error — both produce a row
// that reads perfectly well to whoever it wrongly belongs to — so the only
// place the mistake can be caught is here, before it is made.
func TestAChannelMustSayWhoseCredentialItSpends(t *testing.T) {
	for name, ch := range map[string]extension.Channel{
		"declares nothing":     {Provider: "dispact"},
		"declares a non-model": {Provider: "dispact", CredentialModel: "oauth"},
		"declares empty":       {Provider: "dispact", CredentialModel: ""},
	} {
		t.Run(name, func(t *testing.T) {
			err := ch.Validate()
			if err == nil {
				t.Fatal("a channel that did not say whose credential it spends was accepted — " +
					"the registry would then answer for it, which is the derivation this declaration replaced")
			}
			if !errors.Is(err, extension.ErrInvalid) {
				t.Fatalf("refusal = %v, want ErrInvalid", err)
			}
			// The message has to name both models: a unit author reading it is
			// deciding, not correcting a typo, and a refusal that says only
			// "invalid" sends them to the source of a package they do not own.
			for _, want := range []string{"workspace_bot", "per_member"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal %q does not name %q — it has to say what the two choices mean", err, want)
				}
			}
		})
	}
}
