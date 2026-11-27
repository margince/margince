// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A company says which colleagues hold contacts at it that the reader may not
// see, beside the contact_count of the ones they may. The holders come from
// the section the count reads, so the two describe one population.

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func holderIDs(c crmcontracts.Company) []ids.UUID {
	if c.HiddenContactHolders == nil {
		return nil
	}
	out := []ids.UUID{}
	for _, holder := range *c.HiddenContactHolders {
		out = append(out, ids.UUID(holder.UserId))
	}
	return out
}

func TestCompanyNamesTheColleaguesWhoHoldContactsTheReaderCannotSee(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	empty := e.SeedCompany(t, "Open Account", &e.Rep1)
	seedEmployee(t, e, company, "Visible Buyer", e.Rep1, false)
	seedEmployee(t, e, company, "Hidden Sponsor", e.Rep3, true)
	seedEmployee(t, e, company, "Hidden Engineer", e.Rep3, true)
	seedEmployee(t, e, company, "Orphaned Champion", e.Rep2, true)
	deactivateThroughTheDirectory(t, e, e.Rep2)
	archived := seedEmployee(t, e, company, "Archived Lead", e.AdminUser, true)
	e.WsExec(t, `UPDATE contact SET archived_at = now() WHERE id = $1`, archived)
	seedEmployee(t, e, empty, "Shared Contact", e.Rep3, false)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, hiddenReaderPerms)
	got, err := e.Contacts.GetCompany(rep, companyIDOf(company), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContactCount == nil || *got.ContactCount != 1 {
		t.Errorf("contact_count = %v, want the 1 contact the rep may see", got.ContactCount)
	}
	if holders := holderIDs(got); !slices.Equal(holders, []ids.UUID{e.Rep3}) {
		t.Errorf("hidden_contact_holders = %v, want Rep3 once: not the departed Rep2, not the owner of an archived contact", holders)
	}
	wire, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, withheld := range []string{"Hidden Sponsor", "Hidden Engineer", "Orphaned Champion", "Archived Lead"} {
		if strings.Contains(string(wire), withheld) {
			t.Errorf("the company carries the withheld contact name %q", withheld)
		}
	}

	page, _, err := e.Contacts.ListCompanies(rep, contacts.ListCompaniesInput{})
	if err != nil {
		t.Fatal(err)
	}
	listed := map[ids.UUID][]ids.UUID{}
	for _, row := range page {
		listed[ids.UUID(row.Id)] = holderIDs(row)
		if row.HiddenContactHolders == nil {
			t.Errorf("list row %s has no hidden_contact_holders beside its contact_count", row.DisplayName)
		}
	}
	if !slices.Equal(listed[company], []ids.UUID{e.Rep3}) {
		t.Errorf("list holders for the shared account = %v, want Rep3, as the single read says", listed[company])
	}
	if len(listed[empty]) != 0 {
		t.Errorf("list holders for an account with only visible contacts = %v, want none", listed[empty])
	}
}

func TestAnAllRowsReaderIsToldOnlyOfTheCapturePrivateHolders(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	other := e.SeedCompany(t, "Other Account", &e.Rep1)
	seedEmployee(t, e, company, "Rep3's Shared Contact", e.Rep3, false)
	seedEmployee(t, e, company, "Rep2's Private Contact", e.Rep2, true)
	seedEmployee(t, e, other, "Rep3's Other Shared Contact", e.Rep3, false)

	got, err := e.Contacts.GetCompany(e.Admin(), companyIDOf(company), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if holders := holderIDs(got); !slices.Equal(holders, []ids.UUID{e.Rep2}) {
		t.Errorf("hidden_contact_holders = %v, want Rep2 only: Rep3's contact is shared", holders)
	}
	none, err := e.Contacts.GetCompany(e.Admin(), companyIDOf(other), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if none.HiddenContactHolders == nil || len(*none.HiddenContactHolders) != 0 {
		t.Errorf("hidden_contact_holders = %v, want present and empty", none.HiddenContactHolders)
	}
}

func TestHolderNamesAreAbsentWhereTheCountIs(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	seedEmployee(t, e, company, "Hidden Sponsor", e.Rep3, true)

	noEdges := principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"contact": {Read: true}, "company": {Read: true},
		},
		RowScope: principal.RowScopeTeam,
	}
	got, err := e.Contacts.GetCompany(e.As(e.Rep1, []ids.UUID{e.Team1}, noEdges), companyIDOf(company), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContactCount != nil || got.HiddenContactHolders != nil {
		t.Errorf("without relationship read: contact_count %v, holders %v; want both absent", got.ContactCount, got.HiddenContactHolders)
	}
}

// An agent is never shown a restricted contact, so it is not told one exists.
// The holder is not named and the hidden count leaves it out. A human reader
// of the same account is told. Rep2's ordinary capture-private contact is
// reported to both, so the agent's silence comes from the restriction.
func TestAnAgentIsNotToldAboutARestrictedContactOrItsHolder(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	seedEmployee(t, e, company, "Visible Buyer", e.Rep1, false)
	restricted := seedEmployee(t, e, company, "Restricted Sponsor", e.Rep3, true)
	seedEmployee(t, e, company, "Hidden Engineer", e.Rep2, true)
	e.WsExec(t, `INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'processing_restriction', 'test', 'human:test', 'subject')`, restricted)

	base := principal.WithWorkspaceID(context.Background(), e.WS)
	base = principal.WithCorrelationID(base, ids.NewV7())
	agent := principal.WithActor(base, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test", SeatType: principal.SeatFull,
		UserID: e.Rep1, OnBehalfOf: e.Rep1, Permissions: hiddenReaderPerms,
		TeamIDs: []ids.UUID{e.Team1},
	})
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, hiddenReaderPerms)

	want := func(label string, ctx context.Context, holders []ids.UUID, count int) {
		t.Helper()
		got, err := e.Contacts.GetCompany(ctx, companyIDOf(company), storekit.LiveOnly)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		h := holderIDs(got)
		slices.SortFunc(h, byString)
		slices.SortFunc(holders, byString)
		if !slices.Equal(h, holders) {
			t.Errorf("%s: hidden_contact_holders = %v, want %v", label, h, holders)
		}
		var hidden storekit.HiddenRows
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) (err error) {
			hidden, err = contacts.HiddenCompanyContactsTx(ctx, tx, companyIDOf(company))
			return err
		}); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if hidden.Count != count {
			t.Errorf("%s: hidden count = %d, want %d", label, hidden.Count, count)
		}
	}
	want("agent", agent, []ids.UUID{e.Rep2}, 1)
	want("human", human, []ids.UUID{e.Rep2, e.Rep3}, 2)
}

func byString(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) }
