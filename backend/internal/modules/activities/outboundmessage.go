// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// One send's derived facts, and the two rows they become.
//
// Apart from the send decision itself because they answer a different question:
// SendEmail decides WHETHER a message may go, and this decides what the
// timeline and the delivery each receive once it may. The split matters most
// where the two rows deliberately DIFFER — the wire body carries a live
// unsubscribe token and the recorded one carries a redacted copy, and having
// both projections in one place is what keeps that difference deliberate.

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// outboundMessage is one send's derived facts, computed before the
// transaction opens so the transaction holds writes only. The timeline row and
// the delivery are two renderings of THIS value, which is why they are built
// side by side: a field that disagreed between them would be a message whose
// record and whose transmission say different things.
type outboundMessage struct {
	in        SendEmailInput
	messageID string
	// body is what the recipient receives; recordedBody is what the
	// workspace keeps. They differ by exactly one thing — the live
	// preference token the footer carries — because the timeline row is
	// served back to any seat holding activity:read, and that token is a
	// bearer credential over the recipient's consent record (see
	// redactedToken). Only the delivery may read body.
	body         string
	recordedBody string
	// htmlBody is the markup alternative, empty for a plain-text send. It
	// carries the SAME sign-off and the same unsubscribe footer as body: two
	// alternatives of one message that disagreed would be two messages, and the
	// recipient's client decides which one they read.
	htmlBody string
	// fromName is who the recipient sees this is from. Resolved at send time
	// from the authenticated sender rather than at transmit, so a rename
	// between attempts cannot change a message already in flight.
	fromName string
	// files is what this message carries, already snapshotted.
	files           []OutboundFile
	listUnsubscribe string
	to              []string
	links           []ActivityLinkInput
	// provider is the mailbox this send goes out through, resolved once per
	// send. It is used twice below — the activity's source_system and the
	// delivery's provider — and the two must be the same value or the captured
	// echo keys onto nothing and lands as a second timeline row.
	provider string
}

// htmlAlternativeNote marks a timeline row whose message also went out as
// markup. A fixed sentence rather than the markup itself: the row is served to
// every seat holding activity:read and rendering a sender's HTML there would
// put caller-supplied markup into our own document.
const htmlAlternativeNote = "This message was also sent as HTML."

// activity is the timeline row the send commits.
func (m outboundMessage) activity(chain threading) LogActivityInput {
	direction, sourceSystem := "outbound", m.provider
	recorded := m.recordedBody
	if m.htmlBody != "" {
		// The timeline keeps the PLAIN alternative, and says so when a markup
		// one also went out.
		//
		// The two parts are supplied independently by the caller, so nothing
		// makes them say the same thing — a recipient whose client renders
		// markup can read something the plain part never mentioned. Every
		// activity:read on this row would otherwise show only the benign half
		// and give no sign the other exists, which is worse than showing markup
		// nobody can read: it is a record that looks complete.
		recorded += "\n\n[" + htmlAlternativeNote + "]"
	}
	return LogActivityInput{
		Kind:         "email",
		Subject:      &m.in.Subject,
		Body:         &recorded,
		Direction:    &direction,
		Links:        m.links,
		Source:       sourceManual,
		SourceSystem: &sourceSystem,
		SourceID:     &m.messageID,
		ThreadKey:    chain.threadKey,
		// This row IS the sent copy — its natural key is the one the provider's
		// echo carries, so the echo's upsert will find it and write nothing.
		// The correspondence evidence the echo used to bring therefore has to
		// be written here or it is never written at all (ADR-0072 §1: an
		// outbound activity to an address is what makes it
		// correspondence-positive).
		CounterpartyEmail:            primaryCounterparty(m.to, m.in.Recipients),
		CounterpartyOutboundAttested: true,
	}
}

// delivery is the same message as the delivery machinery receives it.
func (m outboundMessage) delivery(activityID ids.UUID, chain threading, authorization commsauthz.Request) DeliveryRequest {
	return DeliveryRequest{
		ActivityID:      ids.From[ids.ActivityKind](activityID),
		Provider:        m.provider,
		MessageID:       m.messageID,
		Recipients:      m.to,
		Cc:              m.in.Cc,
		Bcc:             m.in.Bcc,
		Subject:         m.in.Subject,
		Body:            m.body,
		HTMLBody:        m.htmlBody,
		FromName:        m.fromName,
		Attachments:     m.files,
		ConsentPurpose:  m.in.ConsentPurpose,
		Authorization:   authorization,
		InReplyTo:       chain.inReplyTo,
		References:      chain.references,
		ThreadKey:       chain.threadKey,
		ListUnsubscribe: m.listUnsubscribe,
	}
}

// authorization is the question the engine is asked about this message.
//
// Built here because this is where the whole message is known — the merged
// recipient list including blind copies, the content that will actually go out,
// and the purpose the caller named. A stager that rebuilt it would be reading
// the same message a second time, and the two readings would drift.
//
// Recipients is the MERGED list, not the To: line. A blind copy is blind to the
// other recipients and never to the engine.
func (m outboundMessage) authorization(origin SendOrigin) commsauthz.Request {
	return commsauthz.Request{
		Recipients:       connector.EmailRecipients(m.in.Recipients),
		Context:          m.in.Context,
		LegacyPurposeKey: m.in.ConsentPurpose,
		MarketingPurpose: m.in.MarketingPurpose,
		OperatorReason:   m.in.OperatorReason,
		Evidence:         m.evidence(origin),
		// The anchor is where a reply's own answer comes from: a message
		// continuing a thread the subject started is a reply whether or not the
		// caller thought to say so. Zero on an account-started send, which is
		// exactly what tells the engine there is no thread to lean on.
		AnchorActivityID: origin.anchor.UUID,
		Links:            linkedRecordIDs(m.links),
		Subject:          m.in.Subject,
		Body:             m.body,
	}
}

// evidence is what the caller offered, with the anchor filled in from the
// origin when they named no activity themselves.
//
// Derived rather than demanded: a rep replying in the compose window names
// nothing, and asking them to would be asking them to restate what the send
// already knows. A caller that DID name an activity keeps theirs — they may be
// pointing at an earlier message in the thread than the one being answered.
func (m outboundMessage) evidence(origin SendOrigin) commsauthz.Evidence {
	e := m.in.Evidence
	if e.ActivityID == (ids.UUID{}) {
		e.ActivityID = origin.anchor.UUID
	}
	return e
}

// linkedRecordIDs names the records this message is filed under, which is where
// a deal, an invoice or a contract validator looks when the caller named none.
func linkedRecordIDs(links []ActivityLinkInput) []ids.UUID {
	out := make([]ids.UUID, 0, len(links))
	for _, l := range links {
		out = append(out, l.EntityID)
	}
	return out
}
