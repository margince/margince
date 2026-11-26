// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"bytes"
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A caller's export carries the audit rows of profile fields and facts of the
// companies they may see. It carries none of another rep's private company.
func TestABoundedExportCarriesTheSidecarAuditRowsOfVisibleCompanies(t *testing.T) {
	e := SetupSearch(t)
	f := e.seedExportFixture(t)
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE company SET visibility = 'owner' WHERE id = $1`, f.rep3Company); err != nil {
		t.Fatalf("making the other rep's company capture-private: %v", err)
	}
	seedSidecarAudit := func(company ids.UUID) (profile, fact, removedFact ids.UUID) {
		profile = e.SeedID(t, `INSERT INTO company_profile_field (id, company_id, field, value, source, captured_by)
			VALUES ($1, $2, 'register_vat', 'DE111', 'human', 'human:x')`, company)
		fact = e.SeedID(t, `INSERT INTO company_fact (id, company_id, category, field, value_key, value, source, captured_by)
			VALUES ($1, $2, 'company', 'employee_range', '', '11-50', 'human', 'human:x')`, company)
		removedFact = ids.NewV7()
		for _, row := range []struct {
			entity string
			id     ids.UUID
			action string
			before string
		}{
			{"company_profile_field", profile, "update", `{"value":"old"}`},
			{"company_fact", fact, "update", `{"value":"old"}`},
			{"company_fact", removedFact, "delete", `{"company_id":"` + company.String() + `","field":"founded_year","value":"1998"}`},
		} {
			e.SeedID(t, `INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, before)
				VALUES ($1, 'human', 'human:x', $2, $3, $4, $5::jsonb)`, row.action, row.entity, row.id, row.before)
		}
		return profile, fact, removedFact
	}
	mineProfile, mineFact, mineRemoved := seedSidecarAudit(f.rep1Company)
	theirProfile, theirFact, theirRemoved := seedSidecarAudit(f.rep3Company)

	var buf bytes.Buffer
	if _, err := compose.NewExportWriter(e.Pool).WriteBundle(e.exportRep(e.Rep1, e.Team1), &buf); err != nil {
		t.Fatal(err)
	}
	exported := map[string]bool{}
	for _, id := range CSVColumn(t, BundleEntries(t, buf.Bytes())["audit_log.csv"], "entity_id") {
		exported[id] = true
	}
	for _, want := range []ids.UUID{mineProfile, mineFact, mineRemoved} {
		if !exported[want.String()] {
			t.Errorf("the export dropped the sidecar audit row %s of the caller's company", want)
		}
	}
	for _, hidden := range []ids.UUID{theirProfile, theirFact, theirRemoved} {
		if exported[hidden.String()] {
			t.Errorf("the export carries the sidecar audit row %s of a company the caller may not see", hidden)
		}
	}
}
