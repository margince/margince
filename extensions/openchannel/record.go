// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// The envelope a sender POSTs, and the record one accepted request becomes.
//
// THE UNIT DECIDES THE ENVELOPE because nobody else can. The core admitted these
// bytes on a signature and interpreted nothing in them; a connector is exactly
// the thing that says what a remote system's document MEANS here. So this file
// is the connector's published contract with whoever configures the sender, and
// every refusal below is one that reaches that person as a parked request on
// their own screen rather than as a silently dropped message.
//
// It is deliberately SMALL. A field this connector cannot land is a field a
// sender would fill in and never see used, and each one added is a promise the
// core's own record shape has to keep.

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// ingressSystem is the declared source this unit lands under, spelled once. The
// core pairs it with the unit name to derive `ext:openchannel:openchannel`, the
// provenance every landed record carries, so this constant and the literal in
// the declaration are the same string on purpose.
//
// Held by: TestTheDeclaredIngressSourceIsTheOneRecordsName (extensions/openchannel/record_test.go)
const ingressSystem = "openchannel"

// provider is this unit's key in channel_provider: the TRANSPORT a message it
// carries names, never the kind. A message lands as kind `message` with this
// name on its own column, which is the separation the whole channel design rests
// on.
//
// Held by: TestTheDeclaredChannelProviderIsTheOneRecordsName (extensions/openchannel/record_test.go)
const provider = "openchannel"

// arrival is the document a sender signs and posts.
type arrival struct {
	// MessageID is the sending system's own identifier for this message, and it
	// is the half of the natural key that makes a redelivery a no-op. It must be
	// the value that system reports identically on a re-read: an id derived from
	// a timestamp or a position produces a second timeline entry every time.
	MessageID string `json:"message_id"`
	// ThreadID is the conversation this belongs to at the sending system, empty
	// for a message that is its own.
	ThreadID string `json:"thread_id"`
	// OccurredAt is when the message happened THERE. Empty falls back to the
	// instant the sender signed, which is the closest thing this side holds; a
	// timeline ordered by when a drain ran would be a timeline of this system's
	// own scheduling.
	OccurredAt time.Time `json:"occurred_at"`
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	// From is the party at the other end, and To is the member as the sending
	// system addresses them. BOTH are named because the core decides whether a
	// message is purely internal — colleagues talking, which is not evidence of
	// a customer relationship — by asking whether every party is on the
	// installation's own domains, and it reads an empty set as "this connector
	// could not enumerate the parties", which keeps the message. A record naming
	// one end would silently disable that gate rather than pass it.
	From party `json:"from"`
	To   party `json:"to"`
}

// party is one end of a message as the sending system names it.
type party struct {
	// Account is that system's own id for the human, and it is what makes a
	// captured message repliable: the core binds it to a person and the reply
	// path resolves the recipient from that binding.
	Account string `json:"account"`
	Email   string `json:"email"`
	Name    string `json:"name"`
}

// recordFor builds the record one accepted request lands as, or says why it
// never can.
//
// ref namespaces the natural key. Two senders numbering their messages from one
// share a provenance namespace here, so a bare id would have the second one's
// message land as a replay of the first's — and the ref is per endpoint, minted
// once and unchanged by re-opening.
func recordFor(ref string, body []byte, sentAt time.Time) (extension.Record, error) {
	var doc arrival
	if err := json.Unmarshal(body, &doc); err != nil {
		return extension.Record{}, fmt.Errorf("%w: the request body is not the document this connector reads", errPayload)
	}
	if strings.TrimSpace(doc.MessageID) == "" {
		return extension.Record{}, fmt.Errorf("%w: the request names no message_id, which is what makes a redelivery land nothing rather than a second copy", errPayload)
	}
	// An empty address set is NOT refused, and the reason is the case this
	// connector exists for: a party identified by an opaque account and nothing
	// else. The core states the rule on Record.Validate — an empty set is
	// admitted precisely when the counterparty names no email, because the
	// internal-message gate reads every party from that set and over an empty
	// one answers "not internal" and keeps the record.
	//
	// The pairing that WOULD be wrong — a counterparty named by address while no
	// addresses are listed — cannot be built here: addressesOf reads the sender's
	// address, so an empty set already means the sender named none. The core's
	// door holds it anyway for a record assembled some other way.
	addresses := addressesOf(doc)
	occurred := doc.OccurredAt
	if occurred.IsZero() {
		occurred = sentAt
	}
	return extension.Record{
		System: ingressSystem,
		Key:    ref + ":" + doc.MessageID,
		Activity: extension.ActivityFields{
			// A message, on this unit's own transport — the two axes stated
			// separately. The provider is a LITERAL here for the reason the
			// declaration's is: the core holds a unit to the channels it
			// DECLARED, and that declaration is read statically from the AST
			// without compiling the unit.
			Kind:            extension.ActivityKindMessage,
			ChannelProvider: "openchannel",
			Subject:         strings.TrimSpace(doc.Subject),
			Body:            strings.TrimSpace(doc.Body),
			OccurredAt:      occurred,
			Direction:       extension.DirectionInbound,
		},
		// Namespaced by this unit AND by the endpoint. A bare thread id would
		// share activity.thread_key with every other source, where two of them
		// can collide and join a stranger's conversation onto this one. A
		// message with no thread is its own, keyed on the id that is already
		// unique within the endpoint.
		ThreadKey:    provider + ":" + ref + ":" + threadOf(doc),
		Counterparty: counterpartyOf(doc.From),
		Addresses:    addresses,
		// The bytes as they arrived, kept as evidence — the document the sender
		// signed rather than a re-encoding of the fields above, so what the
		// installation stores is what was actually posted.
		Raw: body,
	}, nil
}

// threadOf is the conversation half of the thread key: the sender's own thread
// id, or the message's id for a message that belongs to no thread.
func threadOf(doc arrival) string {
	if thread := strings.TrimSpace(doc.ThreadID); thread != "" {
		return thread
	}
	return doc.MessageID
}

// addressesOf enumerates every party the record names, dropping the blanks.
//
// Blanks are dropped rather than carried because the core's internal-message
// gate skips what it cannot read: a blank element is the same hole an empty set
// is, one party at a time.
//
// A SENDER WHO NAMED NO ADDRESS forces the whole set empty, even
// though `to` is almost always present: the gate reads every address in the set
// as one of the parties to the message, and a set holding only OUR OWN member's
// address is indistinguishable from "every party is on our own domain" — it
// would judge a message from a real, merely-unidentifiable outside sender as
// wholly internal and drop it. The empty set is the case the core already
// reserves for exactly this: "could not enumerate the parties", which keeps the
// record rather than passing it through a domain check it cannot honestly
// answer.
func addressesOf(doc arrival) []string {
	// THE SENDER'S ADDRESS DECIDES WHETHER THERE IS A SET AT ALL, and an account
	// does not stand in for it. The core reads this set to ask whether every
	// party is on the installation's own domains, and answers "yes" for a set
	// holding only the member's own address — so a sender who named no address
	// would have their message read as colleagues talking and dropped, however
	// well identified they are by anything else.
	//
	// An account-only sender is not a degenerate case here, it is the case this
	// connector exists for: a party a remote system knows by an opaque id and
	// nothing more. Empty is the answer that keeps them, because the core
	// documents an empty set as "this connector could not enumerate the
	// parties" and keeps the record rather than judging it.
	if strings.TrimSpace(doc.From.Email) == "" {
		return nil
	}
	var found []string
	for _, address := range []string{doc.From.Email, doc.To.Email} {
		if trimmed := strings.TrimSpace(address); trimmed != "" {
			found = append(found, trimmed)
		}
	}
	return found
}

// counterpartyOf names the human at the other end, and the shape decides which
// resolution ladder runs.
//
// AN ACCOUNT OUTRANKS AN ADDRESS, which is the core's rule rather than this
// unit's preference: the account is the key a reply is routed on, so a record
// carrying one is repliable and a record carrying only an address is not. Where
// this unit holds both, the address RIDES ALONG as corroboration — the colleague
// already captured from mail is recognised as the same human instead of quietly
// becoming a second contact — and the core admits it because the ingress source
// declares the email merge key.
func counterpartyOf(from party) extension.Counterparty {
	name := strings.TrimSpace(from.Name)
	if account := strings.TrimSpace(from.Account); account != "" {
		return extension.Counterparty{
			DisplayName: name,
			Direction:   extension.DirectionInbound,
			ChannelIdentity: extension.ChannelIdentity{
				Provider:      provider,
				ChannelUserID: account,
				DisplayName:   name,
			},
			Email: strings.TrimSpace(from.Email),
		}
	}
	return extension.Counterparty{
		Email:       strings.TrimSpace(from.Email),
		DisplayName: name,
		// The core's suppression gates key on the domain, and a record that left
		// it empty would not be opting out of them — it would be failing to
		// answer, which those gates read as "keep". A channel record short-
		// circuits past every one of them, which is why the arm above names none.
		Domain:    mailDomain(from.Email),
		Direction: extension.DirectionInbound,
	}
}

// mailDomain is the lower-cased domain half of an address, or empty when there
// is not one.
func mailDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[at+1:]))
}
