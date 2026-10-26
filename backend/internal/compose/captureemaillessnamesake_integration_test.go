// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Mail from somebody already filed as a contact without an address. Capture
// never writes the address onto that contact. The real sink mints the sender,
// and the duplicate queue pairs the two once the verdict judges the sender.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// aliceSender writes from Company A's own domain. Not example.com: the shipped
// consumer-mail list carries that one, and a consumer domain names no employer.
const aliceSender = "alice@company-a.test"

// colleaguePerms is a seat that reads and edits every contact its row scope admits.
var colleaguePerms = principal.Permissions{
	Objects: map[string]principal.ObjectGrant{
		"contact": {Read: true, Update: true},
		"company": {Read: true},
	},
	RowScope: principal.RowScopeAll,
}

// seedEmaillessAlice files Alice Example by hand with no address, currently
// employed at a company that has the sender's domain registered.
func seedEmaillessAlice(t *testing.T, e *integration.Env) ids.ContactID {
	t.Helper()
	return seedEmaillessAlices(t, e, 1)[0]
}

// seedEmaillessAlices files that many such namesakes at the one company.
func seedEmaillessAlices(t *testing.T, e *integration.Env, count int) []ids.ContactID {
	t.Helper()
	ctx := e.Admin()
	company, err := e.Contacts.CreateCompany(ctx, contacts.CreateCompanyInput{
		DisplayName: "Company A", Source: "manual",
		Domains: []contacts.CompanyDomainInput{{Domain: "company-a.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding Company A: %v", err)
	}
	companyID, primary := ids.From[ids.CompanyKind](ids.UUID(company.Id)), true
	alices := make([]ids.ContactID, count)
	for i := range alices {
		alice := ids.From[ids.ContactKind](e.SeedContact(t, "Alice Example", nil))
		if _, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{
			Kind: "employment", ContactID: &alice, CompanyID: &companyID,
			IsCurrentPrimary: &primary, Source: "manual",
		}); err != nil {
			t.Fatalf("seeding Alice's employment: %v", err)
		}
		alices[i] = alice
	}
	return alices
}

// captureFromAlice runs the T1 path through the real sink: the workspace wrote
// to Alice's address, and she answers.
func captureFromAlice(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	seedAttestedOutbound(t, e, "alice-out-1", aliceSender, "alice-t1")
	captureNamedInbound(mailboxOwnerCtx(e, e.Rep1), t, e, "a@authz.test", "alice-in-1", aliceSender, "Alice Example", "alice-t1")
	dispositionID, queued := openDisposition(t, e, aliceSender)
	if !queued {
		t.Fatal("no verdict was opened for the captured sender")
	}
	return dispositionID
}

// contactsHolding answers the live contacts that hold address.
func contactsHolding(t *testing.T, e *integration.Env, address string) []ids.UUID {
	t.Helper()
	var held []ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT c.id FROM contact c JOIN contact_email ce ON ce.contact_id = c.id
			 WHERE ce.email = $1 AND ce.archived_at IS NULL AND c.archived_at IS NULL`, address)
		if err != nil {
			return err
		}
		held, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	}); err != nil {
		t.Fatalf("reading who holds %s: %v", address, err)
	}
	return held
}

// pairsJoining answers the open contact pairs a reader is shown that join a and b.
func pairsJoining(reader context.Context, t *testing.T, e *integration.Env, a, b ids.UUID) []contacts.DedupeCandidateRow {
	t.Helper()
	rows, _, err := e.Contacts.ListDedupeCandidates(reader, contacts.DedupeQueueInput{EntityType: "contact"})
	if err != nil {
		t.Fatalf("listing the duplicate queue: %v", err)
	}
	var out []contacts.DedupeCandidateRow
	for _, row := range rows {
		ends := map[ids.UUID]bool{row.LeftID: true, row.RightID: true}
		if ends[a] && ends[b] {
			out = append(out, row)
		}
	}
	return out
}

// senderOf answers the one contact capture minted for aliceSender, which is never alice.
func senderOf(t *testing.T, e *integration.Env, alice ids.ContactID) ids.UUID {
	t.Helper()
	held := contactsHolding(t, e, aliceSender)
	if len(held) != 1 || held[0] == alice.UUID {
		t.Fatalf("after capture %v hold %s, want one new contact and never Alice", held, aliceSender)
	}
	return held[0]
}

func TestMailFromAnEmaillessContactIsQueuedAgainstThemOnceTheSenderIsJudged(t *testing.T) {
	e := integration.Setup(t)
	alice := seedEmaillessAlice(t, e)
	colleague := e.As(e.Rep2, nil, colleaguePerms)

	dispositionID := captureFromAlice(t, e)
	sender := senderOf(t, e, alice)
	if pairs := pairsJoining(colleague, t, e, alice.UUID, sender); len(pairs) != 0 {
		t.Fatalf("a colleague is shown %d pairs while the sender awaits its verdict, want none", len(pairs))
	}

	runVerdict(t, e, &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindContact}})
	pairs := pairsJoining(colleague, t, e, alice.UUID, sender)
	if len(pairs) != 1 {
		t.Fatalf("a colleague is shown %d open pairs for Alice and the sender, want exactly 1", len(pairs))
	}

	winner := alice.UUID
	if _, err := e.Contacts.DisposeDedupeCandidate(e.Admin(), pairs[0].ID, "merge", &winner); err != nil {
		t.Fatalf("accepting the pair: %v", err)
	}
	if after := contactsHolding(t, e, aliceSender); len(after) != 1 || after[0] != alice.UUID {
		t.Fatalf("after the merge %v hold %s, want Alice alone", after, aliceSender)
	}
}

// A sender the verdict keeps the owner's, here an advisor, is never offered to
// colleagues as somebody's duplicate.
func TestASenderKeptTheOwnersIsNotQueuedForColleagues(t *testing.T) {
	e := integration.Setup(t)
	alice := seedEmaillessAlice(t, e)

	dispositionID := captureFromAlice(t, e)
	sender := senderOf(t, e, alice)
	runVerdict(t, e, &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindAdvisor}})
	if pairs := pairsJoining(e.As(e.Rep2, nil, colleaguePerms), t, e, alice.UUID, sender); len(pairs) != 0 {
		t.Fatalf("a colleague is shown %d pairs for a sender kept the owner's, want none", len(pairs))
	}
}

// An email-less name matching two namesakes at the employer queues a pair with
// each of them. Each pair is a question for a human: nothing merges, and the
// sender stays its own contact holding its own address.
func TestAnEmaillessNameMatchingTwoNamesakesQueuesAPairWithEach(t *testing.T) {
	e := integration.Setup(t)
	alices := seedEmaillessAlices(t, e, 2)

	dispositionID := captureFromAlice(t, e)
	sender := senderOf(t, e, alices[0])
	runVerdict(t, e, &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindContact}})
	colleague := e.As(e.Rep2, nil, colleaguePerms)
	for _, alice := range alices {
		if got := len(pairsJoining(colleague, t, e, alice.UUID, sender)); got != 1 {
			t.Fatalf("the sender is paired %d times with namesake %s, want 1", got, alice)
		}
	}
	if held := contactsHolding(t, e, aliceSender); len(held) != 1 || held[0] != sender {
		t.Fatalf("after the verdict %v hold %s, want the sender %s alone", held, aliceSender, sender)
	}
	var unmerged int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM contact
			 WHERE id = ANY($1) AND archived_at IS NULL AND merged_into_id IS NULL`,
			[]ids.UUID{sender, alices[0].UUID, alices[1].UUID}).Scan(&unmerged)
	}); err != nil {
		t.Fatalf("reading the three contacts: %v", err)
	}
	if unmerged != 3 {
		t.Fatalf("%d of the sender and two namesakes are live and unmerged, want 3: a review pair never merges", unmerged)
	}
}
