// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package leaddraft_test

// What a lead gives the drafter, and what it honestly cannot.

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/leaddraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func lead(over func(*crmcontracts.Lead)) crmcontracts.Lead {
	name := "Dung Ly Nguyen"
	company := "Newsky VN"
	email := openapi_types.Email("dung.ly@newsky.example")
	out := crmcontracts.Lead{
		Id:          openapi_types.UUID(ids.MustParse("019fe7ae-0000-7000-8000-000000000001")),
		FullName:    &name,
		CompanyName: &company,
		Email:       &email,
	}
	if over != nil {
		over(&out)
	}
	return out
}

func act(id string, at time.Time, direction crmcontracts.ActivityDirection, subject, body string) crmcontracts.Activity {
	return crmcontracts.Activity{
		Id:         openapi_types.UUID(ids.MustParse(id)),
		Kind:       crmcontracts.ActivityKindEmail,
		OccurredAt: at,
		Direction:  &direction,
		Subject:    &subject,
		Body:       &body,
	}
}

var when = time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)

// The lead's own columns reach the recipient the writer addresses. A lead has
// one name column, so the greeting name and the formal name both come off it.
func TestTheLeadIsTheRecipient(t *testing.T) {
	t.Parallel()
	in := leaddraft.FromLead(lead(nil), nil, "", draftfloor.Envelope{})

	if in.Recipient.Name != "Dung Ly Nguyen" {
		t.Errorf("name = %q, want the lead's own", in.Recipient.Name)
	}
	if in.Recipient.FirstName != "Dung" {
		t.Errorf("first name = %q, want the greeting name", in.Recipient.FirstName)
	}
	if in.Recipient.LastName != "Nguyen" {
		t.Errorf("last name = %q, want the formal name", in.Recipient.LastName)
	}
	if in.Recipient.Email != "dung.ly@newsky.example" {
		t.Errorf("email = %q, want the lead's address", in.Recipient.Email)
	}
	if in.Recipient.Employer != "Newsky VN" {
		t.Errorf("employer = %q, want the company the lead wrote", in.Recipient.Employer)
	}
}

// A one-word name is a name. It greets, and it produces no surname rather than
// a formal opening addressed to half of one.
func TestAOneWordNameGreetsAndHasNoSurname(t *testing.T) {
	t.Parallel()
	in := leaddraft.FromLead(lead(func(l *crmcontracts.Lead) {
		only := "Dung"
		l.FullName = &only
	}), nil, "", draftfloor.Envelope{})

	if in.Recipient.FirstName != "Dung" {
		t.Errorf("first name = %q, want the one word there is", in.Recipient.FirstName)
	}
	if in.Recipient.LastName != "" {
		t.Errorf("last name = %q, want none", in.Recipient.LastName)
	}
}

// What a lead cannot ground, stated as a test rather than left to a comment.
//
// A deal, a project and a claim all hang off a contact, and a lead is the
// record that exists before one does. The fold must leave them empty rather
// than reaching for a nearby account's — a draft naming a number nobody agreed
// with this prospect is worse than one that says less.
func TestALeadGroundsNoDealProjectOrClaim(t *testing.T) {
	t.Parallel()
	in := leaddraft.FromLead(lead(nil),
		[]crmcontracts.Activity{
			act("019fe7ae-0000-7000-8000-00000000000a", when.Add(-2*time.Hour),
				crmcontracts.ActivityDirectionInbound, "Pricing?", "What would this cost for 40 seats?"),
		}, "", draftfloor.Envelope{})

	if in.Deal != nil {
		t.Error("a lead folded a deal; a lead has none")
	}
	if in.Project != nil {
		t.Error("a lead folded a project; a lead has none")
	}
	if len(in.Claims) != 0 {
		t.Errorf("a lead folded %d claims; a lead has none", len(in.Claims))
	}
	if in.Meeting != nil {
		t.Error("a lead folded a meeting; a lead's meetings are not read here")
	}
}

// The correspondence reaches the draft through persondraft's own fold, so the
// window and the snippet rule are the ones a contact's draft uses.
func TestTheConversationReachesTheDraft(t *testing.T) {
	t.Parallel()
	in := leaddraft.FromLead(lead(nil),
		[]crmcontracts.Activity{
			act("019fe7ae-0000-7000-8000-00000000000a", when.Add(-2*time.Hour),
				crmcontracts.ActivityDirectionInbound, "Pricing?", "What would this cost for 40 seats?"),
			act("019fe7ae-0000-7000-8000-00000000000b", when.Add(-48*time.Hour),
				crmcontracts.ActivityDirectionOutbound, "Intro", "Good to meet you at the fair."),
		}, "", draftfloor.Envelope{})

	if len(in.Recent) != 2 {
		t.Fatalf("recent = %d exchanges, want both", len(in.Recent))
	}
	if !in.Recent[0].Inbound {
		t.Error("the newest exchange lost its direction")
	}
	// The newest INBOUND message yields its opening, which is what the draft
	// answers. A subject line says a message happened; the words say what about.
	if in.Recent[0].Snippet == "" {
		t.Error("the newest inbound message carried no snippet")
	}
	if in.Recent[1].Snippet != "" {
		t.Error("an outbound message carried a snippet; only the newest inbound does")
	}
}

// Which direction went last is the whole question a follow-up answers, and a
// lead has no column that records it — only one `last_activity_at`. Both stamps
// are derived from the correspondence, newest of each.
func TestEachSidesLastMessageIsDerived(t *testing.T) {
	t.Parallel()
	in := leaddraft.FromLead(lead(nil),
		[]crmcontracts.Activity{
			act("019fe7ae-0000-7000-8000-00000000000a", when.Add(-2*time.Hour),
				crmcontracts.ActivityDirectionInbound, "Pricing?", "What would this cost?"),
			act("019fe7ae-0000-7000-8000-00000000000b", when.Add(-48*time.Hour),
				crmcontracts.ActivityDirectionOutbound, "Intro", "Good to meet you."),
		}, "", draftfloor.Envelope{})

	wantIn := when.Add(-2 * time.Hour).Format(time.RFC3339)
	wantOut := when.Add(-48 * time.Hour).Format(time.RFC3339)
	if in.Recipient.LastInbound != wantIn {
		t.Errorf("last inbound = %q, want %q", in.Recipient.LastInbound, wantIn)
	}
	if in.Recipient.LastOutbound != wantOut {
		t.Errorf("last outbound = %q, want %q", in.Recipient.LastOutbound, wantOut)
	}
}

// A direction that never happened is empty rather than the other one's stamp.
// Collapsing the two would tell a follow-up that we had written when nobody had.
func TestADirectionThatNeverHappenedIsEmpty(t *testing.T) {
	t.Parallel()
	in := leaddraft.FromLead(lead(nil),
		[]crmcontracts.Activity{
			act("019fe7ae-0000-7000-8000-00000000000a", when.Add(-2*time.Hour),
				crmcontracts.ActivityDirectionInbound, "Pricing?", "What would this cost?"),
		}, "", draftfloor.Envelope{})

	if in.Recipient.LastOutbound != "" {
		t.Errorf("last outbound = %q on a lead nobody has written to", in.Recipient.LastOutbound)
	}
}

// The envelope's account of the conversation is derived from the same two
// instants the recipient's stamps are, so a change to one moves the other.
//
// Mutation: point ConversationState at its own walk of the activities and this
// still passes while the two drift on any record where they differ — which is
// why it asserts against convstate.Classify of lastEachWay's OWN answer rather
// than against a state written down here.
func TestTheConversationStateReadsTheSameTwoInstants(t *testing.T) {
	t.Parallel()
	activities := []crmcontracts.Activity{
		act("019fe7ae-0000-7000-8000-00000000000a", when.Add(-2*time.Hour),
			crmcontracts.ActivityDirectionInbound, "Pricing?", "What would this cost?"),
	}
	got := leaddraft.ConversationState(activities, when)
	want := convstate.Classify(when, when.Add(-2*time.Hour), time.Time{})
	if got != want {
		t.Errorf("state = %v, want %v — the envelope and the recipient read different instants", got, want)
	}
}

// A lead with no correspondence at all is a first touch, not a failure.
func TestALeadNobodyHasWrittenToIsAFirstTouch(t *testing.T) {
	t.Parallel()
	if got := leaddraft.ConversationState(nil, when); got != convstate.Classify(when, time.Time{}, time.Time{}) {
		t.Errorf("state = %v on a lead with no history", got)
	}
}
