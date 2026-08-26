// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// ownedRecord is a served record carrying an owner, which is what the query
// path actually hands to attachOwners: hydrated contract JSON, not a struct.
func ownedRecord(entity datasource.EntityType, owner ids.UUID) datasource.Record {
	fields := `{"source":"ui:company-form"}`
	if owner != (ids.UUID{}) {
		fields = `{"source":"ui:company-form","owner_id":"` + owner.String() + `"}`
	}
	return datasource.Record{
		Ref:       datasource.EntityRef{Type: entity, ID: ids.NewV7()},
		Fields:    json.RawMessage(fields),
		Freshness: datasource.FreshnessInfo{Authoritative: true},
	}
}

func humanCtx(seat ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat,
		Scopes: principal.NewScopeSet(principal.ScopeRead),
	})
}

func rowsFor(records ...datasource.Record) []QueryWorkspaceRow {
	rows := make([]QueryWorkspaceRow, 0, len(records))
	for _, rec := range records {
		rows = append(rows, QueryWorkspaceRow{Record: wireRecord{
			RecordType: string(rec.Ref.Type), ID: rec.Ref.ID, Fields: rec.Fields,
		}})
	}
	return rows
}

// The defect this whole file exists for: a company owned by a COLLEAGUE came
// back with a bare owner_id and nothing saying whose it was, so an assistant
// recommended visiting an account another rep was mid-contract on. The row has
// to name the owner and say it is not the caller.
func TestARecordOwnedByAColleagueIsNamedAndMarkedNotYours(t *testing.T) {
	me, sofia := ids.NewV7(), ids.NewV7()
	rows := rowsFor(ownedRecord(datasource.EntityOrganization, sofia))

	named, _ := attachOwners(humanCtx(me), func(_ context.Context, seats []ids.UUID) (map[ids.UUID]string, error) {
		return map[ids.UUID]string{sofia: "Sofia Meier"}, nil
	}, rows)

	if named[0].Owner == nil {
		t.Fatal("a company owned by a colleague came back with no owner at all, which is the defect")
	}
	if named[0].Owner.Name != "Sofia Meier" {
		t.Errorf("the owner is not named: %q — a bare uuid is what a reader cannot act on", named[0].Owner.Name)
	}
	if named[0].Owner.IsYou {
		t.Error("a colleague's account is marked as the caller's own")
	}
	if named[0].Owner.ID != sofia {
		t.Error("the owner id did not survive naming")
	}
}

// The other half: the caller's OWN record must not read as someone else's, or
// the signal is noise and a reader learns to ignore it.
func TestYourOwnRecordIsMarkedAsYours(t *testing.T) {
	me := ids.NewV7()
	rows := rowsFor(ownedRecord(datasource.EntityOrganization, me))

	named, _ := attachOwners(humanCtx(me), func(_ context.Context, seats []ids.UUID) (map[ids.UUID]string, error) {
		return map[ids.UUID]string{me: "Lars"}, nil
	}, rows)

	if named[0].Owner == nil || !named[0].Owner.IsYou {
		t.Fatal("the caller's own account is not marked as theirs")
	}
}

// An unowned record has no owner to check with, and saying it has one would be
// a false claim about a colleague who does not exist.
func TestAnUnownedRecordCarriesNoOwner(t *testing.T) {
	rows := rowsFor(ownedRecord(datasource.EntityOrganization, ids.UUID{}))

	named, _ := attachOwners(humanCtx(ids.NewV7()), func(_ context.Context, seats []ids.UUID) (map[ids.UUID]string, error) {
		t.Error("an unowned record still asked for a seat name")
		return map[ids.UUID]string{}, nil
	}, rows)

	if named[0].Owner != nil {
		t.Errorf("an unowned record claims an owner: %+v", named[0].Owner)
	}
}

// A page of rows sharing owners must cost ONE lookup, not one per row — the
// reason the naming runs after the page is assembled rather than inside serve.
func TestAPageOfRowsIsNamedInOneLookup(t *testing.T) {
	sofia, lena := ids.NewV7(), ids.NewV7()
	rows := rowsFor(
		ownedRecord(datasource.EntityOrganization, sofia),
		ownedRecord(datasource.EntityOrganization, lena),
		ownedRecord(datasource.EntityOrganization, sofia),
		ownedRecord(datasource.EntityOrganization, sofia),
	)

	calls, asked := 0, 0
	_, _ = attachOwners(humanCtx(ids.NewV7()), func(_ context.Context, seats []ids.UUID) (map[ids.UUID]string, error) {
		calls, asked = calls+1, len(seats)
		return map[ids.UUID]string{sofia: "Sofia Meier", lena: "Lena Fischer"}, nil
	}, rows)

	if calls != 1 {
		t.Errorf("four rows cost %d seat lookups; a page must cost one", calls)
	}
	if asked != 2 {
		t.Errorf("asked for %d seats across rows owned by 2 people — the ids are not deduplicated", asked)
	}
}

// A seat that no longer resolves — an archived member — keeps its id and its
// not-yours marker. Dropping the owner entirely would read as UNOWNED, which
// is the one wrong answer: it invites exactly the contact this disclosure
// exists to prevent.
func TestAnArchivedOwnerStillReadsAsOwnedBySomeoneElse(t *testing.T) {
	gone := ids.NewV7()
	rows := rowsFor(ownedRecord(datasource.EntityOrganization, gone))

	named, _ := attachOwners(humanCtx(ids.NewV7()), func(_ context.Context, seats []ids.UUID) (map[ids.UUID]string, error) {
		return map[ids.UUID]string{}, nil
	}, rows)

	if named[0].Owner == nil {
		t.Fatal("an unresolvable owner made the record read as unowned")
	}
	if named[0].Owner.Name != "" {
		t.Errorf("a name was invented for an archived seat: %q", named[0].Owner.Name)
	}
	if named[0].Owner.IsYou {
		t.Error("an archived colleague's record is marked as the caller's own")
	}
}

// Naming is a courtesy on rows the caller has ALREADY been granted. A namer
// that fails must not fail the read — the rows are admitted either way, and
// the id plus the marker still disclose that someone else holds the account.
//
// But it must SAY it failed. Without the note, a lookup timeout and a
// colleague who left the company produce the same unnamed owner, and only one
// of those means "ask around before you contact this account".
func TestANamingFailureDisclosesTheOwnerAndSaysItFailed(t *testing.T) {
	sofia := ids.NewV7()
	rows := rowsFor(ownedRecord(datasource.EntityOrganization, sofia))

	named, note := attachOwners(humanCtx(ids.NewV7()), func(_ context.Context, seats []ids.UUID) (map[ids.UUID]string, error) {
		return nil, context.DeadlineExceeded
	}, rows)

	if named[0].Owner == nil || named[0].Owner.ID != sofia {
		t.Fatal("a failed seat lookup swallowed the ownership disclosure entirely")
	}
	if named[0].Owner.IsYou {
		t.Error("a failed lookup marked a colleague's record as the caller's own")
	}
	if note == nil {
		t.Fatal("a failed lookup is indistinguishable from an owner who left the company")
	}
	if note.Code != CodeOwnerNamesUnavailable {
		t.Errorf("the note carries %q rather than the code a client branches on", note.Code)
	}
}

// The mirror of the above: a seat that simply does not resolve is NOT a
// failure, and must not raise the alarm. An owner who left is an ordinary
// answer.
func TestAnArchivedOwnerRaisesNoFailureNote(t *testing.T) {
	rows := rowsFor(ownedRecord(datasource.EntityOrganization, ids.NewV7()))

	_, note := attachOwners(humanCtx(ids.NewV7()), func(_ context.Context, seats []ids.UUID) (map[ids.UUID]string, error) {
		return map[ids.UUID]string{}, nil
	}, rows)

	if note != nil {
		t.Errorf("an owner who left was reported as a lookup failure: %+v", note)
	}
}

// An installation with no seat namer wired still serves the query and still
// discloses ownership, unnamed.
func TestNoSeatNamerStillDisclosesTheOwner(t *testing.T) {
	sofia := ids.NewV7()
	rows := rowsFor(ownedRecord(datasource.EntityOrganization, sofia))

	named, _ := attachOwners(humanCtx(ids.NewV7()), nil, rows)
	if named[0].Owner == nil || named[0].Owner.ID != sofia {
		t.Fatal("without a namer the owner vanished rather than going unnamed")
	}
}

// An agent reads on a HUMAN's behalf, so "yours" means that human's.
//
// This is the case the first cut of this file got backwards. Reading only a
// human principal's seat left every assistant-driven query marking its own
// operator's accounts as a colleague's — the exact inversion the disclosure
// exists to prevent, and worse than saying nothing, because it sends someone
// to check with themselves.
func TestAnAgentReadsAsTheHumanItActsFor(t *testing.T) {
	me := ids.NewV7()
	rows := rowsFor(ownedRecord(datasource.EntityOrganization, me))

	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:probe", OnBehalfOf: me,
		Scopes: principal.NewScopeSet(principal.ScopeRead),
	})

	named, _ := attachOwners(ctx, func(_ context.Context, seats []ids.UUID) (map[ids.UUID]string, error) {
		return map[ids.UUID]string{me: "Lars"}, nil
	}, rows)

	if named[0].Owner == nil {
		t.Fatal("an agent's read lost the owner")
	}
	if !named[0].Owner.IsYou {
		t.Error("an assistant reading on my behalf marked MY OWN account as somebody else's")
	}
}

// And the other half: a colleague's account stays a colleague's when an agent
// reads it, or the marker means nothing.
func TestAnAgentStillSeesAColleaguesRecordAsTheirs(t *testing.T) {
	me, sofia := ids.NewV7(), ids.NewV7()
	rows := rowsFor(ownedRecord(datasource.EntityOrganization, sofia))

	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:probe", OnBehalfOf: me,
		Scopes: principal.NewScopeSet(principal.ScopeRead),
	})

	named, _ := attachOwners(ctx, func(_ context.Context, seats []ids.UUID) (map[ids.UUID]string, error) {
		return map[ids.UUID]string{sofia: "Sofia Meier"}, nil
	}, rows)

	if named[0].Owner == nil || named[0].Owner.IsYou {
		t.Fatal("a colleague's account read as the operator's own")
	}
}

// A system principal has no human behind it, so nothing is its own.
func TestASystemPrincipalOwnsNothing(t *testing.T) {
	owner := ids.NewV7()
	rows := rowsFor(ownedRecord(datasource.EntityOrganization, owner))

	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:sweep",
		Scopes: principal.NewScopeSet(principal.ScopeRead),
	})

	named, _ := attachOwners(ctx, func(_ context.Context, seats []ids.UUID) (map[ids.UUID]string, error) {
		return map[ids.UUID]string{owner: "Sofia Meier"}, nil
	}, rows)

	if named[0].Owner == nil || named[0].Owner.IsYou {
		t.Error("a system principal is marked as owning a record")
	}
}

// Every owned record type gets the same treatment — the accessor reads the
// contract's own spelling, so a new owned type is covered when it appears.
func TestEveryOwnedRecordTypeIsNamed(t *testing.T) {
	owner := ids.NewV7()
	for _, entity := range []datasource.EntityType{
		datasource.EntityOrganization, datasource.EntityDeal,
		datasource.EntityPerson, datasource.EntityLead,
	} {
		rows := rowsFor(ownedRecord(entity, owner))
		named, _ := attachOwners(humanCtx(ids.NewV7()), func(_ context.Context, seats []ids.UUID) (map[ids.UUID]string, error) {
			return map[ids.UUID]string{owner: "Sofia Meier"}, nil
		}, rows)
		if named[0].Owner == nil || named[0].Owner.Name != "Sofia Meier" {
			t.Errorf("%s did not carry a named owner", entity)
		}
	}
}

// The model only checks with the owner if the copy tells it to. This is the
// half of the fix that lives in words rather than in the payload.
func TestTheCopyTellsTheModelWhatAnOwnerMeans(t *testing.T) {
	described := queryWorkspaceCopy.render()
	if !strings.Contains(described, "owner") {
		t.Fatal("the description never mentions the owner a row now carries")
	}
	if !strings.Contains(described, "is_you") {
		t.Error("the description never names is_you, so a model cannot branch on it")
	}
}

// The wiring, held end to end through Handle.
//
// Every test above calls attachOwners directly, so all of them stay green if
// hydrate stops calling it — which is the whole feature silently gone. This is
// the one that fails when the call is dropped, and it is checked in the RAW
// BYTES because that is what a client actually reads.
func TestAServedRowCarriesItsOwnerThroughTheWholeCall(t *testing.T) {
	me, sofia := ids.NewV7(), ids.NewV7()
	owned := ownedRecord(datasource.EntityOrganization, sofia)

	tool := queryWorkspace{
		p: &queryProbeProvider{records: map[ids.UUID]datasource.Record{owned.Ref.ID: owned}},
		run: func(context.Context, json.RawMessage) (QueryAnswer, error) {
			return QueryAnswer{
				Refs:     []QueryRef{{Type: "organization", ID: owned.Ref.ID}},
				Coverage: CoverageCompleteExact, Limit: 25,
			}, nil
		},
		name: func(_ context.Context, seats []ids.UUID) (map[ids.UUID]string, error) {
			return map[ids.UUID]string{sofia: "Sofia Meier"}, nil
		},
	}

	raw, err := tool.Handle(humanCtx(me), json.RawMessage(`{"plan":{"version":"v1","target":"organization"}}`))
	if err != nil {
		t.Fatalf("handling a plan over an owned record: %v", err)
	}
	if !strings.Contains(string(raw), `"owner"`) {
		t.Fatalf("a served row carries no owner — hydrate is not naming them:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"name":"Sofia Meier"`) {
		t.Errorf("the owner rode out unnamed:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"is_you":false`) {
		t.Errorf("the row never says the account is not the caller's:\n%s", raw)
	}
}
