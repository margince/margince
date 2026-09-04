// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A field mask meeting the report engine: a rep whose role masks another
// team's deal amount must not read those amounts back out of a SUM, and the
// drill-through that explains the sum must show exactly the rows inside it.
// Masked rows are EXCLUDED — from the aggregate and the explanation alike —
// and the envelope carries the withheld count as excluded_by_permission, so
// a smaller total reads as governed rather than as missing data.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maskedDealReader mints a rep on team1 whose role masks deal.amount_minor
// under the given condition — deal update is granted because the write arm is
// what lifts an outside_write_authority mask on the rep's own rows.
func (e *forecastEnv) maskedDealReader(cond principal.MaskCondition) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		TeamIDs: []ids.UUID{e.Team1},
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal":                  {Read: true, Update: true},
				"installation_settings": {Read: true},
			},
			RowScope:   principal.RowScopeTeam,
			FieldMasks: []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: cond}},
		},
	})
}

func sumAmountBody() string {
	return `{"group_by":["currency"],"aggregates":[{"fn":"sum","field":"amount_minor","as":"total"}]}`
}

//craft:ignore naked-any the wire rows decode to map[string]any (decodeWire's UseNumber shape); this is the one coercion seam
func asInt(t *testing.T, v any) int64 {
	t.Helper()
	n, ok := v.(json.Number)
	if !ok {
		t.Fatalf("value %v (%T) is not a number", v, v)
	}
	i, err := n.Int64()
	if err != nil {
		t.Fatal(err)
	}
	return i
}

// A deal outside the reader's POPULATION is absent, and is not reported as
// something permission withheld.
//
// Both exclusions make a total smaller and they mean opposite things.
// `excluded_by_permission` says "these rows are yours to see and their number
// is not" — a governed answer a reader can act on by asking for authority.
// Population says the rows were never this reader's to measure, and counting
// them there tells a rep that data was kept from them when their own pipeline
// is simply their own pipeline. A caller cannot tell the two apart from a
// smaller number.
//
// This case previously proved that `MaskOutsideWriteAuthority` kept a
// colleague's amount out of the aggregate. It cannot any more, and that is the
// point rather than a loss: a report's population is resolved from the same
// row scope that decides writability, so every row a report measures is one
// this reader could write, and that mask condition has nothing left to catch
// inside one. The mask is still proved on the always-condition below, which
// does not depend on authority over the row.
func TestADealOutsideThePopulationIsNotAPermissionExclusion(t *testing.T) {
	e := setupForecast(t)
	own := int64(100_000)
	other := int64(250_000)
	mine := e.seedOpenDeal(t, "Mine", 20, &e.Rep1, &own, nil)
	theirs := e.seedOpenDeal(t, "Theirs", 20, &e.Rep3, &other, nil)

	rep := e.maskedDealReader(principal.MaskOutsideWriteAuthority)
	result := e.runReport(rep, t, "open-deals-per-company", sumAmountBody())
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %v, want the one EUR group", result.Rows)
	}
	if got := asInt(t, result.Rows[0]["total"]); got != own {
		t.Errorf("masked rep's sum = %d, want only their own %d — the masked amount leaked into the aggregate", got, own)
	}
	if result.ExcludedByPermission != nil && *result.ExcludedByPermission != 0 {
		t.Errorf("excluded_by_permission = %d, want none — the colleague's deal was outside "+
			"this reader's population, not withheld from it, and reporting it as withheld "+
			"tells a rep data was kept from them when their own pipeline is their own",
			*result.ExcludedByPermission)
	}

	// The drill-through explains the SAME row set: the masked deal is absent
	// and the recomputed aggregate equals the number it explains.
	derivation := e.explainReport(rep, t, "open-deals-per-company", result.DerivationURL)
	if got := asInt(t, derivation.Aggregates["total"]); got != own {
		t.Errorf("derivation total = %d, want %d — the explanation out-saw the report", got, own)
	}
	if derivation.ExcludedByPermission != nil && *derivation.ExcludedByPermission != 0 {
		t.Errorf("derivation excluded_by_permission = %d, want none — same reason as the "+
			"headline, and the two must agree", *derivation.ExcludedByPermission)
	}
	for _, row := range derivation.Rows {
		if row["id"] == theirs.String() {
			t.Errorf("the drill-through printed %s, which is outside this reader's population", theirs)
		}
	}
	if len(derivation.Rows) != 1 || derivation.Rows[0]["id"] != mine.String() {
		t.Errorf("drill-through rows = %v, want exactly the rep's own deal %s", derivation.Rows, mine)
	}

	// An unmasked admin reads the whole workspace, and the envelope says no
	// mask applied (null, not 0).
	all := e.runReport(e.Admin(), t, "open-deals-per-company", sumAmountBody())
	if got := asInt(t, all.Rows[0]["total"]); got != own+other {
		t.Errorf("admin sum = %d, want %d", got, own+other)
	}
	if all.ExcludedByPermission != nil {
		t.Errorf("admin excluded_by_permission = %v, want absent — no mask applied", *all.ExcludedByPermission)
	}
}

func TestAnAlwaysMaskEmptiesTheAggregateAndSaysSo(t *testing.T) {
	e := setupForecast(t)
	own := int64(100_000)
	other := int64(250_000)
	e.seedOpenDeal(t, "Mine", 20, &e.Rep1, &own, nil)
	e.seedOpenDeal(t, "Theirs", 20, &e.Rep3, &other, nil)

	rep := e.maskedDealReader(principal.MaskAlways)
	result := e.runReport(rep, t, "open-deals-per-company", sumAmountBody())
	if len(result.Rows) != 0 {
		t.Errorf("rows = %v, want none — every row's amount is withheld", result.Rows)
	}
	if result.ExcludedByPermission == nil || *result.ExcludedByPermission != 1 {
		t.Errorf("excluded_by_permission = %v, want the 1 row in this reader's population — the colleague's deal is outside it, so it is not a row permission withheld", result.ExcludedByPermission)
	}
}

// Write authority is the object's update verb AND the row arm. A rep whose
// role lost deal.update still OWNS rows; the mask must not lift on them —
// otherwise revoking the verb would WIDEN what the rep reads out of a sum.
func TestARevokedUpdateVerbKeepsTheMaskOnTheRepsOwnRows(t *testing.T) {
	e := setupForecast(t)
	own := int64(100_000)
	e.seedOpenDeal(t, "Mine", 20, &e.Rep1, &own, nil)

	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	rep := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		TeamIDs: []ids.UUID{e.Team1},
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal":                  {Read: true},
				"installation_settings": {Read: true},
			},
			RowScope:   principal.RowScopeTeam,
			FieldMasks: []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority}},
		},
	})
	result := e.runReport(rep, t, "open-deals-per-company", sumAmountBody())
	if len(result.Rows) != 0 {
		t.Errorf("rows = %v, want none — without the update verb the rep holds write authority over no row", result.Rows)
	}
	if result.ExcludedByPermission == nil || *result.ExcludedByPermission != 1 {
		t.Errorf("excluded_by_permission = %v, want 1", result.ExcludedByPermission)
	}
}
