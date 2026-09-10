// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An exported row does not name a record the exporter could not open.
//
// A deal is customer identity — every seat of the workspace reads every deal —
// and the organization it points at is not: capture privacy makes a company
// private to the colleague who captured it. So the two halves of one bundle can
// disagree about the same record: the organization member omits a company the
// exporter may not open, while deal.csv names its id in a reference column.
//
// Field masking cannot close that. It asks what this role may see of any row,
// never what THIS reader may see of the row this one names — so these cover
// both answers, because a withholding that also blanks a readable reference is
// a regression wearing a security fix's clothes.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedDealOnAPrivateCompany is one deal, readable by everyone, pointing at a
// company that is capture-private to Rep3.
func seedDealOnAPrivateCompany(t *testing.T, e *SearchEnv) (deal, hidden ids.UUID) {
	t.Helper()
	pipelineID := e.SeedID(t, `INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Sales', true, 0)`)
	stageID := e.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipelineID)
	hidden = e.SeedID(t, `INSERT INTO organization (id, display_name, owner_id, visibility, source, captured_by)
		VALUES ($1, 'Meridian Labs', $2, 'owner', 'manual', 'human:x')`, e.Rep3)
	deal = e.SeedID(t, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, organization_id,
		forecast_category, source, captured_by)
		VALUES ($1, $2, 'Meridian renewal', $3, $4, $5, 'commit', 'manual', 'human:x')`,
		e.Rep1, pipelineID, stageID, hidden)
	return deal, hidden
}

// dealAndCompanyReader holds deal AND organization read at team scope.
//
// Both grants, deliberately. VisibleSubset withholds every id when the OBJECT
// grant is missing, so a reader holding only `deal` blanks the reference
// whatever capture privacy says — and a fixture built on one would pass for the
// wrong reason, proving nothing about the arm this closes.
func (e *SearchEnv) dealAndCompanyReader(user ids.UUID) context.Context {
	actor := principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		TeamIDs: []ids.UUID{e.Team1},
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal":             {Read: true},
				"organization":     {Read: true},
				objInstallSettings: {Read: true},
			},
			RowScope: principal.RowScopeTeam,
		},
	}
	return principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), actor)
}

func TestAFilteredExportWithholdsACompanyTheExporterCannotOpen(t *testing.T) {
	e := SetupSearch(t)
	deal, hidden := seedDealOnAPrivateCompany(t, e)

	ctx := e.dealAndCompanyReader(e.Rep1)
	result, err := compose.NewFilteredExportWriter(e.Pool).WriteFiltered(
		ctx, dealEngine(ctx, t, e), storekit.Predicate{Field: "forecast_category", Op: "eq", Value: "commit"}, "csv",
	)
	if err != nil {
		t.Fatalf("filtered export: %v", err)
	}

	// The DEAL is exported — it is identity and this reader may read it. Only
	// what it names is withheld, so a test that saw no rows would be proving
	// the wrong thing.
	ids := CSVColumn(t, result.Body, "id")
	if len(ids) != 1 || ids[0] != deal.String() {
		t.Fatalf("exported ids = %v, want the one visible deal %s", ids, deal)
	}

	for _, got := range CSVColumn(t, result.Body, "organization_id") {
		if got == hidden.String() {
			t.Errorf("the export names organization %s, which this exporter's own organization read would refuse — the id alone is an existence oracle over a capture-private company", hidden)
		}
		if got != "" {
			t.Errorf("organization_id = %q, want empty for a company this exporter cannot open", got)
		}
	}
}

// And the reader who CAN open it still gets it, so the rule withholds from a
// reader rather than emptying the column for everyone.
func TestAFilteredExportKeepsACompanyTheExporterCanOpen(t *testing.T) {
	e := SetupSearch(t)
	_, hidden := seedDealOnAPrivateCompany(t, e)

	ctx := e.dealAndCompanyReader(e.Rep3)
	result, err := compose.NewFilteredExportWriter(e.Pool).WriteFiltered(
		ctx, dealEngine(ctx, t, e), storekit.Predicate{Field: "forecast_category", Op: "eq", Value: "commit"}, "csv",
	)
	if err != nil {
		t.Fatalf("filtered export: %v", err)
	}
	named := CSVColumn(t, result.Body, "organization_id")
	if len(named) != 1 || named[0] != hidden.String() {
		t.Errorf("organization_id = %v, want %s — the company is private TO this reader, so the export owes them the reference", named, hidden)
	}
}
