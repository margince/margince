// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A record's history includes the LINKS made, changed and removed on it, and it
// includes them from BOTH ends: an employment appears on the person and on the
// company, a co-sell appears on both organizations.
//
// The read is where an edge's two disclosures meet. An edge names two records,
// so a line naming the other end has to answer to that record's own gates — the
// caller's row scope, and the erasure that certified its data gone — and it has
// to answer to them IN THE KEYSET, or a page of twenty comes back with three
// rows and a cursor that counts what the caller may not see. Every case below is
// one of those two properties or the paging that depends on them.

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// linkEmployment links a person to an organization through the people store's
// own write path — the one that stamps the audit row this read projects. A
// hand-rolled INSERT would prove nothing about production: the image, the
// action and the entity_type on that row are exactly what is under test.
func linkEmployment(t *testing.T, e *Env, person, org ids.UUID, role string) ids.UUID {
	t.Helper()
	rel, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind:           "employment",
		PersonID:       ptr(PersonIDOf(person)),
		OrganizationID: ptr(ids.From[ids.OrganizationKind](org)),
		Role:           &role,
		Source:         "manual",
	})
	if err != nil {
		t.Fatalf("linking the person to the organization: %v", err)
	}
	return rel.ID
}

// seedCoSell links two organizations as co-sellers: the kind whose two ends sit
// in DIFFERENT columns (organization_id and counterparty_org_id), which is the
// case a single-anchor read drops silently.
func seedCoSell(t *testing.T, e *Env, org, counterparty ids.UUID) ids.UUID {
	t.Helper()
	rel, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind:              "co_sell_with",
		OrganizationID:    ptr(ids.From[ids.OrganizationKind](org)),
		CounterpartyOrgID: ptr(ids.From[ids.OrganizationKind](counterparty)),
		Source:            "manual",
	})
	if err != nil {
		t.Fatalf("linking the two organizations: %v", err)
	}
	return rel.ID
}

func ptr[T any](v T) *T { return &v }

// edgeHistoryOf reads one record's history as the workspace admin and answers
// the summary lines, newest first.
func edgeHistoryOf(t *testing.T, e *Env, entityType string, id ids.UUID, limit *int) privacy.RecordHistoryPage {
	t.Helper()
	page, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: entityType, EntityID: id, Limit: limit,
	})
	if err != nil {
		t.Fatalf("reading %s history: %v", entityType, err)
	}
	return page
}

// summaries is the page's lines, which is where the wording under test lands.
func summaries(page privacy.RecordHistoryPage) []string {
	out := make([]string, 0, len(page.Entries))
	for _, entry := range page.Entries {
		out = append(out, entry.Summary)
	}
	return out
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

// seedScrubTombstone stamps an erase tombstone on a record's own audit spine.
//
// Raw rather than through the eraser, and deliberately: the eraser also ARCHIVES
// its subject, and an archived endpoint is withheld by the edge's endpoint
// conjunction whatever the tombstone says. A tombstone on a LIVE record is the
// only seed that isolates the boundary itself, and no product path writes one
// for an organization — so this is the fixture that makes the erasure filter
// falsifiable rather than merely present.
func seedScrubTombstone(t *testing.T, e *Env, entityType string, id ids.UUID, at time.Time) {
	t.Helper()
	ctx := principal.WithWorkspaceID(t.Context(), e.WS)
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, occurred_at)
			VALUES ($1, 'system', 'system', 'erase', $2, $3, $4)`,
			ids.NewV7(), entityType, id, at)
		return err
	})
	if err != nil {
		t.Fatalf("seeding a %s scrub tombstone: %v", entityType, err)
	}
}

func TestEdgeHistoryShowsAnEmploymentOnBothEnds(t *testing.T) {
	e := Setup(t)
	person := e.SeedPerson(t, "Ada Employed", nil)
	org := e.SeedOrg(t, "Employer GmbH", nil)
	linkEmployment(t, e, person, org, "cto")

	// The person's own history names the COMPANY, because the company is the
	// other end. Naming the person there would say nothing a reader of the
	// person's page does not already know.
	onPerson := summaries(edgeHistoryOf(t, e, "person", person, nil))
	if !containsLine(onPerson, "Rep linked Employer GmbH as cto") {
		t.Errorf("the person's history = %q, want a line naming the company the link was made to", onPerson)
	}

	// The SAME edge, from the other end, naming the person.
	onOrg := summaries(edgeHistoryOf(t, e, "organization", org, nil))
	if !containsLine(onOrg, "Rep linked Ada Employed as cto") {
		t.Errorf("the organization's history = %q, want the same link naming the person", onOrg)
	}
}

func TestEdgeHistoryShowsACoSellEdgeOnBothOrganizations(t *testing.T) {
	e := Setup(t)
	ours := e.SeedOrg(t, "Our Side GmbH", nil)
	theirs := e.SeedOrg(t, "Their Side AG", nil)
	seedCoSell(t, e, ours, theirs)

	// One end matches organization_id, the other counterparty_org_id. A read
	// that knew only about the anchor column would show this edge on exactly one
	// of the two companies, and nothing would say which.
	if lines := summaries(edgeHistoryOf(t, e, "organization", ours, nil)); !containsLine(lines, "Rep linked Their Side AG as co_sell_with") {
		t.Errorf("organization_id end = %q, want the co-sell naming the counterparty", lines)
	}
	if lines := summaries(edgeHistoryOf(t, e, "organization", theirs, nil)); !containsLine(lines, "Rep linked Our Side GmbH as co_sell_with") {
		t.Errorf("counterparty_org_id end = %q, want the co-sell naming the other company", lines)
	}
}

func TestEdgeHistoryWithholdsAnEdgeWhoseOtherEndIsInvisible(t *testing.T) {
	e := Setup(t)
	person := e.SeedPerson(t, "Ada Employed", nil)
	org := e.SeedOrg(t, "Secret Holdings GmbH", nil)
	linkEmployment(t, e, person, org, "cto")
	// Captured privately by Rep1: capture privacy does not yield to
	// row_scope=all, so even the admin reading the person cannot see the
	// company.
	e.MakeCapturePrivate(t, "organization", org, e.Rep1)

	page, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "person", EntityID: person,
	})
	// Absent, never refused: a refusal is proof the company exists.
	if err != nil {
		t.Fatalf("the read must SUCCEED and omit the row, not refuse: %v", err)
	}
	if len(page.Entries) == 0 {
		t.Fatal("the person's own create row is missing — the withholding took the whole page")
	}
	for _, line := range summaries(page) {
		if strings.Contains(line, "Secret Holdings") {
			t.Errorf("the invisible company is named on the person's history: %q", line)
		}
	}
}

func TestEdgeHistoryFillsAFullPageDespiteInvisibleEdges(t *testing.T) {
	e := Setup(t)
	person := e.SeedPerson(t, "Ada Employed", nil)
	// Twenty-five of the person's own rows, dated into the past so the three
	// invisible edges below are the NEWEST events and fall inside page one.
	base := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Microsecond)
	for i := range 25 {
		seedRecordAuditRow(t, e, "update", person, "system", "system", nil,
			nil, map[string]any{"title": "t"}, base.Add(time.Duration(i)*time.Minute))
	}
	// Three edges the caller may not see. Filtered AFTER the keyset window they
	// would eat three of the page's twenty slots; left unfiltered they would name
	// three companies the caller cannot read.
	for _, name := range []string{"Hidden One GmbH", "Hidden Two GmbH", "Hidden Three GmbH"} {
		org := e.SeedOrg(t, name, nil)
		linkEmployment(t, e, person, org, "advisor")
		e.MakeCapturePrivate(t, "organization", org, e.Rep1)
	}

	limit := 20
	page := edgeHistoryOf(t, e, "person", person, &limit)
	if len(page.Entries) != limit {
		t.Fatalf("a full page came back with %d of %d rows — the visibility filter is running AFTER the "+
			"keyset window, which also makes has_more count rows the caller may not see", len(page.Entries), limit)
	}
	if !page.HasMore {
		t.Error("has_more = false with more rows behind the cursor")
	}
	for _, line := range summaries(page) {
		if strings.Contains(line, "Hidden") {
			t.Errorf("a company the caller cannot read is named on the page: %q", line)
		}
	}
}

func TestEdgeHistoryStopsAtTheAnchorsOwnScrubTombstone(t *testing.T) {
	e := Setup(t)
	person := e.SeedPerson(t, "Ada Employed", nil)
	org := e.SeedOrg(t, "Employer GmbH", nil)
	linkEmployment(t, e, person, org, "cto")

	// The tombstone lands on the ANCHOR being read, after the link. Everything
	// strictly older is the data the scrub certified gone, edge rows included:
	// the link's image carries the role and the dates.
	seedScrubTombstone(t, e, "organization", org, time.Now().Add(time.Hour).UTC())

	for _, line := range summaries(edgeHistoryOf(t, e, "organization", org, nil)) {
		if strings.Contains(line, "linked Ada Employed") {
			t.Errorf("an edge row older than the anchor's own tombstone was served: %q", line)
		}
	}
}

func TestEdgeHistoryWithholdsAnErasedSubjectsEdgesFromTheOtherEnd(t *testing.T) {
	e := Setup(t)
	person := e.SeedPerson(t, "Selma Subject", nil)
	org := e.SeedOrg(t, "Employer GmbH", nil)
	linkEmployment(t, e, person, org, "cto")

	if err := privacy.NewEraser(e.DB()).ErasePerson(e.Admin(), person, "art-17"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	// The employment image holds the erased subject's role and dates. Left
	// readable on the COMPANY's history it would outlive the certificate that
	// said it was gone.
	for _, line := range summaries(edgeHistoryOf(t, e, "organization", org, nil)) {
		if strings.Contains(line, "Selma Subject") {
			t.Errorf("an erased subject's employment row is still readable on the company: %q", line)
		}
	}
}

func TestEdgeHistoryWithholdsAnEdgeWhoseOtherEndCarriesAScrubTombstone(t *testing.T) {
	e := Setup(t)
	person := e.SeedPerson(t, "Ada Employed", nil)
	org := e.SeedOrg(t, "Employer GmbH", nil)
	linkEmployment(t, e, person, org, "cto")

	// The tombstone is on the OTHER end and that record is still LIVE, so the
	// endpoint conjunction's own archived arm cannot be what withholds the row.
	// This is the erasure filter alone.
	seedScrubTombstone(t, e, "organization", org, time.Now().Add(time.Hour).UTC())

	for _, line := range summaries(edgeHistoryOf(t, e, "person", person, nil)) {
		if strings.Contains(line, "Employer GmbH") {
			t.Errorf("the other end carries a scrub tombstone and its edge row was still served: %q", line)
		}
	}
}

func TestEdgeHistoryStillShowsAnUnlinkedEdge(t *testing.T) {
	e := Setup(t)
	person := e.SeedPerson(t, "Ada Employed", nil)
	org := e.SeedOrg(t, "Employer GmbH", nil)
	edge := linkEmployment(t, e, person, org, "cto")

	if _, err := e.People.UpdateRelationship(e.Admin(), edge, people.UpdateRelationshipInput{
		Role: ptr("coo"),
	}); err != nil {
		t.Fatalf("changing the link's role: %v", err)
	}
	if _, err := e.People.ArchiveRelationship(e.Admin(), edge, nil); err != nil {
		t.Fatalf("unlinking: %v", err)
	}

	// An unlink ARCHIVES the relationship row, and the unlink is the event a
	// reader most wants to see — so the lookup must not require a live edge.
	lines := summaries(edgeHistoryOf(t, e, "person", person, nil))
	for _, want := range []string{
		"Rep linked Employer GmbH as cto",
		"Rep changed Employer GmbH's role",
		"Rep unlinked Employer GmbH",
	} {
		if !containsLine(lines, want) {
			t.Errorf("history = %q, want a %q line", lines, want)
		}
	}
}

func TestEdgeHistoryInterleavesWithTheRecordsOwnRowsAcrossPages(t *testing.T) {
	e := Setup(t)
	person := e.SeedPerson(t, "Ada Employed", nil)
	org := e.SeedOrg(t, "Employer GmbH", nil)
	edge := linkEmployment(t, e, person, org, "cto")
	if _, err := e.People.ArchiveRelationship(e.Admin(), edge, nil); err != nil {
		t.Fatalf("unlinking: %v", err)
	}
	// Rows on either side of the edge's own two, so the boundary between the two
	// union branches falls inside a page rather than between them.
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	for i := range 4 {
		seedRecordAuditRow(t, e, "update", person, "system", "system", nil,
			nil, map[string]any{"title": "t"}, base.Add(time.Duration(i)*time.Minute))
	}

	// Two pages of three over the whole timeline, keyset-walked.
	limit := 3
	first := edgeHistoryOf(t, e, "person", person, &limit)
	if len(first.Entries) != limit || !first.HasMore {
		t.Fatalf("page one: %d rows, has_more=%v", len(first.Entries), first.HasMore)
	}
	second, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "person", EntityID: person, Limit: &limit, Cursor: &first.NextCursor,
	})
	if err != nil {
		t.Fatalf("page two: %v", err)
	}

	seen := map[ids.UUID]bool{}
	var order []time.Time
	for _, entry := range append(first.Entries, second.Entries...) {
		if seen[entry.ID] {
			t.Errorf("row %s appears on both pages — the keyset repeats across the union boundary", entry.ID)
		}
		seen[entry.ID] = true
		order = append(order, entry.OccurredAt)
	}
	for i := 1; i < len(order); i++ {
		if order[i].After(order[i-1]) {
			t.Fatalf("the union is not ordered newest-first at position %d: %v then %v", i, order[i-1], order[i])
		}
	}
	// The person's own create, the four seeded updates and the edge's two rows
	// are seven; six of them fit the two pages, and none may be an edge row's
	// twin or a record row dropped by the union.
	if len(seen) != 2*limit {
		t.Fatalf("two pages of %d yielded %d distinct rows", limit, len(seen))
	}
}
