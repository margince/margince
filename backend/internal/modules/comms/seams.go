// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// The seams one dispatch attempt runs against, and the facts derived from a
// delivery rather than asked of a collaborator. They live apart from the
// dispatch sequence itself so that the file next door reads as the sequence:
// what each gate asks, and of whom, is settled here.

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// deliveryStore is the persistence the dispatcher needs: one load that counts
// the attempt, and the four transitions that close or defer a delivery. It is
// private because Store is the only implementation the product ships — the
// interface exists so the dispatcher's branch table can be proven without a
// database, not to invite a second store.
type deliveryStore interface {
	Load(ctx context.Context, id ids.UUID) (Delivery, error)
	RecordSent(ctx context.Context, id ids.UUID, receipt connector.SendReceipt) error
	Park(ctx context.Context, id ids.UUID, reason string) error
	// ParkTransmitted parks a delivery the provider ALREADY accepted, keeping
	// the receipt's provider message id on the row. It is a transition of its
	// own rather than an argument to Park because the row it leaves says
	// something Park cannot: the message went, and only its receipt is missing.
	ParkTransmitted(ctx context.Context, id ids.UUID, reason, providerMessageID string) error
	RecordFailure(ctx context.Context, id ids.UUID, reason string) error
	RecordDeferral(ctx context.Context, id ids.UUID, reason string) error
	// The at-most-once marker, for the seams whose retries cannot detect a prior
	// send. Both are ordinary status-guarded transitions; what makes them a
	// guarantee is WHEN the dispatcher calls them (sendseam.go).
	MarkInFlight(ctx context.Context, id ids.UUID) error
	ClearInFlight(ctx context.Context, id ids.UUID) error
	// ClearPayloadRef retires a controller message's one-time link material
	// once the message can no longer be sent. The row stays: it is still the
	// record that a message went out, and what must not survive is a live
	// credential pointing at somebody's mailbox.
	ClearPayloadRef(ctx context.Context, id ids.UUID) error
}

var _ deliveryStore = (*Store)(nil)

// MessageIdentityReconciler re-keys the timeline row for a message whose
// provider stamped an identity different from the one this system minted.
//
// It takes the caller's transaction so the re-key and the timeline row commit
// together — but that transaction is NOT the receipt's. The receipt commits
// whenever the provider accepted the message, and the re-key is bookkeeping
// subordinate to it: one that could roll the receipt back would return the
// delivery to a ladder whose prior-send lookup cannot see a rewritten identity,
// mailing the recipient twice over a bookkeeping fault. So an error here is
// recorded and dropped, and an implementer may fail freely.
//
// previous is the identity the message was staged under, so the implementer
// can tell a conversation ROOT (thread_key == previous) from a reply, which
// must keep its anchor's root.
type MessageIdentityReconciler interface {
	ReconcileMessageIdentityTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, previous, stamped string) error
}

// ConsentGate answers whether these recipients may still be reached for this
// purpose. It is default-deny: a recipient who never granted the purpose, and
// one who withdrew it, are refused alike.
//
// It asks in RECIPIENTS rather than addresses because one ladder carries both
// transports: a channel recipient has no address, so an address-only gate would
// be handed an empty list for every channel delivery — and a default-deny gate
// asked about nobody refuses nobody.
//
// The dispatcher's call is THE AUTHORITATIVE CHECK. Consent is also verified at
// request time, but transmission happens later and a recipient can withdraw in
// between; that earlier check fails fast and does not stand in for this one.
//
// It must distinguish an ANSWER from a FAULT: apperrors.ErrConsentNotGranted
// says consent is absent, and every other error says the question could not be
// asked. The dispatcher parks on the first and retries on the second.
type ConsentGate interface {
	RequireGrantedForRecipients(ctx context.Context, recipients []connector.Recipient, purposeKey string) error
	// AuthorizeTransmit answers whether this delivery may go out NOW and
	// records the answer per recipient before any provider I/O. The ticket it
	// returns is what transmit demands: a send with no current ticket is a
	// send nobody can account for.
	AuthorizeTransmit(ctx context.Context, req commsauthz.TransmitRequest) (commsauthz.TransmitTicket, error)
}

// BounceObserver is told when a delivery report says an address is permanently
// gone, so the module that owns communication policy can stop writing to it.
//
// Declared HERE by the consumer and implemented by consent: comms owns the send
// ledger and must not know what a suppression is.
//
// It runs in the SAME transaction as the bounce mark, so the stop and the
// failure that earned it commit together. A stop against a rolled-back report
// would refuse mail on a failure that never happened; a bounce without its stop
// leaves the address dead on the record and live to the send path.
//
// An error FAILS the bounce recording, which is the safe direction: the provider
// redelivers the report, so failing costs a retry while swallowing costs a dead
// address nobody stops writing to.
type BounceObserver interface {
	HardBounceTx(ctx context.Context, tx pgx.Tx, fact HardBounceFact) error
}

// HardBounceFact is one permanently failed delivery, as the observer needs it.
//
// It carries the address rather than the whole report: the reason text is
// external input already stored on the comms_outbound row, and handing it
// across the seam would invite a second copy of unbounded remote text into
// another module's tables.
// It carries NO contact id, deliberately. Resolving an address to the record
// that owns it is a question about contacts, and the observer's own module
// already reads contact_email to answer questions like it — asking comms to do
// it would put a cross-module read in the ledger that has no other reason to
// know records exist.
type HardBounceFact struct {
	Address    string
	DeliveryID ids.UUID
}

// SeatAuthority answers whether the human whose mailbox is about to transmit is
// still a live, mutation-capable seat, and if not, why. Deactivation revokes
// sessions and passports, but a delivery staged beforehand carries no session of
// its own, so without this an off-boarded account's batch keeps leaving their
// mailbox. A DOWNGRADE binds the same way: a read seat may read but never send,
// whatever staged it.
//
// It reports an ANSWER as (false, reason) and a FAULT as an error, the split the
// consent gate makes: a decision the dispatcher honours by parking with the
// reason named, against a failure to learn it that must not destroy a
// legitimate send.
type SeatAuthority interface {
	// ActiveSeat reports whether userID is a live, mutation-capable seat in
	// the workspace bound on ctx. reason is empty exactly when active is
	// true; when active is false, reason is the sentence the delivery parks
	// with, and it must say WHICH answer this is — an operator reading the
	// park record needs to tell a deactivated account from a live seat this
	// installation never let send.
	ActiveSeat(ctx context.Context, userID ids.UserID) (active bool, reason string, err error)
}

// AttachmentAuthority answers, at transmit time, whether the files a delivery
// carries may still leave the building.
//
// A DELIVERY IS NOT SENT WHEN IT IS STAGED. Between the human pressing send and
// the provider call, a document can be archived and the sender can lose the row
// scope that let them attach it, so the staging check cannot answer for this
// moment — and a message mailing a file its sender may no longer read carries
// that sender's own address out with it.
//
// Same answer-versus-fault split as SeatAuthority.
type AttachmentAuthority interface {
	// EnsureTransmittable reports whether every attachment is still visible
	// to userID in the workspace bound on ctx. reason is empty
	// exactly when ok is true; when ok is false it is the sentence the delivery
	// parks with, and it names WHICH file and WHY, because a park record
	// reading "an attachment cannot be sent" leaves the sender guessing which
	// of several to fix.
	EnsureTransmittable(ctx context.Context, userID ids.UserID, attachmentIDs []ids.UUID) (ok bool, reason string, err error)

	// ReadForSend returns the bytes of each attachment, in the order asked.
	//
	// Separate from EnsureTransmittable because they answer different
	// questions at different costs: may this still be sent, and what is in it.
	// The gate runs first and refuses for free; only a delivery that survives
	// it pays to read the objects.
	//
	// It is called at TRANSMIT, not at staging. A delivery sits on a retry
	// ladder for as long as the maximum age allows, and holding every
	// attachment's bytes in the database for that long — duplicated per
	// delivery — is a cost with no reader.
	ReadForSend(ctx context.Context, userID ids.UserID, attachmentIDs []ids.UUID) ([][]byte, error)
}

// ErrNoMailbox marks a user with no connection to the provider a delivery is
// staged against. There is nothing to retry against, so it parks.
var ErrNoMailbox = errors.New("comms: no mailbox is connected for this provider")

// ErrCannotSend marks a connected provider whose connector cannot transmit —
// it implements capture only. No retry turns a capture-only connector into a
// sender, so this parks too.
var ErrCannotSend = errors.New("comms: this connector cannot transmit messages")

// ErrProviderNotConfigured marks a provider this installation has no
// integration for: the delivery names it, the deployment configured no
// connector to reach it through, and nothing in the process will grow one.
//
// It PARKS rather than retries, and that is the whole reason it is a sentinel
// of its own. Read as a transient fault it is indistinguishable from a provider
// outage, so every attempt fails identically until the runner's ladder is spent
// — and the exhaustion guard runs after this point, so nothing else would ever
// move the row. It would stay pending forever, looking live and never sending.
// Parked, the row carries a reason an operator can act on, and a re-send after
// they configure the integration is one new delivery.
var ErrProviderNotConfigured = errors.New("comms: no integration for this provider is configured on this installation")

// ConnectionResolver resolves the transmitting mailbox: the connector's send
// seam, its unsealed credential, and the scopes the provider says the grant
// actually holds.
//
// ErrNoMailbox, ErrCannotSend and ErrProviderNotConfigured are the only facts
// about the deployment; EVERY OTHER ERROR IS TRANSIENT. A keyvault blip or a
// database timeout here is a failure to get an answer, and parking on one would
// permanently destroy a legitimate send that nothing is wrong with.
type ConnectionResolver interface {
	Resolve(ctx context.Context, userID ids.UserID, provider string) (connector.EmailSender, connector.Auth, []string, error)

	// ResolveChannel resolves the transmitting channel binding for provider, AS
	// userID. A workspace-wide core connector ignores userID — the binding is the
	// workspace's — and it is threaded through so a PER-MEMBER credential has
	// somewhere to resolve against later. The seat check does not move: the human
	// who staged is re-read at transmit time on either transport.
	//
	// No scope list, for the reason SendsWithoutScope names: a bot token carries
	// no OAuth grant, and an empty list would be a refusal rather than an absence.
	//
	// It reports the SAME three deployment facts Resolve does; every other error
	// is transient, including a workspace holding two live bindings, which is a
	// fault an operator repairs.
	ResolveChannel(ctx context.Context, userID ids.UserID, provider string) (connector.MessageSender, connector.Auth, error)
}

// consentRecipients is every subject this delivery reaches, in the vocabulary
// the suppression gate answers about. It is the ONE place a delivery's shape is
// read on the way to that gate, which is what lets the dispatcher ask the same
// question once for both transports instead of branching before a default-deny
// check.
//
// A channel delivery reaches exactly one subject: the channel has no Cc, and
// the provider plus the recipient's account id ARE the resolution key
// (connector.ChannelIdentity). The username is deliberately absent — a handle
// can be released and re-claimed, so nothing may resolve or authorize on it.
func consentRecipients(del Delivery) []connector.Recipient {
	if del.IsChannel() {
		return []connector.Recipient{{Channel: &connector.ChannelIdentity{
			Provider:      del.Provider,
			ChannelUserID: del.ChannelRecipient(),
		}}}
	}
	return connector.EmailRecipients(addressees(del))
}

// addressees is every contact this delivery reaches — To, Cc and Bcc together,
// in that order, deduplicated case- and space-insensitively the way a mail
// server treats an address.
//
// The delivery stores the three lists apart because the wire needs them apart,
// and consent is owed to EVERY addressee. Gating on To alone would leave a Cc'd
// contact no suppression at all, and a blind copy is the same contact with less
// visibility, not less standing — the invisibility is why omitting them would go
// unnoticed.
//
// It fills a slice of its own rather than appending onto the delivery's, because
// the wire rendering reads Recipients and Cc as the separate lists they are.
//
// It appends the NORMALIZED address, so the key it dedupes on and the value it
// hands the gate are one string. Passing the padded spelling would ask about an
// address the gate cannot resolve — a legitimate send parked as "consent not
// granted", which reads as a recipient who opted out.
func addressees(del Delivery) []string {
	size := len(del.Recipients) + len(del.Cc) + len(del.Bcc)
	all := make([]string, 0, size)
	seen := make(map[string]bool, size)
	for _, list := range [][]string{del.Recipients, del.Cc, del.Bcc} {
		for _, addr := range list {
			key := strings.ToLower(strings.TrimSpace(addr))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, key)
		}
	}
	return all
}

// SendCapability is what this installation knows about a provider's ability to
// transmit, and it has THREE states because two cannot express a bot token: the
// token holds no OAuth scope, yet it sends. Collapsing "sends without a scope"
// into "cannot send" would read a channel provider as capture-only and park
// every message it was ever handed, with a reason naming a connector limitation
// that does not exist.
type SendCapability int

const (
	// CannotSend means nothing on this installation transmits for the provider.
	// It is the zero value, so a capability nobody answered refuses rather than
	// sends.
	CannotSend SendCapability = iota
	// SendsWithScope means the provider's grant must hold a named OAuth scope
	// before its credential may transmit, and SendScopeFor's first return names
	// that scope.
	SendsWithScope
	// SendsWithoutScope means the credential IS the whole authority and there is
	// no scope to intersect — a Telegram bot token authorizes sendMessage by
	// itself. SendScopeFor returns an empty scope, and that emptiness is not a
	// refusal.
	SendsWithoutScope
)

// channelProviders mirrors activities.SetChannelProviders/channelProviders —
// a SEPARATE package-level set, because comms and activities are siblings
// under internal/modules and neither may import the other. Both are set from
// the SAME boot-time reconcile in internal/compose (DESIGN-SP4 §4), which is
// what keeps the two from drifting apart independently, even though each
// package still holds its own copy.
var (
	channelProvidersMu sync.RWMutex
	channelProviders   = map[string]bool{"telegram": true}
)

// SetChannelProviders replaces the derived channel-provider set wholesale.
// Last-write-wins, not once-only: see activities.SetChannelProviders's doc for
// why a plain settable var is the right shape here, not a register-and-panic
// one.
func SetChannelProviders(providers []string) {
	next := make(map[string]bool, len(providers))
	for _, p := range providers {
		next[p] = true
	}
	channelProvidersMu.Lock()
	channelProviders = next
	channelProvidersMu.Unlock()
}

// mailSendScopes are the OAuth permissions a MAIL grant must carry to transmit,
// per provider.
//
// A map rather than a chain of ifs because there are now two vendors and the
// chain's failure mode is silent: a provider missing from it reports CannotSend,
// and every one of its deliveries parks with "provider cannot send messages" —
// a connector limitation that does not exist. Each value is a SECOND spelling of
// a string a capture provider already declares (this module must not import
// one), and compose holds the two against each other per provider in
// compose/sendscope_test.go.
var mailSendScopes = map[string]string{
	"gmail": "https://www.googleapis.com/auth/gmail.send",
	"graph": "Mail.Send",
	// test_mailbox has no real OAuth grant — the connector always reports
	// holding this scope (testmailbox.GrantedScopes) — so mailAppConfigured
	// (the deployment flag) is the one gate that decides whether it can
	// transmit at all.
	"test_mailbox": "urn:margince:test_mailbox:send",
}

// MailSendProviders names every provider this module hands a send scope to.
//
// Exported so the gate that binds these strings to the connectors' own
// constants can derive its corpus from THIS map rather than restating it. A
// second list in the test would be a second answer, and the failure it would
// hide is precisely the one that matters: a provider added here and nowhere
// else, whose scope nothing checks against what its connector re-checks.
func MailSendProviders() []string {
	out := make([]string, 0, len(mailSendScopes))
	for provider := range mailSendScopes {
		out = append(out, provider)
	}
	sort.Strings(out)
	return out
}

// SendScopeFor answers whether a provider can transmit and, when its grant must
// carry an OAuth scope to do so, which scope.
//
// Exported so the request-time pre-flight asks the SAME question as the
// authority gate: two spellings of "may this grant send" could disagree, and a
// pre-flight accepting what the gate then parks is worse than none.
//
// The MAIL arm reads mailSendScopes above: gmail and graph are not activity
// kinds, so there is no registry to derive from. See that map for what holds
// those strings to the first spelling.
//
// The CHANNEL arm derives from channelProviders — the same registry
// activities.IsChannelKind reads — so a provider clearing it is answerable
// for both "is this a channel conversation" and "can this installation send
// on it" in one boot-time act.
func SendScopeFor(provider string) (string, SendCapability) {
	if scope, ok := mailSendScopes[provider]; ok {
		return scope, SendsWithScope
	}
	channelProvidersMu.RLock()
	isChannel := channelProviders[provider]
	channelProvidersMu.RUnlock()
	if isChannel {
		return "", SendsWithoutScope
	}
	return "", CannotSend
}

// rfc8058Post derives the List-Unsubscribe-Post header from its partner. RFC
// 8058 fixes the value, so it is derived rather than stored and the pair
// cannot drift apart — a Post header without a target instructs a mail client
// to POST nowhere.
func rfc8058Post(listUnsubscribe string) string {
	if listUnsubscribe == "" {
		return ""
	}
	return "List-Unsubscribe=One-Click"
}
