// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Who the reach answer names, and who it leaves out.
//
// The count is a derived fact, shown to everyone who may read the company so a
// rep knows whom to ask. It is still a fact about a colleague, so a colleague
// may withhold it.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// withholdReach is the member's own answer, as their settings tab records it.
func (e *dedupeEnv) withholdReach(ctx context.Context, t *testing.T, user ids.UUID) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO linkedin_account (user_id, reach_visible_to_team)
			VALUES ($1, false)
			ON CONFLICT (user_id) DO UPDATE SET reach_visible_to_team = false`, user)
		return err
	}); err != nil {
		t.Fatalf("withholding %s's reach: %v", user, err)
	}
}

// accountRows is how many account rows a member has, so the default case can
// prove it is testing a member who never answered.
func (e *dedupeEnv) accountRows(ctx context.Context, t *testing.T, user ids.UUID) int {
	t.Helper()
	var rows int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM linkedin_account WHERE user_id = $1`, user).Scan(&rows)
	}); err != nil {
		t.Fatalf("counting %s's account rows: %v", user, err)
	}
	return rows
}

func TestReachNamesTheColleaguesToAskHighestFirst(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company := e.seedAcmeCompany(t)
	e.seedReach(ctx, t, e.rep, company, "Abbas Fawaz")
	e.seedReach(ctx, t, e.otherRep, company, "Bea Hoffmann")
	e.seedReach(ctx, t, e.otherRep, company, "Cem Yilmaz")

	got, err := e.store.CompanyReach(ctx, company)
	if err != nil {
		t.Fatalf("CompanyReach: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("the answer names %d colleagues, want both: %+v", len(got), got)
	}
	// Highest first, which is the order the question is asked in: who can best
	// get us in.
	if got[0].UserID != e.otherRep || got[0].Connections != 2 {
		t.Errorf("first is %+v, want the colleague with two connections", got[0])
	}
	if got[1].UserID != e.rep || got[1].Connections != 1 {
		t.Errorf("second is %+v, want the colleague with one", got[1])
	}
	if got[0].DisplayName == "" || got[1].DisplayName == "" {
		t.Error("a colleague reached the answer unnamed, so there is nobody to ask")
	}
}

func TestReachLeavesOutAColleagueWhoWithheldIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company := e.seedAcmeCompany(t)
	e.seedReach(ctx, t, e.rep, company, "Abbas Fawaz")
	e.seedReach(ctx, t, e.otherRep, company, "Bea Hoffmann")

	e.withholdReach(ctx, t, e.otherRep)

	got, err := e.store.CompanyReach(ctx, company)
	if err != nil {
		t.Fatalf("CompanyReach: %v", err)
	}
	if len(got) != 1 || got[0].UserID != e.rep {
		t.Fatalf("the answer is %+v, want only the colleague who did not withhold it", got)
	}
	// Absent rather than reported as zero. Zero says they have no reach, which
	// is a different statement and not theirs to have made.
	for _, c := range got {
		if c.UserID == e.otherRep {
			t.Errorf("the colleague who withheld their reach is reported as %d", c.Connections)
		}
	}
}

// A member who has never opened the setting has no row at all, and the default
// is to be listed. Read from Go's zero value this would be withheld, which
// would empty the answer for most of a workspace.
func TestReachListsAColleagueWhoNeverAnsweredTheSetting(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company := e.seedAcmeCompany(t)
	e.seedReach(ctx, t, e.rep, company, "Abbas Fawaz")

	if rows := e.accountRows(ctx, t, e.rep); rows != 0 {
		t.Fatalf("this case needs a member with no account row, and the seed left %d", rows)
	}

	got, err := e.store.CompanyReach(ctx, company)
	if err != nil {
		t.Fatalf("CompanyReach: %v", err)
	}
	if len(got) != 1 || got[0].UserID != e.rep {
		t.Fatalf("the answer is %+v, want the colleague who never answered the setting", got)
	}
}

// The answer says which functions a colleague's connections work in, and only
// where two or more do: one would name the contact behind the title.
func TestReachNamesTheFunctionsTwoOrMoreConnectionsWorkIn(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company := e.seedAcmeCompany(t)
	e.seedTitledReach(ctx, t, e.rep, company, "Abbas Fawaz", "Head of IT")
	e.seedTitledReach(ctx, t, e.rep, company, "Bea Hoffmann", "IT-Leiterin")
	e.seedTitledReach(ctx, t, e.rep, company, "Cem Yilmaz", "Head of HR")
	e.seedTitledReach(ctx, t, e.rep, company, "Dana Ilic", "Director")

	got, err := e.store.CompanyReach(ctx, company)
	if err != nil {
		t.Fatalf("CompanyReach: %v", err)
	}
	if len(got) != 1 || got[0].Connections != 4 {
		t.Fatalf("the answer is %+v, want one colleague with four connections", got)
	}
	want := []crmcontracts.LinkedInReachFunctionCount{{Function: crmcontracts.LinkedInReachFunctionIt, Connections: 2}}
	if len(got[0].Functions) != 1 || got[0].Functions[0] != want[0] {
		t.Errorf("functions are %+v, want %+v: HR has one connection and Director names no function",
			got[0].Functions, want)
	}
}
