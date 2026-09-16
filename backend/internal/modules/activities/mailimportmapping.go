// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The fields an importer supplies about a message it is handing over: who it
// was with, which message it is, and which conversation it belongs to.
//
// Capture derives all of this from the message itself, because it holds the
// message. An importer holds only what its source system kept, so it states
// them — and the two paths must agree on what the values MEAN or the same
// message read through the two doors is two different facts.
const (
	fieldParticipants  = "participants"
	fieldRFCMessageID  = "rfc_message_id"
	fieldThreadKey     = "thread_key"
	fieldICalUID       = "ical_uid"
	fieldICalInstance  = "ical_instance"
	fieldDirection     = "direction"
	fieldParticipantOf = "participants.from"
)

// Which kind may carry a field, for the refusal that names where it belongs.
const (
	onlyAnEmail  = "an email"
	onlyAMeeting = "a meeting"
)

// KindFieldError refuses a field on a kind that cannot carry it.
//
// The contract names `field_not_valid_for_kind` for exactly this, so the code
// is the promised one rather than the module's generic `invalid`. The message
// never echoes the caller's kind: it is unbounded input, and the field pointer
// already names what to fix (the rule TranscriptKindError states).
type KindFieldError struct {
	Field string
	// Only says which kind MAY carry the field, so the message can tell the
	// caller where the value belongs instead of only where it does not.
	Only string
}

func (e *KindFieldError) Error() string {
	return "only " + e.Only + " may carry " + e.Field
}

// FieldFault names the field that was wrong for the kind it arrived with.
func (e *KindFieldError) FieldFault() (field, code, message string) {
	return e.Field, faultNotValidForKind, e.Error()
}

// mailParticipants is the normalized form of the address headers an importer
// supplied: lowercased, trimmed, empties dropped, and no address in two roles.
type mailParticipants struct {
	From string
	To   []string
	Cc   []string
}

// normalizeParticipants folds every supplied address and gives each one a
// single role.
//
// First role wins: an address in both To and Cc is a recipient, and carrying it
// twice would let one address count as two on a message. From outranks both,
// because the sender is who the message is FROM whatever else the headers also
// list them as.
func normalizeParticipants(in *struct {
	Cc   *[]string `json:"cc,omitempty"`
	From *string   `json:"from,omitempty"`
	To   *[]string `json:"to,omitempty"`
},
) mailParticipants {
	var out mailParticipants
	seen := map[string]bool{}
	take := func(raw string) (string, bool) {
		addr := normalizeAddress(raw)
		if addr == "" || seen[addr] {
			return "", false
		}
		seen[addr] = true
		return addr, true
	}
	if in.From != nil {
		if addr, ok := take(*in.From); ok {
			out.From = addr
		}
	}
	if in.To != nil {
		for _, raw := range *in.To {
			if addr, ok := take(raw); ok {
				out.To = append(out.To, addr)
			}
		}
	}
	if in.Cc != nil {
		for _, raw := range *in.Cc {
			if addr, ok := take(raw); ok {
				out.Cc = append(out.Cc, addr)
			}
		}
	}
	return out
}

// importedCounterparty answers who a message was with, from its direction and
// its addresses.
//
// This mirrors capture's derivation (mailmap.go): an inbound message is with
// its sender; an outbound one is with the first recipient who is not the
// sending side. The non-owner rule is the part that matters and the part that
// is easy to drop — a sender who copies themselves is on their own To line,
// and taking the first recipient blindly would record the workspace as its own
// counterparty, which reads as correspondence with nobody.
//
// primaryCounterparty (email.go) answers the same question for a message this
// installation SENDS, where the addressee list never contains the sender. An
// imported message carries the headers as they were, so the sender can appear
// among its own recipients and the owner check has to be made rather than
// assumed.
//
// `owned` reports whether an address belongs to the sending side. The API has
// no mailbox to ask, so the caller supplies the acting principal's own
// addresses; when it cannot name any, every address is foreign and the first
// recipient stands — the answer capture reaches for a mailbox whose owner
// identity it has not learned yet.
func importedCounterparty(direction string, parts mailParticipants, owned func(string) bool) string {
	if direction == directionInbound {
		return parts.From
	}
	for _, addr := range parts.To {
		if !owned(addr) {
			return addr
		}
	}
	return ""
}

// trimMessageID strips the angle brackets an RFC 5322 Message-ID wears on the
// wire.
//
// The header form is <id@host>; the identity is the part inside. Capture
// trims the same way (mailmap.go), so an import that kept its brackets and a
// capture that shed them must not become two identities for one message.
func trimMessageID(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), "<>")
}

// mailIdentityFrom reads the identity and participant fields an importer sent,
// refusing any that the kind cannot carry.
//
// Every refusal here fires before the insert, so a caller learns which field to
// fix rather than having the value silently ignored — the defect this whole
// path exists to close was a 201 that dropped what it was given.
//
// The counterparty is NOT derived here. Deciding which recipient is the other
// side needs the sending side's own addresses, and those are a read against the
// database that this mapping — pure, shared by four doors, and called before
// any transaction opens — cannot make. The store derives it under the write
// (deriveImportedCounterparty), where the transaction exists.
func mailIdentityFrom(req crmcontracts.CreateActivityRequest, in *LogActivityInput) error {
	isEmail := in.Kind == string(crmcontracts.ActivityKindEmail)
	isMeeting := in.Kind == KindMeeting
	for _, refusal := range []struct {
		present bool
		allowed bool
		field   string
		only    string
	}{
		{req.Participants != nil, isEmail, fieldParticipants, onlyAnEmail},
		{req.RfcMessageId != nil, isEmail, fieldRFCMessageID, onlyAnEmail},
		{req.ThreadKey != nil, isEmail, fieldThreadKey, onlyAnEmail},
		{req.IcalUid != nil, isMeeting, fieldICalUID, onlyAMeeting},
		{req.IcalInstance != nil, isMeeting, fieldICalInstance, onlyAMeeting},
	} {
		if refusal.present && !refusal.allowed {
			return &KindFieldError{Field: refusal.field, Only: refusal.only}
		}
	}
	if err := mailParticipantsInto(req, in); err != nil {
		return err
	}
	if req.RfcMessageId != nil {
		in.RFCMessageID = trimMessageID(*req.RfcMessageId)
	}
	if req.ThreadKey != nil {
		in.ThreadKey = strings.TrimSpace(*req.ThreadKey)
	}
	// A message with no stated conversation starts one under its own id, which
	// is what capture does for a mail that begins a thread.
	if in.ThreadKey == "" {
		in.ThreadKey = in.RFCMessageID
	}
	return meetingIdentityInto(req, in)
}

// mailParticipantsInto takes the supplied addresses, and refuses the shapes
// that cannot yield a counterparty.
func mailParticipantsInto(req crmcontracts.CreateActivityRequest, in *LogActivityInput) error {
	if req.Participants == nil {
		return nil
	}
	// Without a direction the same addresses mean two different things — the
	// sender is the counterparty on an inbound message and our own side on an
	// outbound one — so guessing would file half the imported mail against the
	// wrong correspondent.
	if in.Direction == nil || *in.Direction == "" {
		return &RequiredFieldError{Field: fieldDirection}
	}
	parts := normalizeParticipants(req.Participants)
	if *in.Direction == directionInbound && parts.From == "" {
		return &RequiredFieldError{Field: fieldParticipantOf}
	}
	in.EmailFrom = parts.From
	in.EmailTo = parts.To
	in.EmailCc = parts.Cc
	return nil
}

// meetingIdentityInto takes the calendar identity, which is a pair.
//
// A recurring series shares ONE iCal UID across every occurrence — Google
// expands recurrences into separate events that all carry it — so a UID alone
// names the series, not the meeting. Accepting it alone would let every
// occurrence of a weekly call resolve to the same identity.
func meetingIdentityInto(req crmcontracts.CreateActivityRequest, in *LogActivityInput) error {
	if req.IcalUid == nil {
		return nil
	}
	uid := strings.TrimSpace(*req.IcalUid)
	if uid == "" {
		return nil
	}
	var instance string
	if req.IcalInstance != nil {
		instance = strings.TrimSpace(*req.IcalInstance)
	}
	if instance == "" {
		return &RequiredFieldError{Field: fieldICalInstance}
	}
	in.ICalUID = uid
	in.ICalInstance = instance
	return nil
}
