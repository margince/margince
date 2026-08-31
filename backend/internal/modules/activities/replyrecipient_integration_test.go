// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// Naming who a reply goes TO, over a real migrated Postgres.
//
// The composer cannot send without a recipient — `to` is required on
// send-email — so an unresolved address is not a cosmetic gap: it is a reply
// the reader must address by hand against a record that already holds the
// answer. These cases pin the resolution the draft response carries.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedPersonEmail gives a person one address, the way capture records one.
func (e *sendEnv) seedPersonEmail(t *testing.T, person ids.UUID, email string, primary bool, position int) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO person_email (person_id, email, is_primary, position, source, captured_by)
		VALUES ($1, $2, $3, $4, 'manual', 'human:x')`,
		person, email, primary, position); err != nil {
		t.Fatalf("seeding the person address: %v", err)
	}
}

// seedFirstName fills the split name capture records alongside the full one.
func (e *sendEnv) seedFirstName(t *testing.T, person ids.UUID, first string) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE person SET first_name = $2 WHERE id = $1`, person, first); err != nil {
		t.Fatalf("seeding the first name: %v", err)
	}
}

// participate stamps one participant role on the anchor.
func (e *sendEnv) participate(t *testing.T, anchor ids.ActivityID, person ids.UUID, role string) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity_participant (activity_id, person_id, role)
		VALUES ($1, $2, $3)`, anchor, person, role); err != nil {
		t.Fatalf("stamping the %s participant: %v", role, err)
	}
}

func TestAReplyIsAddressedToTheSenderOfTheMessageItAnswers(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	// The copied colleague is stamped FIRST, so insertion order favours the
	// wrong person: only the role ranking can put the sender ahead of them.
	// Seeded the other way round, this passes with no ranking at all.
	copied := e.linkPerson(t, anchor, "Anne Wiegert")
	e.seedPersonEmail(t, copied, "anne@buyer.test", true, 0)
	e.participate(t, anchor, copied, "cc")

	sender := e.linkPerson(t, anchor, "Dietmar Rietsch")
	e.seedPersonEmail(t, sender, "dietmar@buyer.test", true, 0)
	e.participate(t, anchor, sender, "from")

	got, err := e.store(nil).ReplyRecipientFor(e.as(principal.RowScopeAll), anchor)
	if err != nil {
		t.Fatalf("ReplyRecipientFor: %v", err)
	}
	if got.Address != "dietmar@buyer.test" {
		t.Errorf("address = %q, want the sender's dietmar@buyer.test — a reply addressed to a copied colleague answers the wrong person", got.Address)
	}
	if got.FullName != "Dietmar Rietsch" {
		t.Errorf("full name = %q, want Dietmar Rietsch", got.FullName)
	}
}

func TestTheAddressAndTheGreetedNameAreOnePerson(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	// Copied colleague first, so the ranking rather than insertion order has
	// to be what picks the pair the assertion reads.
	copied := e.linkPerson(t, anchor, "Anne Wiegert")
	e.seedFirstName(t, copied, "Anne")
	e.seedPersonEmail(t, copied, "anne@buyer.test", true, 0)
	e.participate(t, anchor, copied, "cc")

	sender := e.linkPerson(t, anchor, "Dietmar Rietsch")
	e.seedFirstName(t, sender, "Dietmar")
	e.seedPersonEmail(t, sender, "dietmar@buyer.test", true, 0)
	e.participate(t, anchor, sender, "from")

	got, err := e.store(nil).ReplyRecipientFor(e.as(principal.RowScopeAll), anchor)
	if err != nil {
		t.Fatalf("ReplyRecipientFor: %v", err)
	}
	// The failure this guards is a draft greeting one person by name and
	// addressed to another's mailbox — which reads as correct on screen and
	// is wrong in the reader's sent folder.
	if got.FirstName != "Dietmar" || got.Address != "dietmar@buyer.test" {
		t.Errorf("greeted %q but addressed %q — the name and the address must be one person",
			got.FirstName, got.Address)
	}
}

func TestAPersonWithSeveralAddressesIsAnsweredAtTheirPrimary(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	sender := e.linkPerson(t, anchor, "Dietmar Rietsch")
	// Written non-primary first, so a resolution that took whatever the
	// table handed back would pick the wrong one.
	e.seedPersonEmail(t, sender, "old@buyer.test", false, 0)
	e.seedPersonEmail(t, sender, "dietmar@buyer.test", true, 1)
	e.participate(t, anchor, sender, "from")

	got, err := e.store(nil).ReplyRecipientFor(e.as(principal.RowScopeAll), anchor)
	if err != nil {
		t.Fatalf("ReplyRecipientFor: %v", err)
	}
	if got.Address != "dietmar@buyer.test" {
		t.Errorf("address = %q, want the primary dietmar@buyer.test", got.Address)
	}
}

func TestAnArchivedAddressIsNotOfferedAsTheRecipient(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	sender := e.linkPerson(t, anchor, "Dietmar Rietsch")
	e.seedPersonEmail(t, sender, "gone@buyer.test", true, 0)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE person_email SET archived_at = now() WHERE person_id = $1`, sender); err != nil {
		t.Fatalf("archiving the address: %v", err)
	}
	e.participate(t, anchor, sender, "from")

	got, err := e.store(nil).ReplyRecipientFor(e.as(principal.RowScopeAll), anchor)
	if err != nil {
		t.Fatalf("ReplyRecipientFor: %v", err)
	}
	// Empty is the answer here, not the archived address: the reader fills
	// one in. Offering a retired mailbox sends the reply nowhere and reads
	// as though the system knew where it was going.
	if got.Address != "" {
		t.Errorf("address = %q, want empty — an archived address is not somewhere to write", got.Address)
	}
	if got.FullName != "Dietmar Rietsch" {
		t.Errorf("full name = %q, want the name to survive an unusable address", got.FullName)
	}
}

func TestAnErasedPersonYieldsNeitherNameNorAddress(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	person := e.linkPerson(t, anchor, "Dietmar Rietsch")
	e.seedPersonEmail(t, person, "dietmar@buyer.test", true, 0)
	e.participate(t, anchor, person, "from")
	// Art. 17 erasure archives the person in place, leaving the activity and
	// its participant row behind. The reply must not go on naming or writing
	// to them.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE person SET archived_at = now() WHERE id = $1`, person); err != nil {
		t.Fatalf("archiving the person: %v", err)
	}

	got, err := e.store(nil).ReplyRecipientFor(e.as(principal.RowScopeAll), anchor)
	if err != nil {
		t.Fatalf("ReplyRecipientFor: %v", err)
	}
	if got.Address != "" || got.FullName != "" {
		t.Errorf("got name %q address %q, want both empty for an erased person",
			got.FullName, got.Address)
	}
}

func TestDraftingRefusesACallerWithoutThePersonReadGrant(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")
	sender := e.linkPerson(t, anchor, "Dietmar Rietsch")
	e.seedPersonEmail(t, sender, "dietmar@buyer.test", true, 0)
	e.participate(t, anchor, sender, "from")

	// A seat that may read the conversation and not the people on it. The
	// address must not reach them through the drafting door that the people
	// surface closes.
	ctx := principal.WithCorrelationID(
		principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})

	if _, err := e.store(nil).ReplyRecipientFor(ctx, anchor); err == nil {
		t.Error("ReplyRecipientFor returned a recipient to a caller with no person read grant")
	}
}

// TestTheRecipientEndpointAndTheDraftNameOneAddress holds the seam this
// endpoint exists to keep honest. The composer shows a recipient when it
// opens and a draft fills one in tens of seconds later; if those two ever
// resolved differently, the reader would watch the address they were shown
// be replaced by another, with nothing saying which is the one that sends.
func TestTheRecipientEndpointAndTheDraftNameOneAddress(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	copied := e.linkPerson(t, anchor, "Anne Wiegert")
	e.seedPersonEmail(t, copied, "anne@buyer.test", true, 0)
	e.participate(t, anchor, copied, "cc")
	sender := e.linkPerson(t, anchor, "Dietmar Rietsch")
	e.seedPersonEmail(t, sender, "dietmar@buyer.test", true, 0)
	e.participate(t, anchor, sender, "from")

	handlers := Handlers{store: NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))}
	ctx := e.as(principal.RowScopeAll)

	shown := httptest.NewRecorder()
	handlers.GetReplyRecipient(shown,
		httptest.NewRequest(http.MethodGet, "/v1/activities/x/reply-recipient", nil).WithContext(ctx),
		crmcontracts.Id(anchor.UUID))
	if shown.Code != http.StatusOK {
		t.Fatalf("reply-recipient answered %d, want 200", shown.Code)
	}
	var recipient crmcontracts.ReplyRecipient
	if err := json.Unmarshal(shown.Body.Bytes(), &recipient); err != nil {
		t.Fatalf("decoding the recipient: %v", err)
	}

	// The drafter is nil here, so the draft runs the deterministic floor —
	// which is the point: the address must not depend on a model answering.
	drafted := httptest.NewRecorder()
	handlers.DraftEmail(drafted,
		httptest.NewRequest(http.MethodPost, "/v1/activities/x/draft-email", nil).WithContext(ctx),
		crmcontracts.Id(anchor.UUID))
	if drafted.Code != http.StatusOK {
		t.Fatalf("draft-email answered %d, want 200", drafted.Code)
	}
	var draft crmcontracts.EmailDraft
	if err := json.Unmarshal(drafted.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decoding the draft: %v", err)
	}

	if draft.To == nil || len(*draft.To) == 0 {
		t.Fatal("the draft carried no recipient — a reply the reader must address by hand")
	}
	if got := string((*draft.To)[0]); got != recipient.Address {
		t.Errorf("shown %q but drafted %q — one thread must have one recipient",
			recipient.Address, got)
	}
	if recipient.Address != "dietmar@buyer.test" {
		t.Errorf("both named %q, want the sender dietmar@buyer.test", recipient.Address)
	}
}

func TestAnOwnerPrivateContactIsNotAddressableByAColleague(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	// Captured privately by somebody else. Capture privacy binds even an
	// unbounded seat, so this is not a row-scope case: neither the address
	// nor the name may reach a colleague through the drafting door.
	person := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO person (id, full_name, owner_id, visibility, source, captured_by)
		 VALUES ($1, 'Dietmar Rietsch', $2, 'owner', 'manual', 'human:x')`,
		person, e.other); err != nil {
		t.Fatalf("seeding the owner-private person: %v", err)
	}
	e.seedPersonEmail(t, person, "dietmar@buyer.test", true, 0)
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity_link (activity_id, entity_type, person_id)
		 VALUES ($1, 'person', $2)`, anchor, person); err != nil {
		t.Fatalf("linking the private person: %v", err)
	}
	e.participate(t, anchor, person, "from")

	// RowScopeAll: the widest seat there is, so admitting here would mean the
	// address is reachable by everyone rather than by its owner.
	//
	// The refusal lands on the ACTIVITY rather than the person: a conversation
	// whose only linked record is privately captured is not reachable at all,
	// so the caller is told the message does not exist rather than that its
	// counterparty is withheld. Both halves are the same protection, and this
	// asserts the outer one because it is the one that fires.
	_, err := e.store(nil).ReplyRecipientFor(e.as(principal.RowScopeAll), anchor)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound — a privately captured contact must not be addressable, and existence must stay hidden", err)
	}
}

func TestAnOwnerPrivateContactStaysWithheldOnAReachableConversation(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	// A colleague on the thread makes the conversation reachable, so the
	// activity gate admits and the PERSON gate is what has to hold. Without
	// this case the capture-privacy arm is never exercised: the case above
	// passes on the activity refusal alone.
	colleague := e.linkPerson(t, anchor, "Anne Wiegert")
	e.seedPersonEmail(t, colleague, "anne@buyer.test", true, 0)
	e.participate(t, anchor, colleague, "cc")

	private := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO person (id, full_name, owner_id, visibility, source, captured_by)
		 VALUES ($1, 'Dietmar Rietsch', $2, 'owner', 'manual', 'human:x')`,
		private, e.other); err != nil {
		t.Fatalf("seeding the owner-private person: %v", err)
	}
	e.seedPersonEmail(t, private, "dietmar@buyer.test", true, 0)
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity_link (activity_id, entity_type, person_id)
		 VALUES ($1, 'person', $2)`, anchor, private); err != nil {
		t.Fatalf("linking the private person: %v", err)
	}
	// The private contact is the SENDER, so the role ranking wants them: only
	// the capture-privacy arm can keep their address off this reply.
	e.participate(t, anchor, private, "from")

	got, err := e.store(nil).ReplyRecipientFor(e.as(principal.RowScopeAll), anchor)
	if err != nil {
		t.Fatalf("ReplyRecipientFor: %v", err)
	}
	if got.Address == "dietmar@buyer.test" || got.FullName == "Dietmar Rietsch" {
		t.Fatalf("got name %q address %q — the privately captured sender leaked through the drafting door",
			got.FullName, got.Address)
	}
	// It degrades to the colleague it MAY name rather than to nothing: the
	// reply is still addressable, just not to the withheld contact.
	if got.Address != "anne@buyer.test" {
		t.Errorf("address = %q, want the readable colleague anne@buyer.test", got.Address)
	}
}
