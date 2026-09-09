// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Deciding the matcher's suggestions, and what a decision does (ADR-0078
// §2.1b).
//
// The unit tests cannot reach any of this: every rule here is a property of
// SQL — that a confirmation stamps the connection's own URL onto the contact,
// that it never overwrites one already there, that a rejection is durable, and
// that the reach read counts what it says it counts.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// suggestedGhost is the export's one name-and-employer match — the row every
// test here decides. Named rather than parameterized because the fixture has
// exactly one suggestion by construction: Dana carries an address and
// auto-confirms, and Nobody Atall works somewhere unknown.
const suggestedGhost = "Andreas Müller"

// seedMember adds a second workspace member, for the tests that need a record
// belonging to somebody other than the importer.
func (e *dedupeEnv) seedMember(t *testing.T, name string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO app_user (id, email, display_name)
			VALUES ($1, $2, $3)`, id, id.String()+"@dd.test", name)
		return err
	}); err != nil {
		t.Fatalf("seeding member %q: %v", name, err)
	}
	return id
}

func (e *dedupeEnv) ghostID(t *testing.T) ids.UUID {
	t.Helper()
	ctx := e.as()
	var id ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT id FROM linkedin_connection WHERE full_name = $1`, suggestedGhost).Scan(&id)
	}); err != nil {
		t.Fatalf("reading the id of ghost %q: %v", suggestedGhost, err)
	}
	return id
}

func (e *dedupeEnv) linkedInHandle(t *testing.T, person ids.PersonID) (string, bool) {
	t.Helper()
	ctx := e.as()
	var handle string
	found := true
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT handle FROM person_social WHERE person_id = $1 AND platform = 'linkedin'`,
			person.UUID).Scan(&handle)
		if err != nil && err.Error() == pgx.ErrNoRows.Error() {
			found = false
			return nil
		}
		return err
	}); err != nil {
		t.Fatalf("reading %s's LinkedIn handle: %v", person, err)
	}
	return handle, found
}

// importAndMatch is the two steps every test here starts from: the export
// lands, the matcher runs, and the suggestions are waiting.
func (e *dedupeEnv) importAndMatch(t *testing.T) {
	t.Helper()
	e.importExport(t)
	if _, err := e.store.MatchLinkedInConnections(e.as(), e.rep); err != nil {
		t.Fatalf("matching: %v", err)
	}
}

func TestApplyingAnApprovedMatchPutsTheLinkedInURLOnTheContact(t *testing.T) {
	// The write an approved proposal releases. It is the same write the
	// automatic exact-name path performs — the difference is only who released
	// it, a string comparison there and a person here.
	e := setupDedupe(t)
	company := e.seedCompanyNamed(t, "Acme GmbH")
	andreas := e.seedContact(t, "Andreas Muller")
	e.employ(t, andreas, company)
	e.importAndMatch(t)

	if err := e.store.ApplyLinkedInMatch(e.as(), e.ghostID(t), e.rep, andreas.UUID); err != nil {
		t.Fatalf("applying the approved match: %v", err)
	}
	if status, person := e.ghostStatus(t, suggestedGhost); status != "confirmed" || person == nil || *person != andreas.UUID {
		t.Fatalf("the connection is %q → %v, want confirmed → %s", status, person, andreas)
	}
	handle, found := e.linkedInHandle(t, andreas)
	if !found {
		t.Fatal("the contact gained no LinkedIn handle — a match that changes nothing is not worth approving")
	}
	if handle != "https://www.linkedin.com/in/amueller" {
		t.Errorf("the contact carries %q, want the CONNECTION's own profile URL", handle)
	}
}

func TestApplyingAMatchNeverOverwritesAHandleTheContactAlreadyHad(t *testing.T) {
	// A value already on a record is somebody's statement, and approving a
	// match is not grounds to replace it.
	e := setupDedupe(t)
	company := e.seedCompanyNamed(t, "Acme GmbH")
	andreas := e.seedContact(t, "Andreas Muller")
	e.employ(t, andreas, company)
	existing := "https://www.linkedin.com/in/the-one-we-already-had"
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO person_social (person_id, platform, handle)
			VALUES ($1, 'linkedin', $2)`,
			andreas.UUID, existing)
		return err
	}); err != nil {
		t.Fatalf("seeding the existing handle: %v", err)
	}
	e.importAndMatch(t)

	if err := e.store.ApplyLinkedInMatch(e.as(), e.ghostID(t), e.rep, andreas.UUID); err != nil {
		t.Fatalf("applying the approved match: %v", err)
	}
	// The link still stands — only the copy did not happen.
	if status, _ := e.ghostStatus(t, suggestedGhost); status != "confirmed" {
		t.Errorf("the connection is %q, want confirmed even though nothing was copied", status)
	}
	if handle, _ := e.linkedInHandle(t, andreas); handle != existing {
		t.Errorf("the contact's handle became %q, want the one already on the record %q", handle, existing)
	}
}

func TestTheMatchesAwaitingADecisionAreTheCallersOwnAndCarryTheExportsSpelling(t *testing.T) {
	e := setupDedupe(t)
	company := e.seedCompanyNamed(t, "Acme GmbH")
	andreas := e.seedContact(t, "Andreas Muller")
	e.employ(t, andreas, company)
	e.importAndMatch(t)

	pending, err := e.store.PendingLinkedInMatches(e.as())
	if err != nil {
		t.Fatalf("reading the pending matches: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("%d matches await a decision (%+v), want the one folded-name candidate", len(pending), pending)
	}
	// The ORIGINAL strings: nobody can judge a proposal rendered as
	// "andreas muller · acme".
	if pending[0].ConnectionName != suggestedGhost || pending[0].ConnectionCompany != "Acme GmbH" {
		t.Errorf("the candidate reads %q at %q, want the export's own spelling",
			pending[0].ConnectionName, pending[0].ConnectionCompany)
	}
	if pending[0].PersonName != "Andreas Muller" {
		t.Errorf("the candidate names contact %q, want the suggested contact", pending[0].PersonName)
	}
}

func TestReachCountsConnectionsPerAccountAndSaysWhatItCannotShow(t *testing.T) {
	e := setupDedupe(t)
	company := e.seedCompanyNamed(t, "Acme GmbH")
	dana := e.seedContact(t, "Dana Buyer")
	e.seedEmail(t, dana, "dana@acme.test")
	e.employ(t, dana, company)
	andreas := e.seedContact(t, "Andreas Müller")
	e.employ(t, andreas, company)
	e.importAndMatch(t)

	reach, err := e.store.MyLinkedInReach(e.as(), nil)
	if err != nil {
		t.Fatalf("reading reach: %v", err)
	}
	if len(reach.Accounts) != 1 {
		t.Fatalf("reach lists %d accounts (%+v), want the one on file", len(reach.Accounts), reach.Accounts)
	}
	acme := reach.Accounts[0]
	if acme.CompanyID != company.UUID {
		t.Errorf("reach names account %s, want %s", acme.CompanyID, company)
	}
	// Two of the three exported connections work at Acme.
	if acme.Connections != 2 {
		t.Errorf("reach counts %d connections at the account, want 2", acme.Connections)
	}
	// Both confirmed without a human: Dana by her exported address, Andreas by
	// an exact name at an employer that resolved. Neither needed asking, which
	// is the point — a queue nobody has to work through is the best queue.
	if acme.ContactsOnFile != 2 {
		t.Errorf("reach counts %d contacts on file, want 2 (the address match and the exact-name match)",
			acme.ContactsOnFile)
	}
	// The connection at a company nobody here knows is reported as unresolved
	// rather than dropped, because that number is what shrinks as accounts are
	// created.
	if reach.UnresolvedConnections != 1 {
		t.Errorf("reach reports %d unresolved connections, want 1", reach.UnresolvedConnections)
	}
	if reach.AccountsTotal != 1 {
		t.Errorf("reach reports %d accounts in total, want 1", reach.AccountsTotal)
	}
}

func TestCollapseNeverLetsAMachineGuessOverrideAHumanConfirmation(t *testing.T) {
	e := setupDedupe(t)
	company := e.seedCompanyNamed(t, "Acme GmbH")
	guessed := e.seedContact(t, "Guessed Person")
	confirmed := e.seedContact(t, "Confirmed Person")
	e.employ(t, guessed, company)
	e.employ(t, confirmed, company)

	// Two rows for ONE connection, the state a normalizer change leaves behind.
	// The OLDER carries the matcher's guess; the NEWER carries a human's
	// confirmation of somebody else. Treating `suggested` as a decision let the
	// older row win on age and then inherit the status `confirmed` — reporting
	// a decision the member never made, about a person they did not name.
	ctx := e.as()
	older, newer := ids.NewV7(), ids.NewV7()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		for _, row := range []struct {
			id       ids.UUID
			company  string
			status   string
			person   ids.PersonID
			syncedAt string
		}{
			{older, "Acme GmbH | Digital", "suggested", guessed, "2026-01-01"},
			{newer, "Acme GmbH", "confirmed", confirmed, "2026-06-01"},
		} {
			if _, err := tx.Exec(ctx, `
				INSERT INTO linkedin_connection
				    (id, owner_user_id, full_name, normalized_name,
				     company_name, normalized_company, matched_person_id, match_status,
				     source, synced_at)
				VALUES ($1,
				        $2, 'Dup Person', 'dup person', $3, $4, $5, $6, 'csv_export', $7::date)`,
				row.id, e.rep, row.company, NormalizeCompanyName(row.company),
				row.person.UUID, row.status, row.syncedAt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the duplicate pair: %v", err)
	}

	if _, err := e.store.RenormalizeLinkedInCompanyKeys(e.as()); err != nil {
		t.Fatalf("re-normalizing: %v", err)
	}

	status, person := e.ghostStatus(t, "Dup Person")
	if status != "confirmed" {
		t.Fatalf("the surviving row is %q, want confirmed — the human decided", status)
	}
	if person == nil || *person != confirmed.UUID {
		t.Errorf("the surviving row points at %v, want the contact the HUMAN confirmed (%s), "+
			"not the matcher's guess (%s)", person, confirmed, guessed)
	}
}

func TestAConnectionNeverMatchesAContactItsOwnerMayNotSee(t *testing.T) {
	e := setupDedupe(t)
	company := e.seedCompanyNamed(t, "Acme GmbH")
	// A contact somebody ELSE captured privately. Capture privacy makes it
	// theirs alone — not even an admin reads it — and the matcher runs as a
	// system principal, which is exempt from that rule by design. Without the
	// boundary carried on the match itself, this ghost would link to it and the
	// review list would report the link back to a member who cannot open it.
	private := e.seedContact(t, "Andreas Müller")
	e.employ(t, private, company)
	// Owned by a DIFFERENT member. Owned by the importer it would rightly
	// match — capture privacy is about whose record it is, not about hiding it
	// from everybody.
	colleague := e.seedMember(t, "Colleague")
	ctx := e.as()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE person SET visibility = 'owner', owner_id = $2 WHERE id = $1`,
			private.UUID, colleague)
		return err
	}); err != nil {
		t.Fatalf("making the contact owner-private: %v", err)
	}

	e.importAndMatch(t)

	if status, person := e.ghostStatus(t, "Andreas Müller"); status != "unmatched" || person != nil {
		t.Errorf("a ghost matched a contact its owner may not see: %q → %v", status, person)
	}
}

// The apply is BOUND to the connection its proposal was staged for, on every
// axis the payload names.
//
// The guards on this path gate the PERSON — the update grant and the target's
// visibility — and nothing tied the CONNECTION. So an apply would re-point an
// already-confirmed row to a different contact, and would land on another
// member's connection, if the payload said so. Each case here is one of the
// three predicates, and each must change zero rows and answer not-found rather
// than reporting a link it did not make.
func TestApplyingAMatchRefusesAConnectionTheProposalDoesNotDescribe(t *testing.T) {
	e := setupDedupe(t)
	company := e.seedCompanyNamed(t, "Acme GmbH")
	andreas := e.seedContact(t, "Andreas Muller")
	e.employ(t, andreas, company)
	e.importAndMatch(t)
	ghost := e.ghostID(t)

	other := e.seedContact(t, "Someone Else")

	for _, tc := range []struct {
		name          string
		owner, person ids.UUID
		what          string
	}{
		{
			name: "another member's connection", owner: ids.NewV7(), person: andreas.UUID,
			what: "a member decides about their own imported network and nobody else's",
		},
		{
			name: "a pair the row does not name", owner: e.rep, person: other.UUID,
			what: "the pair IS the claim, so an approval released against one contact must not " +
				"apply to whoever the row points at now",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := e.store.ApplyLinkedInMatch(e.as(), ghost, tc.owner, tc.person); !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("err = %v, want ErrNotFound — %s", err, tc.what)
			}
			if status, _ := e.ghostStatus(t, suggestedGhost); status != "suggested" {
				t.Errorf("the connection is %q, want it untouched at suggested", status)
			}
		})
	}

	// The outcome the three clauses exist for, asserted as an outcome: a
	// confirmed row is never re-pointed.
	//
	// NOT claimed as a test of the state clause. I could not build a case where
	// `match_status = 'suggested'` is the operative refusal through this entry
	// point — every route to an apply carrying a non-suggested row is turned
	// away earlier, by the pair clause or by the person lock — and neutralising
	// the state clause alone fails nothing here. It stays because the ticket
	// asks for it and because it is true, and it is defence in depth on this
	// path rather than the guard that holds. The owner and pair clauses ARE
	// load-bearing, and each fails this case when neutralised.
	if err := e.store.ApplyLinkedInMatch(e.as(), ghost, e.rep, andreas.UUID); err != nil {
		t.Fatalf("the correct apply was refused: %v", err)
	}
	if err := e.store.ApplyLinkedInMatch(e.as(), ghost, e.rep, other.UUID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound — a confirmed row is a link somebody already has", err)
	}
	if status, person := e.ghostStatus(t, suggestedGhost); status != "confirmed" ||
		person == nil || *person != andreas.UUID {
		t.Errorf("the connection is %q → %v, want it left confirmed to the contact it names",
			status, person)
	}
}

// A proposal staged BEFORE owner_user_id existed still applies.
//
// The payload gained the field, and a pending approval minted before this
// shipped carries none — so its OwnerUserID decodes to the zero value. Binding
// on that would match no row, the effect would fail, and the member could never
// decide it: re-deciding a decided row answers 409. So a zero owner is resolved
// from the connection, which is where staging reads it from anyway.
//
// The zero id is passed directly rather than through a stored payload: what is
// under test is the store's handling of a proposal that names no owner, and
// building a legacy approval row to carry one there would be asserting against
// a fixture of the old format rather than against the behaviour.
func TestAProposalWithNoOwnerStillAppliesAgainstItsConnection(t *testing.T) {
	e := setupDedupe(t)
	company := e.seedCompanyNamed(t, "Acme GmbH")
	andreas := e.seedContact(t, "Andreas Muller")
	e.employ(t, andreas, company)
	e.importAndMatch(t)

	if err := e.store.ApplyLinkedInMatch(e.as(), e.ghostID(t), ids.Nil, andreas.UUID); err != nil {
		t.Fatalf("a proposal predating owner_user_id was refused: %v", err)
	}
	if status, person := e.ghostStatus(t, suggestedGhost); status != "confirmed" ||
		person == nil || *person != andreas.UUID {
		t.Errorf("the connection is %q → %v, want it confirmed to the contact the proposal named",
			status, person)
	}
}

// The decided event names the MEMBER, not the connection.
//
// linkedin_match.decided is a self-only event — a colleague's professional
// network is theirs — so delivery admits it only when the subscriber IS the
// user its entity id names. It was emitted with the CONNECTION id, which can
// never equal a user id, so the event was silently undeliverable for every
// subscriber, always.
//
// Asserted on the outbox row rather than through the deliverer: what was wrong
// is the id this write puts on the wire, and that is a fact about this
// transaction. Whether the visibility rule then admits the right subscriber is
// the webhooks suite's own subject, and it covers all three self-only events.
func TestTheDecidedEventCarriesTheMembersOwnID(t *testing.T) {
	e := setupDedupe(t)
	company := e.seedCompanyNamed(t, "Acme GmbH")
	andreas := e.seedContact(t, "Andreas Muller")
	e.employ(t, andreas, company)
	e.importAndMatch(t)
	ghost := e.ghostID(t)

	if err := e.store.ApplyLinkedInMatch(e.as(), ghost, e.rep, andreas.UUID); err != nil {
		t.Fatalf("applying the approved match: %v", err)
	}

	var entityID ids.UUID
	if err := e.store.tx(e.as(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.as(),
			`SELECT (envelope->'entity'->>'id')::uuid FROM event_outbox
			  WHERE envelope->>'type' = 'linkedin_match.decided'`).Scan(&entityID)
	}); err != nil {
		t.Fatalf("reading the decided event: %v", err)
	}
	if entityID == ghost {
		t.Fatal("the decided event names the CONNECTION — a self-only rule compares its id against a " +
			"subscriber's own user id, so no subscriber can ever receive it")
	}
	if entityID != e.rep {
		t.Errorf("the decided event names %s, want the member whose network produced the match (%s)",
			entityID, e.rep)
	}
}
