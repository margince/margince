// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// emailRequest is the smallest create request that will reach the mail
// identity rules: the kind that carries them, and the source every create
// requires.
func emailRequest() crmcontracts.CreateActivityRequest {
	return crmcontracts.CreateActivityRequest{
		Kind:   crmcontracts.CreateActivityRequestKindCreateActivityRequestKindEmail,
		Source: "hubspot_import:1",
	}
}

func participantsOf(from string, to, cc []string) *struct {
	Cc   *[]string `json:"cc,omitempty"`
	From *string   `json:"from,omitempty"`
	To   *[]string `json:"to,omitempty"`
} {
	out := &struct {
		Cc   *[]string `json:"cc,omitempty"`
		From *string   `json:"from,omitempty"`
		To   *[]string `json:"to,omitempty"`
	}{}
	if from != "" {
		out.From = &from
	}
	if to != nil {
		out.To = &to
	}
	if cc != nil {
		out.Cc = &cc
	}
	return out
}

func directionOf(d string) *crmcontracts.CreateActivityRequestDirection {
	out := crmcontracts.CreateActivityRequestDirection(d)
	return &out
}

// faultFieldOf reports the field a refusal names, so a test asserts WHICH
// field was refused rather than merely that something was.
func faultFieldOf(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected a refusal, got none")
	}
	fault, ok := err.(interface {
		FieldFault() (field, code, message string)
	})
	if !ok {
		t.Fatalf("refusal %v does not state a field fault, so no surface can report which field to fix", err)
	}
	field, _, _ := fault.FieldFault()
	return field
}

func TestOnlyAnEmailCarriesTheHeadersOfOne(t *testing.T) {
	msgID := "abc@example.test"
	for _, tc := range []struct {
		name  string
		field string
		apply func(*crmcontracts.CreateActivityRequest)
	}{
		{"participants", fieldParticipants, func(r *crmcontracts.CreateActivityRequest) {
			r.Participants = participantsOf("a@example.test", nil, nil)
		}},
		{"message id", fieldRFCMessageID, func(r *crmcontracts.CreateActivityRequest) {
			r.RfcMessageId = &msgID
		}},
		{"thread key", fieldThreadKey, func(r *crmcontracts.CreateActivityRequest) {
			r.ThreadKey = &msgID
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Every non-email kind, not just `note`. The identity table keys on
			// (identity_kind, identity_key) and the mail kind is ONE namespace
			// across every activity kind, so a task or a meeting permitted to
			// state a Message-ID could take the identity of a real message just
			// as a note could. A guard that covered one kind would be a guard
			// with three ways round it.
			for _, kind := range []crmcontracts.CreateActivityRequestKind{
				crmcontracts.CreateActivityRequestKindCreateActivityRequestKindNote,
				crmcontracts.CreateActivityRequestKindCreateActivityRequestKindCall,
				crmcontracts.CreateActivityRequestKindCreateActivityRequestKindTask,
				crmcontracts.CreateActivityRequestKindCreateActivityRequestKindMeeting,
			} {
				req := emailRequest()
				req.Kind = kind
				tc.apply(&req)
				if got := faultFieldOf(t, mustRefuse(t, req)); got != tc.field {
					t.Fatalf("[%s] refused %q, want the field the caller sent: %q", kind, got, tc.field)
				}
			}
		})
	}
}

func TestOnlyAMeetingCarriesACalendarIdentity(t *testing.T) {
	uid := "series-1@example.test"
	req := emailRequest()
	req.IcalUid = &uid
	if got := faultFieldOf(t, mustRefuse(t, req)); got != fieldICalUID {
		t.Fatalf("refused %q, want %q", got, fieldICalUID)
	}
}

// A UID names a recurring SERIES — every occurrence of a weekly call carries
// the same one. Accepting it alone would give every occurrence one identity.
func TestACalendarIdentityNamesTheOccurrenceNotJustTheSeries(t *testing.T) {
	uid := "series-1@example.test"
	req := emailRequest()
	req.Kind = crmcontracts.CreateActivityRequestKindCreateActivityRequestKindMeeting
	req.IcalUid = &uid
	if got := faultFieldOf(t, mustRefuse(t, req)); got != fieldICalInstance {
		t.Fatalf("refused %q, want %q", got, fieldICalInstance)
	}

	instance := "2026-09-16T10:00:00Z"
	req.IcalInstance = &instance
	in, err := LogActivityInputFrom(req)
	if err != nil {
		t.Fatalf("a meeting naming its occurrence is well-formed: %v", err)
	}
	if in.ICalUID != uid || in.ICalInstance != instance {
		t.Fatalf("calendar identity = (%q, %q), want (%q, %q)", in.ICalUID, in.ICalInstance, uid, instance)
	}
}

// The addresses mean opposite things in the two directions, so a message that
// does not say which it is cannot be filed against anybody.
func TestParticipantsWithoutADirectionAreRefused(t *testing.T) {
	req := emailRequest()
	req.Participants = participantsOf("them@example.test", []string{"us@example.test"}, nil)
	if got := faultFieldOf(t, mustRefuse(t, req)); got != fieldDirection {
		t.Fatalf("refused %q, want %q", got, fieldDirection)
	}
}

func TestAnInboundEmailNamesItsSender(t *testing.T) {
	req := emailRequest()
	req.Direction = directionOf("inbound")
	req.Participants = participantsOf("", []string{"us@example.test"}, nil)
	if got := faultFieldOf(t, mustRefuse(t, req)); got != fieldParticipantOf {
		t.Fatalf("refused %q, want %q", got, fieldParticipantOf)
	}
}

func TestAddressesArriveFoldedAndInOneRoleEach(t *testing.T) {
	req := emailRequest()
	req.Direction = directionOf("inbound")
	req.Participants = participantsOf(
		"  Them@Example.TEST ",
		[]string{"Us@example.test", "", "them@example.test"},
		[]string{"US@EXAMPLE.TEST", "other@example.test"},
	)
	in, err := LogActivityInputFrom(req)
	if err != nil {
		t.Fatalf("well-formed headers were refused: %v", err)
	}
	if in.EmailFrom != "them@example.test" {
		t.Fatalf("from = %q, want it folded to them@example.test", in.EmailFrom)
	}
	// them@ is already the sender and us@ is already a recipient, so neither
	// may appear a second time: one address on a message is one participant.
	if len(in.EmailTo) != 1 || in.EmailTo[0] != "us@example.test" {
		t.Fatalf("to = %v, want only us@example.test", in.EmailTo)
	}
	if len(in.EmailCc) != 1 || in.EmailCc[0] != "other@example.test" {
		t.Fatalf("cc = %v, want only other@example.test", in.EmailCc)
	}
}

func TestAMessageIDArrivesWithoutItsAngleBrackets(t *testing.T) {
	raw := " <abc@example.test> "
	req := emailRequest()
	req.RfcMessageId = &raw
	in, err := LogActivityInputFrom(req)
	if err != nil {
		t.Fatalf("a bracketed Message-ID is the wire form, not an error: %v", err)
	}
	if in.RFCMessageID != "abc@example.test" {
		t.Fatalf("message id = %q, want the identity inside the brackets", in.RFCMessageID)
	}
}

// A message that states no conversation starts one under its own id, which is
// what capture does for a mail that begins a thread.
func TestAThreadKeyFallsBackToTheMessageID(t *testing.T) {
	msgID := "abc@example.test"
	req := emailRequest()
	req.RfcMessageId = &msgID
	in, err := LogActivityInputFrom(req)
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}
	if in.ThreadKey != msgID {
		t.Fatalf("thread key = %q, want it to fall back to %q", in.ThreadKey, msgID)
	}

	stated := "root@example.test"
	req.ThreadKey = &stated
	in, err = LogActivityInputFrom(req)
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}
	if in.ThreadKey != stated {
		t.Fatalf("thread key = %q, want the stated conversation %q", in.ThreadKey, stated)
	}
}

func TestOnlyATimedKindStatesHowLongItTook(t *testing.T) {
	seconds := 600
	req := emailRequest()
	req.Kind = crmcontracts.CreateActivityRequestKindCreateActivityRequestKindNote
	req.DurationSeconds = &seconds
	if got := faultFieldOf(t, mustRefuse(t, req)); got != "duration_seconds" {
		t.Fatalf("refused %q, want duration_seconds", got)
	}

	req.Kind = crmcontracts.CreateActivityRequestKindCreateActivityRequestKindCall
	in, err := LogActivityInputFrom(req)
	if err != nil {
		t.Fatalf("a call may state its duration: %v", err)
	}
	if in.DurationSeconds == nil || *in.DurationSeconds != seconds {
		t.Fatalf("duration = %v, want %d carried to the store", in.DurationSeconds, seconds)
	}
}

// raw was accepted by the contract and dropped by the mapping, which is the
// defect this path closes: a 201 that silently discards what it was given.
func TestRawReachesTheStore(t *testing.T) {
	req := emailRequest()
	raw := map[string]any{"id": "145347700", "subject": "as the source system kept it"}
	req.Raw = &raw
	in, err := LogActivityInputFrom(req)
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}
	if in.Raw == nil {
		t.Fatal("raw was accepted on the wire and dropped before the store, which is the defect")
	}
	if (*in.Raw)["id"] != "145347700" {
		t.Fatalf("raw = %v, want the caller's own object", *in.Raw)
	}
}

// The counterparty is NOT derived in the mapping: it needs the acting side's
// own address, which is a read. The store derives it under the write.
func TestTheMappingLeavesTheCounterpartyToTheWrite(t *testing.T) {
	req := emailRequest()
	req.Direction = directionOf("outbound")
	req.Participants = participantsOf("us@example.test", []string{"them@example.test"}, nil)
	in, err := LogActivityInputFrom(req)
	if err != nil {
		t.Fatalf("unexpected refusal: %v", err)
	}
	if in.CounterpartyEmail != "" {
		t.Fatalf("counterparty = %q, want it left for the write that can ask who we are", in.CounterpartyEmail)
	}
}

func mustRefuse(t *testing.T, req crmcontracts.CreateActivityRequest) error {
	t.Helper()
	if _, err := LogActivityInputFrom(req); err != nil {
		return err
	}
	t.Fatal("expected a refusal, got none")
	return nil
}
