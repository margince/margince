// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// Addressing a reply from the HTTP surface, over a real migrated Postgres.
//
// Sending REQUIRES `to`, so an unresolved address is not a cosmetic gap: it is
// a reply the reader must address by hand against a thread that already names
// the person. These cases drive the two handlers that answer it — the draft
// response and the recipient endpoint — because what they must agree on is WHO
// may be written to, and that is a different question from who a draft greets.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ourDomain answers the colleague question the way the own-domain store does
// for a workspace that has registered one domain.
type ourDomain struct{ suffix string }

func (d ourDomain) Covers(context.Context) (func(string) bool, error) {
	return func(address string) bool {
		return len(address) > len(d.suffix) &&
			address[len(address)-len(d.suffix):] == d.suffix
	}, nil
}

// seedPersonEmail gives a person their one primary address, the way capture
// records one. Which of SEVERAL addresses wins is ReplyAddressFor's own
// question and is covered where that resolver lives; what these cases vary is
// WHOSE address is offered.
func (e *sendEnv) seedPersonEmail(t *testing.T, person ids.UUID, email string) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO person_email (person_id, email, is_primary, position, source, captured_by)
		VALUES ($1, $2, true, 0, 'manual', 'human:x')`,
		person, email); err != nil {
		t.Fatalf("seeding the person address: %v", err)
	}
}

// participate stamps one participant role on the anchor. A participant that
// carries a user_id is one of OUR OWN people, which is the difference between
// a reply and a message to ourselves.
func (e *sendEnv) participate(t *testing.T, anchor ids.ActivityID, person ids.UUID, role string, seat *ids.UUID) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity_participant (activity_id, person_id, role, user_id)
		VALUES ($1, $2, $3, $4)`, anchor, person, role, seat); err != nil {
		t.Fatalf("stamping the %s participant: %v", role, err)
	}
}

// handlers is the module's HTTP surface with the colleague reader wired, which
// is how compose builds it. A nil reader is a different case with its own test.
func (e *sendEnv) handlers(colleagues ColleagueDomains) Handlers {
	h := Handlers{store: NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))}
	if colleagues != nil {
		h = h.WithColleagues(colleagues)
	}
	return h
}

// recipientOf drives GET /activities/{id}/reply-recipient and decodes it.
func recipientOf(ctx context.Context, t *testing.T, h Handlers, anchor ids.ActivityID) crmcontracts.ReplyRecipient {
	t.Helper()
	rec := httptest.NewRecorder()
	h.GetReplyRecipient(rec,
		httptest.NewRequest(http.MethodGet, "/v1/activities/x/reply-recipient", nil).WithContext(ctx),
		crmcontracts.Id(anchor.UUID))
	if rec.Code != http.StatusOK {
		t.Fatalf("reply-recipient answered %d, want 200", rec.Code)
	}
	var out crmcontracts.ReplyRecipient
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding the recipient: %v", err)
	}
	return out
}

// draftedTo drives POST /activities/{id}/draft-email and returns its `to`.
func draftedTo(ctx context.Context, t *testing.T, h Handlers, anchor ids.ActivityID) []string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.DraftEmail(rec,
		httptest.NewRequest(http.MethodPost, "/v1/activities/x/draft-email", nil).WithContext(ctx),
		crmcontracts.Id(anchor.UUID))
	if rec.Code != http.StatusOK {
		t.Fatalf("draft-email answered %d, want 200", rec.Code)
	}
	var draft crmcontracts.EmailDraft
	if err := json.Unmarshal(rec.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decoding the draft: %v", err)
	}
	if draft.To == nil {
		return nil
	}
	out := make([]string, 0, len(*draft.To))
	for _, address := range *draft.To {
		out = append(out, string(address))
	}
	return out
}

func TestAReplyIsAddressedToTheSenderOfTheMessageItAnswers(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	// The copied contact is stamped FIRST, so insertion order favours the
	// wrong person: only the role ranking can put the sender ahead of them.
	// Seeded the other way round, this passes with no ranking at all.
	copied := e.linkPerson(t, anchor, "Anne Wiegert")
	e.seedPersonEmail(t, copied, "anne@buyer.test")
	e.participate(t, anchor, copied, "cc", nil)

	sender := e.linkPerson(t, anchor, "Dietmar Rietsch")
	e.seedPersonEmail(t, sender, "dietmar@buyer.test")
	e.participate(t, anchor, sender, "from", nil)

	got := recipientOf(e.as(principal.RowScopeAll), t,
		e.handlers(ourDomain{suffix: "@demo.test"}), anchor)
	if got.Address != "dietmar@buyer.test" {
		t.Errorf("address = %q, want the sender's dietmar@buyer.test — a reply addressed to a copied contact answers the wrong person", got.Address)
	}
	if got.FullName != "Dietmar Rietsch" {
		t.Errorf("full name = %q, want Dietmar Rietsch", got.FullName)
	}
}

func TestOurOwnSenderIsNeverTheAddressOnOurOutboundMail(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	// OUR rep wrote it, and their address is on file as a contact — the shape
	// that turns a `from`-first ranking into a message to ourselves.
	//
	// Their address is deliberately OFF our own domain, so the colleague
	// predicate cannot catch them and only the seat exclusion can. Given them
	// an @demo.test address instead, this passes with the seat filter deleted
	// and proves nothing about it.
	us := e.linkPerson(t, anchor, "Sofia Meier")
	e.seedPersonEmail(t, us, "sofia.private@gmail.test")
	e.participate(t, anchor, us, "from", &e.rep)

	buyer := e.linkPerson(t, anchor, "Dietmar Rietsch")
	e.seedPersonEmail(t, buyer, "dietmar@buyer.test")
	e.participate(t, anchor, buyer, "to", nil)

	got := recipientOf(e.as(principal.RowScopeAll), t,
		e.handlers(ourDomain{suffix: "@demo.test"}), anchor)
	if got.Address == "sofia.private@gmail.test" {
		t.Fatal("the reply is addressed to our own sender — this composes a message to ourselves")
	}
	if got.Address != "dietmar@buyer.test" {
		t.Errorf("address = %q, want the counterparty dietmar@buyer.test", got.Address)
	}
	// The GREETING still names the sender, and that is correct rather than a
	// mismatch: the two fields answer different questions, and only the
	// address is constrained to a counterparty.
	if got.FullName != "Sofia Meier" {
		t.Errorf("full name = %q, want the greeted sender Sofia Meier", got.FullName)
	}
}

func TestAColleagueWithoutASeatIsNotAddressable(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	// A co-worker on our own domain who has NO login, so no user_id marks
	// them as ours. Only the domain predicate can tell.
	coworker := e.linkPerson(t, anchor, "Jonas Weber")
	e.seedPersonEmail(t, coworker, "jonas@demo.test")
	e.participate(t, anchor, coworker, "from", nil)

	got := recipientOf(e.as(principal.RowScopeAll), t,
		e.handlers(ourDomain{suffix: "@demo.test"}), anchor)
	if got.Address != "" {
		t.Errorf("address = %q, want empty — a thread with only colleagues on it has nobody outside the company to answer", got.Address)
	}
}

func TestAnUnwiredColleagueReaderOffersNoAddressRatherThanAnUnfilteredOne(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")
	coworker := e.linkPerson(t, anchor, "Jonas Weber")
	e.seedPersonEmail(t, coworker, "jonas@demo.test")
	e.participate(t, anchor, coworker, "from", nil)

	// Nil reader: a deployment that has not wired the own-domain store cannot
	// tell a colleague from a counterparty, so it offers nobody rather than
	// offering everybody.
	got := recipientOf(e.as(principal.RowScopeAll), t, e.handlers(nil), anchor)
	if got.Address != "" {
		t.Errorf("address = %q, want empty — an unwired colleague reader must fail closed", got.Address)
	}
}

func TestTheDraftCarriesTheAddressTheRecipientEndpointNames(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	us := e.linkPerson(t, anchor, "Sofia Meier")
	e.seedPersonEmail(t, us, "sofia.private@gmail.test")
	e.participate(t, anchor, us, "from", &e.rep)
	buyer := e.linkPerson(t, anchor, "Dietmar Rietsch")
	e.seedPersonEmail(t, buyer, "dietmar@buyer.test")
	e.participate(t, anchor, buyer, "to", nil)

	h := e.handlers(ourDomain{suffix: "@demo.test"})
	ctx := e.as(principal.RowScopeAll)

	// The composer shows one address on open and the draft fills another in
	// tens of seconds later; a reader watching the first be replaced by the
	// second cannot tell which one sends. The anchor is our OWN outbound, so
	// a surface that skipped the counterparty rule would answer our own rep
	// here rather than agreeing with the other by accident.
	shown := recipientOf(ctx, t, h, anchor).Address
	drafted := draftedTo(ctx, t, h, anchor)

	if len(drafted) != 1 {
		t.Fatalf("the draft carried %d recipients, want exactly one", len(drafted))
	}
	if drafted[0] != shown {
		t.Errorf("shown %q but drafted %q — one thread must have one recipient", shown, drafted[0])
	}
	if shown != "dietmar@buyer.test" {
		t.Errorf("both named %q, want the counterparty dietmar@buyer.test", shown)
	}
}

func TestAnArchivedAddressIsNotOfferedAsTheRecipient(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	sender := e.linkPerson(t, anchor, "Dietmar Rietsch")
	e.seedPersonEmail(t, sender, "gone@buyer.test")
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE person_email SET archived_at = now() WHERE person_id = $1`, sender); err != nil {
		t.Fatalf("archiving the address: %v", err)
	}
	e.participate(t, anchor, sender, "from", nil)

	got := recipientOf(e.as(principal.RowScopeAll), t,
		e.handlers(ourDomain{suffix: "@demo.test"}), anchor)

	// Empty is the answer here, not the archived address: the reader fills one
	// in. Offering a retired mailbox sends the reply nowhere and reads as
	// though the system knew where it was going.
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

	// A readable contact keeps the CONVERSATION reachable, so this exercises
	// the person filter rather than the activity gate refusing outright.
	readable := e.linkPerson(t, anchor, "Anne Wiegert")
	e.seedPersonEmail(t, readable, "anne@buyer.test")
	e.participate(t, anchor, readable, "cc", nil)

	erased := e.linkPerson(t, anchor, "Dietmar Rietsch")
	e.seedPersonEmail(t, erased, "dietmar@buyer.test")
	e.participate(t, anchor, erased, "from", nil)
	// Art. 17 erasure archives the person in place, leaving the activity and
	// its participant row behind. The reply must not go on naming or writing
	// to them.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE person SET archived_at = now() WHERE id = $1`, erased); err != nil {
		t.Fatalf("archiving the person: %v", err)
	}

	got := recipientOf(e.as(principal.RowScopeAll), t,
		e.handlers(ourDomain{suffix: "@demo.test"}), anchor)
	if got.Address == "dietmar@buyer.test" || got.FullName == "Dietmar Rietsch" {
		t.Fatalf("got name %q address %q — an erased person is still being answered",
			got.FullName, got.Address)
	}
	if got.Address != "anne@buyer.test" {
		t.Errorf("address = %q, want the readable anne@buyer.test", got.Address)
	}
}

func TestAPrivatelyCapturedContactStaysWithheldOnAReachableConversation(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	// A readable contact makes the CONVERSATION reachable, so the activity
	// gate admits and the person gate is what has to hold. Without them the
	// activity is unreachable outright and this would pass on the outer
	// refusal, never exercising capture privacy at all.
	readable := e.linkPerson(t, anchor, "Anne Wiegert")
	e.seedPersonEmail(t, readable, "anne@buyer.test")
	e.participate(t, anchor, readable, "cc", nil)

	private := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO person (id, full_name, owner_id, visibility, source, captured_by)
		 VALUES ($1, 'Dietmar Rietsch', $2, 'owner', 'manual', 'human:x')`,
		private, e.other); err != nil {
		t.Fatalf("seeding the owner-private person: %v", err)
	}
	e.seedPersonEmail(t, private, "dietmar@buyer.test")
	// The private contact is the SENDER, so the role ranking wants them: only
	// the capture-privacy arm can keep their address off this reply.
	e.participate(t, anchor, private, "from", nil)

	// RowScopeAll: the widest seat there is, so admitting here would mean the
	// address is reachable by everyone rather than by its owner.
	got := recipientOf(e.as(principal.RowScopeAll), t,
		e.handlers(ourDomain{suffix: "@demo.test"}), anchor)
	if got.Address == "dietmar@buyer.test" || got.FullName == "Dietmar Rietsch" {
		t.Fatalf("got name %q address %q — the privately captured sender leaked through the drafting door",
			got.FullName, got.Address)
	}
	// It degrades to the contact it MAY name rather than to nothing: the reply
	// is still addressable, just not to the withheld one.
	if got.Address != "anne@buyer.test" {
		t.Errorf("address = %q, want the readable anne@buyer.test", got.Address)
	}
}

func TestAddressingRefusesACallerWithoutThePersonReadGrant(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")
	sender := e.linkPerson(t, anchor, "Dietmar Rietsch")
	e.seedPersonEmail(t, sender, "dietmar@buyer.test")
	e.participate(t, anchor, sender, "from", nil)

	// A seat that may read the conversation and not the people on it. Neither
	// the name nor the address may reach them through the drafting door that
	// the people surface closes.
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

	rec := httptest.NewRecorder()
	e.handlers(ourDomain{suffix: "@demo.test"}).GetReplyRecipient(rec,
		httptest.NewRequest(http.MethodGet, "/v1/activities/x/reply-recipient", nil).WithContext(ctx),
		crmcontracts.Id(anchor.UUID))
	if rec.Code == http.StatusOK {
		t.Errorf("reply-recipient answered 200 to a caller with no person read grant: %s", rec.Body.String())
	}
}

func TestAWithheldContactIsStillAnsweredAtTheAddressTheyWroteFrom(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	// Privately captured by somebody else, so the person row is unreadable —
	// and the message they wrote carries the address they wrote FROM, which is
	// on the activity this caller already reached rather than on the person.
	private := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO person (id, full_name, owner_id, visibility, source, captured_by)
		 VALUES ($1, 'Dietmar Rietsch', $2, 'owner', 'manual', 'human:x')`,
		private, e.other); err != nil {
		t.Fatalf("seeding the owner-private person: %v", err)
	}
	e.seedPersonEmail(t, private, "dietmar@buyer.test")
	readable := e.linkPerson(t, anchor, "Anne Wiegert")
	e.seedPersonEmail(t, readable, "anne@buyer.test")
	e.participate(t, anchor, readable, "cc", nil)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity_participant (activity_id, person_id, role, address)
		VALUES ($1, $2, 'from', 'dietmar@buyer.test')`, anchor, private); err != nil {
		t.Fatalf("stamping the corresponding address: %v", err)
	}

	got := recipientOf(e.as(principal.RowScopeAll), t,
		e.handlers(ourDomain{suffix: "@demo.test"}), anchor)

	// The address is answered even though the person is not readable, and that
	// is the honest outcome rather than a leak: it is on the message this
	// caller can already open, and withholding it would refuse a reply to
	// correspondence they are looking at.
	if got.Address != "dietmar@buyer.test" {
		t.Errorf("address = %q, want the address on the message itself", got.Address)
	}
	// The NAME cannot come from the withheld person, so it falls through to
	// the readable contact — the two fields name two different people here.
	// Worth pinning: a reader who assumed they were one person would greet
	// the colleague and mail the sender.
	if got.FullName != "Anne Wiegert" {
		t.Errorf("full name = %q, want the readable Anne Wiegert — the withheld person cannot be named", got.FullName)
	}
	if got.FullName == "Dietmar Rietsch" {
		t.Error("the withheld person was named through the drafting door")
	}
}

func TestTheSurnameAFormalGreetingNeedsIsResolvedWithTheFirstName(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	sender := e.linkPerson(t, anchor, "Dietmar Rietsch")
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE person SET first_name = 'Dietmar', last_name = 'Rietsch' WHERE id = $1`,
		sender); err != nil {
		t.Fatalf("seeding the split name: %v", err)
	}
	e.seedPersonEmail(t, sender, "dietmar@buyer.test")
	e.participate(t, anchor, sender, "from", nil)

	got, err := e.store(nil).ReplyRecipientFor(e.as(principal.RowScopeAll), anchor)
	if err != nil {
		t.Fatalf("ReplyRecipientFor: %v", err)
	}
	// Both, from one read. A formal German greeting takes the surname and the
	// familiar one takes the given name; resolving only the first leaves a
	// model writing formal German to invent the half it was not given.
	if got.FirstName != "Dietmar" || got.LastName != "Rietsch" {
		t.Errorf("got first %q last %q, want Dietmar / Rietsch", got.FirstName, got.LastName)
	}
}

func TestAContactWithNoSurnameOnRecordOffersNone(t *testing.T) {
	e := setupSend(t)
	anchor := e.seedAnchor(t, "", "")

	// One-word name: a name, not a mistake. Empty is the answer that keeps the
	// greeting familiar rather than producing a formal one with nothing after
	// the honorific.
	sender := e.linkPerson(t, anchor, "Cher")
	e.seedPersonEmail(t, sender, "cher@buyer.test")
	e.participate(t, anchor, sender, "from", nil)

	got, err := e.store(nil).ReplyRecipientFor(e.as(principal.RowScopeAll), anchor)
	if err != nil {
		t.Fatalf("ReplyRecipientFor: %v", err)
	}
	if got.LastName != "" {
		t.Errorf("last name = %q, want empty for a one-word name", got.LastName)
	}
	if got.FullName != "Cher" {
		t.Errorf("full name = %q, want Cher", got.FullName)
	}
}
