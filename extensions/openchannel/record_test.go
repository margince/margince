// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// arrivalJSON builds one sender's document. Fields a test does not care about
// are filled with values that keep the record buildable, so a test naming one
// refusal is not also exercising four others.
func arrivalJSON(tb testing.TB, doc arrival) []byte {
	tb.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		tb.Fatalf("building the fixture: %v", err)
	}
	return raw
}

func landableArrival() arrival {
	return arrival{
		MessageID: "m-9182",
		ThreadID:  "t-44",
		Subject:   "Re: the quote",
		Body:      "Can you send it again?",
		From:      party{Account: "acct-77", Email: "buyer@example.com", Name: "Ada Buyer"},
		To:        party{Email: "rep@margince.test"},
	}
}

// The declaration spells its ingress system and its channel provider as
// LITERALS, because the operator manifest is derived from New's AST without
// compiling it. Every record the drain builds reaches for the constants. A
// mismatch on either is silent and expensive: the core refuses a record naming a
// source the unit did not declare, and a provider that is not the declared one
// lands a message on a transport nothing can reply through.
func TestTheDeclaredIngressSourceIsTheOneRecordsName(t *testing.T) {
	t.Parallel()
	declared := New().Ingress
	if len(declared) != 1 {
		t.Fatalf("the unit declares %d ingress sources; this test knows about one", len(declared))
	}
	if declared[0].System != ingressSystem {
		t.Fatalf("the declaration names %q and records name %q", declared[0].System, ingressSystem)
	}
	rec, err := recordFor(ownerRef, arrivalJSON(t, landableArrival()), signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	if rec.System != declared[0].System {
		t.Fatalf("a built record names source %q and the declaration is %q", rec.System, declared[0].System)
	}
	// The same check the manifest generator and the boot preflight run, so a
	// declaration this unit could not land under fails here rather than at boot.
	if err := declared[0].Validate(); err != nil {
		t.Fatalf("the declared source is not usable: %v", err)
	}
}

func TestTheDeclaredChannelProviderIsTheOneRecordsName(t *testing.T) {
	t.Parallel()
	declared := New().Channels
	if len(declared) != 1 {
		t.Fatalf("the unit declares %d channels; this test knows about one", len(declared))
	}
	if declared[0].Provider != provider {
		t.Fatalf("the declaration supplies %q and records name %q", declared[0].Provider, provider)
	}
	rec, err := recordFor(ownerRef, arrivalJSON(t, landableArrival()), signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	if rec.Activity.ChannelProvider != declared[0].Provider {
		t.Fatalf("a built record carries provider %q and the declaration supplies %q",
			rec.Activity.ChannelProvider, declared[0].Provider)
	}
	if rec.Counterparty.ChannelIdentity.Provider != declared[0].Provider {
		t.Fatalf("the counterparty is bound at provider %q and the declaration supplies %q",
			rec.Counterparty.ChannelIdentity.Provider, declared[0].Provider)
	}
	if err := declared[0].Validate(); err != nil {
		t.Fatalf("the declared channel is not mountable: %v", err)
	}
}

// A declared Send obliges a declared Live. The core refuses the pair at boot;
// what this holds is that the unit ships both functions rather than a name.
func TestTheTransportCanSayWhetherItStillMay(t *testing.T) {
	t.Parallel()
	declared := New().Channels[0]
	if declared.Send == nil {
		t.Fatal("the unit declares a channel with no sender, so nothing can be replied to")
	}
	if declared.Live == nil {
		t.Fatal("the unit declares a sender with no liveness check, so the core must guess whether a member may still send")
	}
}

// The natural key is what makes a redelivery land nothing rather than a second
// timeline entry, so it has to be namespaced by the endpoint: two senders
// numbering their messages from one share this unit's single provenance
// namespace.
func TestTheNaturalKeyIsNamespacedByTheAddressItArrivedOn(t *testing.T) {
	t.Parallel()
	body := arrivalJSON(t, landableArrival())
	mine, err := recordFor(ownerRef, body, signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	theirs, err := recordFor("a-second-endpoints-ref", body, signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	if mine.Key == theirs.Key {
		t.Fatalf("two endpoints built key %q from the same message id, so one member's message would land as a replay of another's", mine.Key)
	}
	if mine.ThreadKey == theirs.ThreadKey {
		t.Fatalf("two endpoints built thread key %q, so a stranger's conversation joins onto this one", mine.ThreadKey)
	}
}

// The core decides whether a message is purely internal by asking whether every
// party is on the installation's own domains, and it reads an empty set as "this
// connector could not enumerate them", which KEEPS the message. So a record
// naming one end would silently disable the gate rather than pass it.
func TestBothEndsOfAMessageAreNamed(t *testing.T) {
	t.Parallel()
	rec, err := recordFor(ownerRef, arrivalJSON(t, landableArrival()), signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	want := map[string]bool{"buyer@example.com": true, "rep@margince.test": true}
	if len(rec.Addresses) != len(want) {
		t.Fatalf("the record names %v; both ends of the message have to be there", rec.Addresses)
	}
	for _, address := range rec.Addresses {
		if !want[address] {
			t.Fatalf("the record names %q, which is neither end of the message", address)
		}
	}
}

// An unidentifiable sender — neither account nor email — must not leave the
// record naming only OUR OWN member's address. A set with just that one
// address is exactly what the internal-message gate reads as "every party is
// on our own domain", and it would drop a message from a real outside sender
// this connector simply could not identify. The empty set is the case the
// core's own comment describes: "could not enumerate the parties" — not "we
// enumerated one and it was ours".
func TestAnUnidentifiableSenderNamesNoAddresses(t *testing.T) {
	t.Parallel()
	doc := landableArrival()
	doc.From = party{Name: "Someone Outside"}
	rec, err := recordFor(ownerRef, arrivalJSON(t, doc), signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	if len(rec.Addresses) != 0 {
		t.Fatalf("the record names addresses %v for a sender it could not identify at all — the recipient's own address alone would read as wholly internal and get the message dropped", rec.Addresses)
	}
}

// An account outranks an address: it is the key a reply is routed on, so a
// record carrying one is repliable and a record carrying only an address is not.
// The address rides along as corroboration where this connector holds both.
func TestAnAccountNamesTheCounterpartyAndTheAddressCorroborates(t *testing.T) {
	t.Parallel()
	rec, err := recordFor(ownerRef, arrivalJSON(t, landableArrival()), signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	if rec.Counterparty.ChannelIdentity.ChannelUserID != "acct-77" {
		t.Fatalf("the counterparty is bound to %q; a message with an account is repliable through it",
			rec.Counterparty.ChannelIdentity.ChannelUserID)
	}
	if rec.Counterparty.Email != "buyer@example.com" {
		t.Fatalf("the counterparty carries address %q; dropping it throws away evidence only this side holds", rec.Counterparty.Email)
	}
	// A channel record short-circuits past the domain-keyed suppression gates,
	// so it names no domain — naming one would be answering a question that is
	// not asked of this shape.
	if rec.Counterparty.Domain != "" {
		t.Fatalf("a channel-identified counterparty named domain %q", rec.Counterparty.Domain)
	}
}

// The case this connector exists for: a party identified by an opaque account
// and nothing else, on both ends. The core admits an empty address set precisely
// when the counterparty names no email, because the internal-message gate reads
// every party from that set and over an empty one answers "not internal" and
// keeps the record. Refusing it here would park every message from a provider
// that issues account ids and no mail.
func TestAMessageNamingOnlyAccountsIsLandable(t *testing.T) {
	t.Parallel()
	doc := landableArrival()
	doc.From = party{Account: "acct-77", Name: "Ada Buyer"}
	doc.To = party{Account: "acct-mine"}
	rec, err := recordFor(ownerRef, arrivalJSON(t, doc), signedAt)
	if err != nil {
		t.Fatalf("a message identified only by account was refused: %v", err)
	}
	if len(rec.Addresses) != 0 {
		t.Fatalf("the record names addresses %v that the document did not carry", rec.Addresses)
	}
	if rec.Counterparty.Email != "" {
		t.Fatalf("the counterparty carries address %q the document did not name", rec.Counterparty.Email)
	}
	if rec.Counterparty.ChannelIdentity.ChannelUserID != "acct-77" {
		t.Fatal("the counterparty is not repliable, which is the whole of what a channel record is for")
	}
	// The core's own door, so this test fails if the two rules ever diverge
	// rather than only if this unit's does.
	if err := rec.Validate(); err != nil {
		t.Fatalf("the core refuses a record this unit built: %v", err)
	}
}

func TestAnAddressOnlySenderGoesThroughTheMailLadder(t *testing.T) {
	t.Parallel()
	doc := landableArrival()
	doc.From.Account = ""
	rec, err := recordFor(ownerRef, arrivalJSON(t, doc), signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	if rec.Counterparty.ChannelIdentity.ChannelUserID != "" {
		t.Fatal("a sender with no account was bound to a channel identity, which a reply would then be routed on")
	}
	if rec.Counterparty.Domain != "example.com" {
		t.Fatalf("the counterparty names domain %q; the core's suppression gates key on it and read an empty one as 'keep'", rec.Counterparty.Domain)
	}
}

// The instant the sender SIGNED stands in when the document names none. A
// timeline ordered by when a drain ran would be a timeline of this system's own
// scheduling.
func TestAMessageWithNoTimeTakesTheInstantItWasSigned(t *testing.T) {
	t.Parallel()
	rec, err := recordFor(ownerRef, arrivalJSON(t, landableArrival()), signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	if !rec.Activity.OccurredAt.Equal(signedAt) {
		t.Fatalf("the record happened at %s and the sender signed at %s", rec.Activity.OccurredAt, signedAt)
	}
	doc := landableArrival()
	doc.OccurredAt = signedAt.Add(-2 * time.Hour)
	rec, err = recordFor(ownerRef, arrivalJSON(t, doc), signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	if !rec.Activity.OccurredAt.Equal(doc.OccurredAt) {
		t.Fatalf("the record happened at %s and the sender said %s", rec.Activity.OccurredAt, doc.OccurredAt)
	}
}

// The bytes are kept VERBATIM as evidence. A re-encoding of the fields this
// connector happens to read is exactly the thing evidence is not.
func TestTheEvidenceIsTheDocumentThatWasSigned(t *testing.T) {
	t.Parallel()
	body := arrivalJSON(t, landableArrival())
	rec, err := recordFor(ownerRef, body, signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	if string(rec.Raw) != string(body) {
		t.Fatalf("the record keeps\n%s\nand the sender posted\n%s", rec.Raw, body)
	}
}

// Each refusal below is a request a member sees PARKED on their own screen, so
// each has to be a refusal this connector makes rather than one the core makes
// on its behalf with a message nobody can act on.
func TestADocumentThisConnectorCannotLandIsRefusedByName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"a body that is not the document at all", []byte("not json")},
		{"a document naming no message id", arrivalJSON(t, arrival{
			From: party{Email: "buyer@example.com"},
		})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := recordFor(ownerRef, tc.body, signedAt)
			if !errors.Is(err, errPayload) {
				t.Fatalf("the document was answered %v; a document this connector cannot land is its own refusal, and it parks the request rather than retrying it", err)
			}
			class, terminal := drainFailure(err)
			if class.Class != classPayloadUnusable.Class || !terminal {
				t.Fatalf("it was classified %q terminal=%t", class.Class, terminal)
			}
		})
	}
}

// A message belonging to no thread is its own thread. Falling back to nothing
// would put every threadless message from one endpoint into one conversation.
func TestAMessageWithNoThreadIsItsOwn(t *testing.T) {
	t.Parallel()
	doc := landableArrival()
	doc.ThreadID = ""
	rec, err := recordFor(ownerRef, arrivalJSON(t, doc), signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	other := doc
	other.MessageID = "m-9183"
	second, err := recordFor(ownerRef, arrivalJSON(t, other), signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	if rec.ThreadKey == second.ThreadKey {
		t.Fatalf("two threadless messages share thread key %q, so unrelated messages read as one conversation", rec.ThreadKey)
	}
}

// A message lands as the MESSAGE kind on this unit's transport — the two axes
// stated separately. A unit that named a kind of its own would undo that
// separation from the outside.
func TestAMessageLandsAsAMessageOnThisUnitsTransport(t *testing.T) {
	t.Parallel()
	rec, err := recordFor(ownerRef, arrivalJSON(t, landableArrival()), signedAt)
	if err != nil {
		t.Fatalf("building a record: %v", err)
	}
	if rec.Activity.Kind != extension.ActivityKindMessage {
		t.Fatalf("a captured message landed as kind %q", rec.Activity.Kind)
	}
	if rec.Activity.Direction != extension.DirectionInbound {
		t.Fatalf("a message that arrived here landed as %q", rec.Activity.Direction)
	}
}
