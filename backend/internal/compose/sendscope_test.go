// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Gmail send scope is asked for, demanded, and re-checked in three
// different packages, and only compose imports all of them. Drift between them
// fails SILENTLY: every send parks with "this mailbox connection was not
// granted the send scope", which reads as a user who declined consent rather
// than as a typo, and no amount of reconnecting repairs it.

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/modules/capture/imap"
	"github.com/margince/margince/backend/internal/modules/capture/testmailbox"
	"github.com/margince/margince/backend/internal/modules/comms"
)

// EVERY sending mail provider, in one table, so a third one cannot be added to
// two of the three places and pass. The connector's own constant is the
// authority: comms spells the string a second time because it may not import a
// capture provider, and the consent must REQUEST what the gate then demands.
var sendingMailProviders = []struct {
	provider  string
	recheck   string   // the constant the connector re-checks a grant against
	requested []string // the permissions that provider's consent asks for
}{
	{provider: "gmail", recheck: gmail.SendScope, requested: gmailScopes},
	{provider: "graph", recheck: graph.SendScope, requested: graphScopes},
	// test_mailbox and imap have no OAuth consent screen. Their "request" is the
	// one scope the connector reports holding (GrantedScopes). comms.SendScopeFor
	// demands that same scope back.
	{provider: "test_mailbox", recheck: testmailbox.SendScope, requested: []string{testmailbox.SendScope}},
	{provider: "imap", recheck: imap.SendScope, requested: []string{imap.SendScope}},
}

// The scope the authority gate DEMANDS must be the scope the connector
// RE-CHECKS. comms cannot import a capture provider (a module never imports a
// sibling), so SendScopeFor spells the string a second time and this is the
// only place the two can be held against each other.
func TestTheSendScopeCommsDemandsIsTheOneTheConnectorRechecks(t *testing.T) {
	for _, p := range sendingMailProviders {
		t.Run(p.provider, func(t *testing.T) {
			scope, capability := comms.SendScopeFor(p.provider, comms.PreRegistryChannelSending)
			if capability != comms.SendsWithScope {
				t.Fatalf("comms.SendScopeFor(%q, comms.PreRegistryChannelSending) = %v, want SendsWithScope; a grant that is never scope-checked, or a delivery that always parks", p.provider, capability)
			}
			if scope != p.recheck {
				t.Errorf("comms demands %q, the %s connector re-checks %q — every send would park as ungranted", scope, p.provider, p.recheck)
			}
		})
	}
}

// The OAuth consent must REQUEST what the gate demands. No vendor adds a
// permission to an existing grant, so a connection without it must reconnect.
func TestEveryMailConsentRequestsTheScopeTheSendPathDemands(t *testing.T) {
	for _, p := range sendingMailProviders {
		t.Run(p.provider, func(t *testing.T) {
			scope, _ := comms.SendScopeFor(p.provider, comms.PreRegistryChannelSending)
			if !slices.Contains(p.requested, scope) {
				t.Errorf("the %s consent requests %v, which does not include the send scope %q the dispatcher demands", p.provider, p.requested, scope)
			}
		})
	}
}

// The table above must cover EXACTLY the providers comms hands a scope to.
//
// Derived from comms' own map rather than restated, because a second list is a
// second answer and the failure it hides is the one that matters: a provider
// added to mailSendScopes and to nothing else, whose scope no test ever checks
// against what its connector re-checks. The compile-time connector.EmailSender
// assertions elsewhere already prove each named connector can send; what only
// this can prove is that none was forgotten.
func TestTheSendScopeTableCoversExactlyWhatCommsHandsAScopeTo(t *testing.T) {
	covered := make(map[string]bool, len(sendingMailProviders))
	for _, p := range sendingMailProviders {
		covered[p.provider] = true
	}
	for _, provider := range comms.MailSendProviders() {
		if !covered[provider] {
			t.Errorf("comms hands %q a send scope and this file does not bind it to a connector constant — "+
				"its scope is checked against nothing", provider)
		}
		delete(covered, provider)
	}
	for provider := range covered {
		t.Errorf("this file binds %q, which comms hands no send scope: every delivery for it parks as "+
			"\"provider cannot send messages\"", provider)
	}
}

// The channel provider comms answers for must be the one capture connects. The
// same silent-drift argument as the scope above, in the other direction: comms
// spells "telegram" a second time because it may not import capture, and a
// misspelling here reads a live bot as capture-only — every reply parks with
// "provider cannot send messages", which is a connector limitation that does not
// exist.
func TestTheChannelProviderCommsCanSendForIsTheOneCaptureConnects(t *testing.T) {
	scope, capability := comms.SendScopeFor(capture.ProviderTelegram, comms.PreRegistryChannelSending)
	if capability != comms.SendsWithoutScope {
		t.Fatalf("comms.SendScopeFor(%q, comms.PreRegistryChannelSending) = %v, want SendsWithoutScope", capture.ProviderTelegram, capability)
	}
	if scope != "" {
		t.Errorf("a bot token has no OAuth grant to intersect, yet comms demands scope %q of it", scope)
	}
}

// The THIRD spelling of the same provider name, and the same silent drift —
// restated across the axis split (ADR-0107/A158). Capture files a Telegram
// update under kind=message with channel_provider=telegram, and the reply
// operation reads BOTH: the kind to know it is a conversation, the provider to
// know what can carry the answer. Drift in either half refuses every Telegram
// reply with a 422 about the wrong record — nothing parks, nothing logs, and no
// mail test notices.
func TestTheChannelKindTheReplyPathAnswersIsTheOneCaptureFilesUnder(t *testing.T) {
	if !activities.IsChannelKind(activities.KindMessage) {
		t.Errorf("activities does not recognise %q as a channel conversation; every channel reply would be refused as the wrong kind of anchor",
			activities.KindMessage)
	}
	// The transport half: capture's provider name has to be one the send path
	// will act on, which is the drift this test has always been about.
	if !comms.PreRegistryChannelSending(capture.ProviderTelegram) {
		t.Errorf("a composition before its boot cannot send on %q, the provider capture files Telegram messages under; every Telegram reply would be refused as uncarriable",
			capture.ProviderTelegram)
	}
	// The fourth spelling: capture restates the message kind rather than
	// importing it, so the two literals are held against each other here.
	// Drift would file every captured message under a kind the reply path does
	// not answer, and the kind's foreign key would fail the write.
	if capture.ChatMessageKind != activities.KindMessage {
		t.Errorf("capture files channel messages under kind %q and the reply path answers %q; every captured message would be unrepliable",
			capture.ChatMessageKind, activities.KindMessage)
	}
	// And it is not a blanket yes: mail has its own send path, and admitting it
	// here would route a mail reply through a transport with no address to send
	// to.
	if activities.IsChannelKind("email") {
		t.Error("activities treats mail as a messaging channel; a mail anchor would resolve a channel recipient it has none of")
	}
}
