// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a company merge does to the projects the two companies carry, and what
// it refuses to do.
//
// Its own file because the refusal is a rule of its own — two companies each
// carrying live work do not combine — and because it is read from two ends: the
// engine that refuses the write, and the card that decides whether to offer the
// merge at all. Those have to agree, and the last test here is where they meet.

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PROJ-LIFE-4: a project's anchor is NOT NULL … ON DELETE RESTRICT, so it
// cannot stay behind on a dissolved company. Leaving it is not cosmetic —
// the deals move to the survivor and the same-company trigger then refuses
// their NEXT edit, which is how a healthy deal becomes un-editable over a
// mismatch nobody made.
func TestMergingCompaniesReAnchorsTheProjectWithItsDeals(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	source := e.SeedOrg(t, "BAER Pharma GmbH", nil)
	target := e.SeedOrg(t, "BAER Pharma", nil)
	p := seedProject(e.Admin(), t, e, "ERP replacement", source, nil)

	sourceID := orgIDOf(source)
	d, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Phase one", PipelineID: pipeline, StageID: open,
		OrganizationID: &sourceID, ProjectID: &p.ID, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.People.MergeOrganization(e.Admin(), sourceID, orgIDOf(target)); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM project WHERE id = $1 AND organization_id = $2`,
		p.ID, target); n != 1 {
		t.Error("the project stayed on the merged-away company")
	}
	// The proof that the re-anchor is load-bearing: editing the deal after
	// the merge must still work. Before the fix this raised the
	// same-company trigger.
	name := "Phase one, renamed"
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](ids.UUID(d.Id)),
		deals.UpdateDealInput{Name: &name}); err != nil {
		t.Errorf("the merged deal became un-editable: %v", err)
	}
}

// PROJ-LIFE-4's ask: two companies that each hold live bodies of work may,
// once merged, be running the same one twice or two different ones — and
// nothing in the data says which. The merge stops and names them rather
// than leaving a human to find the duplicates later.
func TestMergingTwoCompaniesThatBothCarryProjectsIsRefused(t *testing.T) {
	e := Setup(t)
	source := e.SeedOrg(t, "BAER Pharma GmbH", nil)
	target := e.SeedOrg(t, "BAER Pharma", nil)
	seedProject(e.Admin(), t, e, "ERP replacement", source, nil)
	kept := seedProject(e.Admin(), t, e, "Validation", target, nil)

	_, err := e.People.MergeOrganization(e.Admin(), orgIDOf(source), orgIDOf(target))
	var both *people.BothCompaniesCarryProjectsError
	if !errors.As(err, &both) {
		t.Fatalf("merging two project-carrying companies produced %v, want a refusal", err)
	}
	if len(both.Source) != 1 || len(both.Target) != 1 {
		t.Errorf("the refusal named %v and %v, want one project from each side", both.Source, both.Target)
	}

	// Refusing must change nothing: the transaction rolls back whole.
	if n := e.WsCount(t, `SELECT count(*) FROM organization WHERE id = $1 AND archived_at IS NULL`, source); n != 1 {
		t.Error("the refused merge still archived the source company")
	}

	// And it is actionable: archive one side, then the merge proceeds.
	if _, err := e.Projects.ArchiveProject(e.Admin(), kept.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.People.MergeOrganization(e.Admin(), orgIDOf(source), orgIDOf(target)); err != nil {
		t.Errorf("archiving one side did not unblock the merge: %v", err)
	}
}

// The merge refusal reads both sides, and it must block on work the caller
// does not own — otherwise a rep quietly combines two companies whose projects
// another team is delivering.
//
// It names them too, and that is not a leak: every seat holding the object
// grant reads every project (platform/auth tableclass.go), and a project
// cannot be capture-private at all: project_visibility_check admits only
// 'workspace'. A refusal that counted these without
// naming them would be withholding from a caller who can open both records
// on the project page a moment later — precision, not silence, is the point.
func TestTheMergeRefusalBlocksAndNamesProjectsTheCallerDoesNotOwn(t *testing.T) {
	e := Setup(t)
	// The merging rep owns both companies (a merge is a write, and an own-scope
	// seat only writes what it owns), but neither project under them.
	source := e.SeedOrg(t, "Helios GmbH", &e.Rep3)
	target := e.SeedOrg(t, "Helios AG", &e.Rep3)
	seedProject(e.Admin(), t, e, "Another team's migration", source, &e.Rep1)
	seedProject(e.Admin(), t, e, "Another team's rollout", target, &e.Rep2)

	outsider := e.As(e.Rep3, []ids.UUID{e.Team2}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"organization":          {Read: true, Update: true, Delete: true},
			"project":               {Read: true},
			"person":                {Read: true, Update: true},
			"installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	})

	_, err := e.People.MergeOrganization(outsider, orgIDOf(source), orgIDOf(target))
	var both *people.BothCompaniesCarryProjectsError
	if !errors.As(err, &both) {
		t.Fatalf("the merge produced %v, want a refusal — another team's work still blocks it", err)
	}
	if both.SourceCount != 1 || both.TargetCount != 1 {
		t.Errorf("counted %d and %d live projects, want one each", both.SourceCount, both.TargetCount)
	}
	// Named, so the rep can act on the refusal instead of hunting for what
	// blocked it. A count with no name is an instruction to guess.
	for _, name := range []string{"Another team's migration", "Another team's rollout"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("the refusal does not name %q, so the caller cannot act on it: %v", name, err)
		}
	}
}

// The same refusal, seen by someone who owns both projects: it names them,
// because for this caller they are not a secret — the point of scoping the
// naming is precision, not silence.
func TestTheMergeRefusalNamesTheProjectsTheCallerCanSee(t *testing.T) {
	e := Setup(t)
	source := e.SeedOrg(t, "Vector Ltd", &e.Rep1)
	target := e.SeedOrg(t, "Vector Limited", &e.Rep1)
	seedProject(e.Admin(), t, e, "Mine A", source, &e.Rep1)
	seedProject(e.Admin(), t, e, "Mine B", target, &e.Rep1)

	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"organization":          {Read: true, Update: true, Delete: true},
			"project":               {Read: true},
			"person":                {Read: true, Update: true},
			"installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	})

	_, err := e.People.MergeOrganization(owner, orgIDOf(source), orgIDOf(target))
	var both *people.BothCompaniesCarryProjectsError
	if !errors.As(err, &both) {
		t.Fatalf("the merge produced %v, want a refusal", err)
	}
	if len(both.Source) != 1 || both.Source[0] != "Mine A" {
		t.Errorf("source projects = %v, want the one this caller owns", both.Source)
	}
	if len(both.Target) != 1 || both.Target[0] != "Mine B" {
		t.Errorf("target projects = %v, want the one this caller owns", both.Target)
	}
}

// The other half of the same rule: naming a project is a read of it, so a
// caller who never held project.read is refused the merge WITHOUT the names.
//
// The merge entry point gates on organization.update alone, so this seat is a
// real one — a rep who may tidy up duplicate companies and has no business
// with the delivery side. Row scope does not narrow a project any more, but
// the object grant is a separate gate, and the counts are what tells this
// caller the work exists without telling them what it is called.
func TestTheMergeRefusalWithholdsProjectNamesFromACallerWithoutTheGrant(t *testing.T) {
	e := Setup(t)
	source := e.SeedOrg(t, "Kepler GmbH", &e.Rep1)
	target := e.SeedOrg(t, "Kepler AG", &e.Rep1)
	seedProject(e.Admin(), t, e, "Secret migration", source, &e.Rep1)
	seedProject(e.Admin(), t, e, "Secret rollout", target, &e.Rep1)

	// Everything the merge itself demands, and no project grant at all.
	ungranted := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"organization":          {Read: true, Update: true, Delete: true},
			"person":                {Read: true, Update: true},
			"installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	})

	_, err := e.People.MergeOrganization(ungranted, orgIDOf(source), orgIDOf(target))
	var both *people.BothCompaniesCarryProjectsError
	if !errors.As(err, &both) {
		t.Fatalf("the merge produced %v, want a refusal — work the caller cannot see still blocks it", err)
	}
	// Still refused, and still on the true counts: the decision is unscoped.
	if both.SourceCount != 1 || both.TargetCount != 1 {
		t.Errorf("counted %d and %d live projects, want one each", both.SourceCount, both.TargetCount)
	}
	if len(both.Source) != 0 || len(both.Target) != 0 {
		t.Errorf("the refusal named %v and %v to a caller holding no project grant", both.Source, both.Target)
	}
	// And no name reaches the rendered message either, which is what the
	// handler puts on the wire as the 409 detail.
	for _, name := range []string{"Secret migration", "Secret rollout"} {
		if strings.Contains(err.Error(), name) {
			t.Errorf("the refusal message discloses %q to a caller holding no project grant: %v", name, err)
		}
	}
}

// The CARD and the MERGE agree about who carries projects.
//
// The duplicates lane offers a Merge button on any pair whose two records the
// reader may write. Two companies each holding live work refuse to combine, so
// a data steward with authority over both was handed a button that answered
// 409 every time — the same dead end the authority check was added to remove,
// reached by another path.
//
// Both readings go through one predicate now. This is what holds them
// together: the same two companies that make the merge refuse make the card
// withhold, and archiving one side releases both in the same breath.
func TestTheCardAndTheMergeAgreeOnWhoCarriesProjects(t *testing.T) {
	e := Setup(t)
	source := e.SeedOrg(t, "BAER Pharma GmbH", nil)
	target := e.SeedOrg(t, "BAER Pharma", nil)
	seedProject(e.Admin(), t, e, "ERP replacement", source, nil)
	kept := seedProject(e.Admin(), t, e, "Validation", target, nil)

	carrying, err := e.People.OrganizationsCarryingLiveProjects(e.Admin(),
		[]ids.UUID{source, target})
	if err != nil {
		t.Fatalf("reading which companies carry live projects: %v", err)
	}
	if !carrying[source] || !carrying[target] {
		t.Fatalf("carrying = %+v, want both — these are the two the merge refuses on, "+
			"so a card reading anything else offers a button that answers 409", carrying)
	}

	// The merge refuses on exactly that pair, which is the agreement.
	_, err = e.People.MergeOrganization(e.Admin(), orgIDOf(source), orgIDOf(target))
	var both *people.BothCompaniesCarryProjectsError
	if !errors.As(err, &both) {
		t.Fatalf("the merge produced %v, want the refusal the card is predicting", err)
	}

	// Archive one side and BOTH answers change together. A card still
	// withholding here would be the mirror defect: a merge that would now
	// succeed, offered nowhere.
	if _, err := e.Projects.ArchiveProject(e.Admin(), kept.ID, nil); err != nil {
		t.Fatalf("archiving the target's project: %v", err)
	}
	carrying, err = e.People.OrganizationsCarryingLiveProjects(e.Admin(),
		[]ids.UUID{source, target})
	if err != nil {
		t.Fatalf("re-reading after the archive: %v", err)
	}
	if carrying[target] {
		t.Errorf("the target still reads as carrying live work after its only project " +
			"was archived — an archived project is a grouping already ended")
	}
	if _, err := e.People.MergeOrganization(e.Admin(), orgIDOf(source), orgIDOf(target)); err != nil {
		t.Errorf("the merge the card would now offer was refused: %v", err)
	}
}
