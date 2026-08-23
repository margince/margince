// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An agent writes exactly what its granting human writes, never more.
//
// Margince states this to customers as "the AI uses your scope", and MCP is
// governed the same way: a rep's assistant must not be able to change a
// colleague's records when the rep could not. Today the promise holds by
// construction — the agent door is middleware into the same REST handlers, and
// AgentIdentity.Principal() copies the human's permissions and teams — but
// construction is not a guarantee. A future tool executor that resolved records
// itself would break the promise without failing anything.
//
// So the promise is asserted as an OUTCOME rather than as a shape: for each of
// the five record types every seat reads, the same mutation is driven by the
// human, by their agent, and by a different human's agent, and the three
// verdicts must line up. The shape half — that Principal() copies what it
// should — is held next to the code it constrains, in
// modules/identity/passportprincipal_test.go, which needs no database.

import (
	"context"
	"errors"
	"testing"

	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/modules/identity"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// writeVerdict is what a mutation answered, collapsed to the three outcomes the
// access model distinguishes. It is deliberately three-valued rather than a
// bool: 403 and 404 mean different things here — the row is visibly not yours
// to change, versus the row is not yours to know about — and a boolean would
// let one silently become the other while the parity assertion stayed green.
type writeVerdict string

const (
	verdictAdmitted writeVerdict = "admitted"
	verdictDenied   writeVerdict = "refused 403 (visible, not yours to change)"
	verdictHidden   writeVerdict = "refused 404 (not yours to see)"
)

func classifyWrite(err error) writeVerdict {
	switch {
	case err == nil:
		return verdictAdmitted
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return verdictDenied
	case errors.Is(err, apperrors.ErrNotFound):
		return verdictHidden
	default:
		return writeVerdict("unexpected error: " + err.Error())
	}
}

// parityCase is one record type: how to seed a row owned by somebody, and how
// to attempt the plainest possible change to it.
type parityCase struct {
	table string
	seed  func(t *testing.T, e *Env, owner *ids.UUID) ids.UUID
	write func(ctx context.Context, e *Env, id ids.UUID) error
	perms principal.Permissions
}

func parityCases(t *testing.T, e *Env) []parityCase {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	title := "Changed by the parity suite"

	// AccountRepPerms reads organizations but does not update them, and no
	// shipped rep fixture does — several suites read those fixtures as a rep who
	// CANNOT write a company, so widening one there would make them pass while
	// proving nothing. This case needs the object verb before row scope is even
	// consulted (auth.Require runs first and refuses everyone without it), so it
	// carries its own copy rather than reaching for a shared one.
	orgWriterPerms := AccountRepPerms
	orgWriterPerms.Objects = map[string]principal.ObjectGrant{}
	for object, grant := range AccountRepPerms.Objects {
		orgWriterPerms.Objects[object] = grant
	}
	orgWriterPerms.Objects[objOrg] = principal.ObjectGrant{Create: true, Read: true, Update: true}

	return []parityCase{
		{
			table: "person",
			perms: AccountRepPerms,
			seed:  func(t *testing.T, e *Env, owner *ids.UUID) ids.UUID { return e.SeedPerson(t, "Parity person", owner) },
			write: func(ctx context.Context, e *Env, id ids.UUID) error {
				_, err := e.People.UpdatePerson(ctx, ids.From[ids.PersonKind](id),
					people.UpdatePersonInput{Title: &title})
				return err
			},
		},
		{
			table: "organization",
			perms: orgWriterPerms,
			seed:  func(t *testing.T, e *Env, owner *ids.UUID) ids.UUID { return e.SeedOrg(t, "Parity company", owner) },
			write: func(ctx context.Context, e *Env, id ids.UUID) error {
				_, err := e.People.UpdateOrganization(ctx, ids.From[ids.OrganizationKind](id),
					people.UpdateOrganizationInput{Description: &title})
				return err
			},
		},
		{
			table: "deal",
			perms: AccountRepPerms,
			seed: func(t *testing.T, e *Env, owner *ids.UUID) ids.UUID {
				return e.SeedDeal(t, "Parity deal", pipeline, open, owner)
			},
			write: func(ctx context.Context, e *Env, id ids.UUID) error {
				_, err := e.Deals.UpdateDeal(ctx, ids.From[ids.DealKind](id),
					deals.UpdateDealInput{Name: &title})
				return err
			},
		},
	}
}

// TestAnAgentWritesExactlyWhatItsHumanWrites is the promise itself. For each
// record type and each ownership arrangement, the human's verdict and their
// agent's verdict must be the SAME verdict — not merely both refusals, and not
// merely both successes.
//
// The third principal is what makes the pair meaningful. An agent that refused
// everything would satisfy a human/agent comparison trivially, so every row is
// also driven by an agent acting for the OTHER human, whose verdict must be the
// mirror image: admitted where the first pair is refused, refused where it is
// admitted.
func TestAnAgentWritesExactlyWhatItsHumanWrites(t *testing.T) {
	e := Setup(t)
	svc := identity.NewService(e.Pool)

	for _, tc := range parityCases(t, e) {
		t.Run(tc.table, func(t *testing.T) {
			rep1Team := []ids.UUID{e.Team1}
			rep3Team := []ids.UUID{e.Team2}

			mine := tc.seed(t, e, &e.Rep1)
			theirs := tc.seed(t, e, &e.Rep3)
			sharedWithMe := tc.seed(t, e, &e.Rep3)

			// The share goes through the real writer: a hand-inserted grant row
			// would prove the rule against a state production cannot reach.
			if _, err := svc.CreateRecordGrant(e.Admin(), identity.CreateGrantInput{
				RecordType: tc.table, RecordID: sharedWithMe,
				SubjectType: "user", SubjectID: e.Rep1, Access: "write",
			}); err != nil {
				t.Fatalf("sharing the %s with rep1 at write: %v", tc.table, err)
			}

			// The verdicts are asserted ABSOLUTELY, not merely against each
			// other. A gate that refused everyone would keep the human and their
			// agent in perfect agreement on every row, so a relative assertion
			// would report the promise kept by a product that had stopped
			// answering yes to anybody.
			for _, row := range []struct {
				name                       string
				id                         ids.UUID
				rep1, rep1Agent, rep3Agent writeVerdict
			}{
				{"owned by rep1", mine, verdictAdmitted, verdictAdmitted, verdictDenied},
				{"owned by rep3", theirs, verdictDenied, verdictDenied, verdictAdmitted},
				{"owned by rep3, shared to rep1 at write", sharedWithMe,
					verdictAdmitted, verdictAdmitted, verdictAdmitted},
			} {
				human := classifyWrite(tc.write(e.As(e.Rep1, rep1Team, tc.perms), e, row.id))
				agent := classifyWrite(tc.write(e.AgentFor(t, e.Rep1, rep1Team, tc.perms), e, row.id))
				stranger := classifyWrite(tc.write(e.AgentFor(t, e.Rep3, rep3Team, tc.perms), e, row.id))

				if human != agent {
					t.Errorf("%s %s: the human was %s and their own agent was %s — an agent's write "+
						"authority is its granting human's, and this pair has come apart",
						tc.table, row.name, human, agent)
				}
				for _, got := range []struct {
					who       string
					got, want writeVerdict
				}{
					{"rep1", human, row.rep1},
					{"an agent for rep1", agent, row.rep1Agent},
					{"an agent for rep3", stranger, row.rep3Agent},
				} {
					if got.got != got.want {
						t.Errorf("%s %s: %s was %s, want %s",
							tc.table, row.name, got.who, got.got, got.want)
					}
				}
			}
		})
	}
}

// TestAnAgentForAStrangerIsRefusedOnEveryRecordType is the guard against the
// parity test passing vacuously. If EnsureWritable were made to return nil
// unconditionally, every verdict above would become "admitted" on both sides
// and the pair would still line up. This asserts the refusal itself.
func TestAnAgentForAStrangerIsRefusedOnEveryRecordType(t *testing.T) {
	e := Setup(t)
	for _, tc := range parityCases(t, e) {
		t.Run(tc.table, func(t *testing.T) {
			theirs := tc.seed(t, e, &e.Rep1)
			got := classifyWrite(tc.write(e.AgentFor(t, e.Rep3, []ids.UUID{e.Team2}, tc.perms), e, theirs))
			if got == verdictAdmitted {
				t.Errorf("an agent acting for rep3 changed a %s owned by rep1 — the AI is not bounded "+
					"by the human that granted it", tc.table)
			}
		})
	}
}
