// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const keyNamedSource = "pipedrive"

func keyNameRepairer(e *Env) context.Context {
	return principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:deal_key_names")
}

// keyNamedFixture is one workspace's pipeline and open stage, so every deal a
// test imports lands where an import would put it.
type keyNamedFixture struct {
	e        *Env
	pipeline ids.PipelineID
	open     ids.StageID
}

func newKeyNamedFixture(t *testing.T) keyNamedFixture {
	t.Helper()
	e := Setup(t)
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	return keyNamedFixture{e: e, pipeline: pipeline, open: open}
}

// importDeal writes a deal the way an import leaves one: named by its source
// key, stamped with its source system.
func (f keyNamedFixture) importDeal(t *testing.T, key string, company *ids.CompanyID) ids.DealID {
	t.Helper()
	system := keyNamedSource
	d, err := f.e.Deals.CreateDeal(f.e.Admin(), deals.CreateDealInput{
		Name: key, PipelineID: f.pipeline, StageID: f.open,
		CompanyID: company, Source: "manual", SourceSystem: &system,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.DealKind](ids.UUID(d.Id))
}

func (f keyNamedFixture) company(t *testing.T, name string) *ids.CompanyID {
	t.Helper()
	id := ids.From[ids.CompanyKind](f.e.SeedCompany(t, name, nil))
	return &id
}

func (f keyNamedFixture) repair(t *testing.T, apply bool, entries ...deals.KeyNamedDeal) []deals.KeyNameResult {
	t.Helper()
	results, err := f.e.Deals.RenameKeyNamedDeals(keyNameRepairer(f.e), entries, apply)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(entries) {
		t.Fatalf("%d results for %d export rows", len(results), len(entries))
	}
	return results
}

func (f keyNamedFixture) nameOf(t *testing.T, id ids.DealID) string {
	t.Helper()
	return f.e.WsScalar(t, `SELECT name FROM deal WHERE id = $1`, id)
}

func (f keyNamedFixture) updateAudits(t *testing.T, id ids.DealID) int {
	t.Helper()
	return f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'update'`, id)
}

func exportRow(key, title string) deals.KeyNamedDeal {
	return deals.KeyNamedDeal{SourceSystem: keyNamedSource, SourceKey: key, SourceTitle: title}
}

func TestADryRunReportsTheRenameAndWritesNothing(t *testing.T) {
	f := newKeyNamedFixture(t)
	id := f.importDeal(t, "acme-q3", f.company(t, "Acme"))

	got := f.repair(t, false, exportRow("acme-q3", "Acme Q3 renewal"))[0]

	if got.Outcome != deals.KeyNameWouldRename || got.To != "Acme Q3 renewal" || got.DealID != id {
		t.Errorf("dry run = %+v, want would-rename of %s to the title", got, id)
	}
	if name := f.nameOf(t, id); name != "acme-q3" {
		t.Errorf("a dry run renamed the deal to %q", name)
	}
	if n := f.updateAudits(t, id); n != 0 {
		t.Errorf("a dry run wrote %d update audit rows", n)
	}
}

func TestApplyingRenamesToTheSourceTitleWithItsOwnAuditAndEvent(t *testing.T) {
	f := newKeyNamedFixture(t)
	id := f.importDeal(t, "acme-q3", f.company(t, "Acme"))

	got := f.repair(t, true, exportRow("acme-q3", "Acme Q3 renewal"))[0]

	if got.Outcome != deals.KeyNameRenamed || got.To != "Acme Q3 renewal" {
		t.Errorf("apply = %+v, want renamed to the title", got)
	}
	if name := f.nameOf(t, id); name != "Acme Q3 renewal" {
		t.Errorf("deal name = %q, want the source title", name)
	}
	if n := f.e.WsCount(t, `
		SELECT count(*) FROM audit_log
		 WHERE entity_id = $1 AND action = 'update'
		   AND actor_type = 'system' AND evidence->>'source_key' = $2`, id, "acme-q3"); n != 1 {
		t.Errorf("%d system update audit rows naming the source key, want 1", n)
	}
	if n := f.e.WsCount(t, `
		SELECT count(*) FROM event_outbox
		 WHERE envelope->>'type' = 'deal.updated'
		   AND envelope->'entity'->>'id' = $1`, id.String()); n != 1 {
		t.Errorf("%d deal.updated events for the renamed deal, want 1", n)
	}
}

func TestWithoutATitleTheDealIsNamedForItsCompanyAndStage(t *testing.T) {
	f := newKeyNamedFixture(t)
	id := f.importDeal(t, "acme-q3", f.company(t, "Acme"))
	stage := f.e.WsScalar(t, `SELECT name FROM stage WHERE id = $1`, f.open)

	got := f.repair(t, true, exportRow("acme-q3", ""))[0]

	want := "Acme · " + stage
	if got.Outcome != deals.KeyNameRenamed || f.nameOf(t, id) != want {
		t.Errorf("apply = %+v, name %q; want renamed to %q", got, f.nameOf(t, id), want)
	}
}

func TestANameAPersonChangedIsLeftAlone(t *testing.T) {
	f := newKeyNamedFixture(t)
	id := f.importDeal(t, "acme-q3", f.company(t, "Acme"))
	edited := "Acme expansion"
	if _, err := f.e.Deals.UpdateDeal(f.e.Admin(), id, deals.UpdateDealInput{Name: &edited}); err != nil {
		t.Fatal(err)
	}

	got := f.repair(t, true, exportRow("acme-q3", "Acme Q3 renewal"))[0]

	if got.Outcome != deals.KeyNameNoMatch {
		t.Errorf("outcome = %q, want no-match for a deal a person renamed", got.Outcome)
	}
	if name := f.nameOf(t, id); name != edited {
		t.Errorf("deal name = %q, want the person's %q", name, edited)
	}
}

func TestADealWithNoTitleAndNoCompanyIsSkipped(t *testing.T) {
	f := newKeyNamedFixture(t)
	id := f.importDeal(t, "acme-q3", nil)

	got := f.repair(t, true, exportRow("acme-q3", ""))[0]

	if got.Outcome != deals.KeyNameNoCompany || got.DealID != id || got.To != "" {
		t.Errorf("apply = %+v, want no-company for %s", got, id)
	}
	if name := f.nameOf(t, id); name != "acme-q3" {
		t.Errorf("deal name = %q, want the key left in place", name)
	}
}

func TestAKeyTwoDealsShareIsSkippedAsAmbiguous(t *testing.T) {
	f := newKeyNamedFixture(t)
	first := f.importDeal(t, "acme-q3", f.company(t, "Acme"))
	second := f.importDeal(t, "acme-q3", f.company(t, "Acme Holdings"))

	got := f.repair(t, true, exportRow("acme-q3", "Acme Q3 renewal"))[0]

	var zero ids.DealID
	if got.Outcome != deals.KeyNameAmbiguous || got.DealID != zero {
		t.Errorf("apply = %+v, want ambiguous with no deal named", got)
	}
	for _, id := range []ids.DealID{first, second} {
		if name := f.nameOf(t, id); name != "acme-q3" {
			t.Errorf("deal %s renamed to %q under an ambiguous key", id, name)
		}
	}
}

func TestAnArchivedDealIsNotRenamed(t *testing.T) {
	f := newKeyNamedFixture(t)
	id := f.importDeal(t, "acme-q3", f.company(t, "Acme"))
	if _, err := f.e.Deals.ArchiveDeal(f.e.Admin(), id, nil); err != nil {
		t.Fatal(err)
	}

	got := f.repair(t, true, exportRow("acme-q3", "Acme Q3 renewal"))[0]

	if got.Outcome != deals.KeyNameNoMatch {
		t.Errorf("outcome = %q, want no-match for an archived deal", got.Outcome)
	}
	if name := f.nameOf(t, id); name != "acme-q3" {
		t.Errorf("archived deal renamed to %q", name)
	}
}

func TestASeatCannotRunTheRepair(t *testing.T) {
	f := newKeyNamedFixture(t)
	f.importDeal(t, "acme-q3", f.company(t, "Acme"))

	_, err := f.e.Deals.RenameKeyNamedDeals(f.e.Admin(), []deals.KeyNamedDeal{exportRow("acme-q3", "Acme Q3 renewal")}, true)

	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an admin seat ran the repair: err = %v, want permission denied", err)
	}
}

func TestARerunAfterApplyFindsNothingLeft(t *testing.T) {
	f := newKeyNamedFixture(t)
	id := f.importDeal(t, "acme-q3", f.company(t, "Acme"))
	other := f.importDeal(t, "globex-q4", f.company(t, "Globex"))
	export := []deals.KeyNamedDeal{exportRow("acme-q3", "Acme Q3 renewal"), exportRow("globex-q4", "")}

	f.repair(t, true, export...)
	again := f.repair(t, true, export...)

	for _, got := range again {
		if got.Outcome != deals.KeyNameNoMatch {
			t.Errorf("rerun of %q = %q, want no-match", got.Entry.SourceKey, got.Outcome)
		}
	}
	for _, deal := range []ids.DealID{id, other} {
		if n := f.updateAudits(t, deal); n != 1 {
			t.Errorf("deal %s has %d update audit rows after two runs, want 1", deal, n)
		}
	}
}
