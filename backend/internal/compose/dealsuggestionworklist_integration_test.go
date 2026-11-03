// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A Deal Scout suggestion is its company owner's to decide. It reaches that
// owner's Worklist, the named-member view of them and their team's view. Only
// an unowned company's suggestion waits under Unassigned.

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// scoutWorklist is a Worklist feed over the scout env at its clock.
func (e *scoutEnv) scoutWorklist() *attention.Service {
	return newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return e.now })
}

// suggestedCompanies reads one Worklist view and answers the companies its
// suggestion rows are about and the decisions it counts. The suggestion count
// is the view's own, so a view never says more decisions wait than it read.
func (e *scoutEnv) suggestedCompanies(
	ctx context.Context, t *testing.T, scope string, owner ids.UUID,
) ([]ids.UUID, int) {
	t.Helper()
	page, err := e.scoutWorklist().Worklist(ctx, scope, "all", owner, 100, "")
	if err != nil {
		t.Fatalf("reading the %s worklist: %v", scope, err)
	}
	var companies []ids.UUID
	for _, row := range page.Queue {
		if row.Source == "deal_suggestion" && row.Subject != nil {
			companies = append(companies, ids.UUID(row.Subject.Id))
		}
	}
	decisions := 0
	for _, count := range page.Counts {
		if count.Category == crmcontracts.WorklistCountCategoryDecisions {
			decisions = count.Considered
			if count.MoreAvailable {
				t.Errorf("the %s view counts more decisions than the %d it read", scope, decisions)
			}
		}
	}
	return inIDOrder(companies...), decisions
}

// unownedCompany creates a company and clears the owner its creator was given.
func (e *scoutEnv) unownedCompany(t *testing.T, name string) ids.UUID {
	t.Helper()
	company := e.SeedCompany(t, name, nil)
	if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](company),
		contacts.UpdateCompanyInput{Clear: []string{"owner_id"}}); err != nil {
		t.Fatalf("clearing the owner of %s: %v", name, err)
	}
	return company
}

// raiseFor makes the scout raise one suggestion on each company. Each buyer has
// a name of their own, so no duplicate-contact decision joins the count.
func (e *scoutEnv) raiseFor(t *testing.T, companies ...ids.UUID) {
	t.Helper()
	for i, company := range companies {
		buyer := e.employee(t, fmt.Sprintf("Buyer %d", i+1), company)
		request := e.buyerRequest(t, buyer, e.daysAgo(1), "")
		e.signal(t, "new_opportunity", company, request, e.now, ids.Nil, buyer)
		e.signal(t, "commitment_made", company, request, e.now, ids.Nil)
	}
	if pass := e.pass(t); pass.Raised != len(companies) {
		t.Fatalf("the scout raised %d suggestions, want %d", pass.Raised, len(companies))
	}
}

func inIDOrder(list ...ids.UUID) []ids.UUID {
	out := slices.Clone(list)
	slices.SortFunc(out, func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) })
	return out
}

func TestDealSuggestionsReachTheirCompanyOwnersWorklist(t *testing.T) {
	e := setupScout(t)
	alexs := e.SeedCompany(t, "Northwind Traders", &e.Rep1)
	sams := e.SeedCompany(t, "Contoso Logistics", &e.Rep3)
	nobodys := e.unownedCompany(t, "Fabrikam Freight")
	e.raiseFor(t, alexs, sams, nobodys)
	alex, sam := e.rep(e.Rep1, e.Team1), e.rep(e.Rep3, e.Team2)

	for name, view := range map[string]struct {
		ctx   context.Context
		scope string
		owner ids.UUID
		want  []ids.UUID
	}{
		"Alex's own":             {alex, "mine", ids.Nil, []ids.UUID{alexs}},
		"Sam's own":              {sam, "mine", ids.Nil, []ids.UUID{sams}},
		"an admin who owns none": {e.signalReader(), "mine", ids.Nil, nil},
		"Alex named by an admin": {e.signalReader(), "all", e.Rep1, []ids.UUID{alexs}},
		"Sam named by an admin":  {e.signalReader(), "all", e.Rep3, []ids.UUID{sams}},
		"unassigned":             {e.signalReader(), "unassigned", ids.Nil, []ids.UUID{nobodys}},
		"Alex's team":            {e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminWithSignals), "team", ids.Nil, inIDOrder(alexs, nobodys)},
		"Sam's team":             {e.As(e.Rep3, []ids.UUID{e.Team2}, integration.AdminWithSignals), "team", ids.Nil, inIDOrder(sams, nobodys)},
		"the whole workspace":    {e.signalReader(), "all", ids.Nil, inIDOrder(alexs, sams, nobodys)},
	} {
		got, decisions := e.suggestedCompanies(view.ctx, t, view.scope, view.owner)
		if !slices.Equal(got, view.want) || decisions != len(view.want) {
			t.Errorf("%s view holds suggestions for %v and counts %d decisions, want %v", name, got, decisions, view.want)
		}
	}
}

func TestAnOwnershipChangeReroutesTheSuggestionWithoutAWrite(t *testing.T) {
	e := setupScout(t)
	moving := e.SeedCompany(t, "Northwind Traders", &e.Rep1)
	e.raiseFor(t, moving)
	before := e.onlySuggestion(e.signalReader(), t, moving)

	sam := ids.From[ids.UserKind](e.Rep3)
	if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](moving),
		contacts.UpdateCompanyInput{OwnerID: &sam}); err != nil {
		t.Fatalf("handing the company to Sam: %v", err)
	}
	if got, decisions := e.suggestedCompanies(e.rep(e.Rep1, e.Team1), t, "mine", ids.Nil); len(got) != 0 || decisions != 0 {
		t.Errorf("Alex still holds %v (%d decisions) after the company moved to Sam", got, decisions)
	}
	if got, decisions := e.suggestedCompanies(e.rep(e.Rep3, e.Team2), t, "mine", ids.Nil); !slices.Equal(got, []ids.UUID{moving}) || decisions != 1 {
		t.Errorf("Sam holds %v (%d decisions), want the moved company's suggestion", got, decisions)
	}
	if after := e.onlySuggestion(e.signalReader(), t, moving); after.Version != before.Version {
		t.Fatalf("the suggestion moved from version %d to %d: rerouting it wrote to it", before.Version, after.Version)
	}
}

func TestAnOwnerWhoCannotSeeTheEvidenceIsNotShownTheSuggestion(t *testing.T) {
	e := setupScout(t)
	sams := e.SeedCompany(t, "Contoso Logistics", &e.Rep3)
	request, buyer := e.buyingEmail(t, sams, e.daysAgo(1))
	e.signal(t, "new_opportunity", sams, request, e.now, ids.Nil, buyer)
	e.signal(t, "commitment_made", sams, request, e.now, ids.Nil)
	if pass := e.pass(t); pass.Raised != 1 {
		t.Fatalf("the scout raised %d suggestions, want 1", pass.Raised)
	}
	if got, decisions := e.suggestedCompanies(e.rep(e.Rep3, e.Team2), t, "mine", ids.Nil); !slices.Equal(got, []ids.UUID{sams}) || decisions != 1 {
		t.Fatalf("before the limit Sam holds %v (%d decisions), want his company's suggestion", got, decisions)
	}
	if _, err := e.Activities.SetAudience(e.Admin(), ids.From[ids.ActivityKind](request),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the buyer's request to its participants: %v", err)
	}
	views := map[string]context.Context{"Sam's own": e.rep(e.Rep3, e.Team2), "unassigned": e.signalReader()}
	for _, name := range slices.Sorted(maps.Keys(views)) {
		scope := map[string]string{"Sam's own": "mine", "unassigned": "unassigned"}[name]
		if got, decisions := e.suggestedCompanies(views[name], t, scope, ids.Nil); len(got) != 0 || decisions != 0 {
			t.Errorf("%s view holds %v (%d decisions) though Sam may not read the evidence", name, got, decisions)
		}
	}
}
