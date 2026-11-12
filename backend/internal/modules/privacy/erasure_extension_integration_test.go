// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// What an extension's declaration buys the subject. The export serves the
// declared personal columns. The erasure clears them in the same transaction as
// the core's own tables, and the core writes the audit row.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	extdecl "github.com/margince/margince/backend/pkg/extension/privacy"
)

const probeTable = "ext_probe_note"

// declareProbeTable creates a unit table through the owner connection and
// declares it, then undoes both. The table has a required text column and a
// required bytea column, the two shapes the core has to blank without a NULL.
func declareProbeTable(t *testing.T, env *sarIdentifierEnv) extprivacy.Registry {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`GRANT USAGE ON SCHEMA ext TO margince_app`,
		`CREATE TABLE ext.` + probeTable + ` (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			contact_id uuid, lead_id uuid, body text NOT NULL, raw bytea NOT NULL, kind text NOT NULL)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.` + probeTable + ` TO margince_app`,
	} {
		if _, err := env.owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparing %s (%s): %v", probeTable, stmt, err)
		}
	}
	t.Cleanup(func() {
		if _, err := env.owner.Exec(context.Background(), `DROP TABLE ext.`+probeTable); err != nil {
			t.Errorf("removing %s: %v", probeTable, err)
		}
	})
	return extprivacy.Declared([]extdecl.Table{{
		Name: probeTable,
		Links: []extdecl.Link{
			{Column: "contact_id", Record: extdecl.Contact}, {Column: "lead_id", Record: extdecl.Lead},
		},
		Personal:  []string{"body", "raw"},
		Retention: jurisdiction.CommercialCorrespondence,
	}})
}

func seedProbeRows(t *testing.T, env *sarIdentifierEnv) (mine, other ids.UUID) {
	t.Helper()
	ctx := context.Background()
	mine, other = ids.NewV7(), ids.NewV7()
	for _, row := range []struct {
		id      ids.UUID
		contact ids.UUID
		body    string
	}{{mine, env.contact.UUID, "Sara's own words"}, {other, ids.NewV7(), "somebody else"}} {
		if _, err := env.owner.Exec(ctx,
			`INSERT INTO ext.`+probeTable+` (id, contact_id, body, raw, kind) VALUES ($1, $2, $3, $4, 'note')`,
			row.id, row.contact, row.body, []byte(row.body)); err != nil {
			t.Fatal(err)
		}
	}
	return mine, other
}

func TestSubjectExportServesDeclaredExtensionColumns(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareProbeTable(t, env)
	mine, _ := seedProbeRows(t, env)

	pkg, err := AssembleSARWithExtensions(env.ctx, env.db, env.contact, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.ExtensionData) != 1 {
		t.Fatalf("export holds %d extension rows, want exactly the subject's one: %v", len(pkg.ExtensionData), pkg.ExtensionData)
	}
	row := pkg.ExtensionData[0]
	if row["table"] != probeTable || row["id"] != mine {
		t.Errorf("export names %v/%v, want %s/%s", row["table"], row["id"], probeTable, mine)
	}
	data, ok := row["personal_data"].(map[string]any)
	if !ok || data["body"] != "Sara's own words" {
		t.Errorf("export personal data = %v, want the declared body", row["personal_data"])
	}
	if _, leaked := data["kind"]; leaked {
		t.Error("export served a column the unit did not declare personal")
	}
}

func TestSubjectExportServesAnEmptyExtensionListForASubjectNoUnitNames(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareProbeTable(t, env)

	pkg, err := AssembleSARWithExtensions(env.ctx, env.db, env.contact, registry)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"extension_data":[]`) {
		t.Errorf("export without extension rows must serve an empty list like every other section, got %s", body)
	}
}

func TestErasureClearsDeclaredExtensionColumnsAndTheCoreAuditsIt(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareProbeTable(t, env)
	mine, other := seedProbeRows(t, env)

	if err := NewEraser(env.db).WithExtensions(registry).EraseContact(env.ctx, env.contact.UUID, "request"); err != nil {
		t.Fatal(err)
	}

	read := func(id ids.UUID) (body string, raw []byte, kind string) {
		t.Helper()
		if err := env.owner.QueryRow(context.Background(),
			`SELECT body, raw, kind FROM ext.`+probeTable+` WHERE id = $1`, id).Scan(&body, &raw, &kind); err != nil {
			t.Fatal(err)
		}
		return body, raw, kind
	}
	body, raw, kind := read(mine)
	if body != "" || len(raw) != 0 {
		t.Errorf("the subject's row still holds body=%q raw=%q after erasure", body, raw)
	}
	if kind != "note" {
		t.Errorf("an undeclared column was cleared: kind=%q", kind)
	}
	if body, _, _ := read(other); body != "somebody else" {
		t.Errorf("another subject's row was touched: body=%q", body)
	}

	var audited int
	if err := env.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = $1 AND entity_id = $2 AND action = 'anonymize'`, probeTable, mine).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Errorf("core wrote %d anonymize audit rows for the cleared extension row, want 1", audited)
	}
}

func TestDeletingAnUntoldLeadClearsTheExtensionRowsLinkedToIt(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareProbeTable(t, env)
	ctx := context.Background()
	lead := ids.NewV7()
	for _, stmt := range []string{
		`INSERT INTO lead (id, full_name, source, captured_by) VALUES ($1, 'Untold', 'manual', 'system:test')`,
		`INSERT INTO lead_privacy_notice (lead_id, template_key, template_version, state, failure, failed_at) VALUES ($1, 'site_lead', 1, 'failed', 'refused', now())`,
	} {
		if _, err := env.owner.Exec(ctx, stmt, lead); err != nil {
			t.Fatalf("seeding the untold lead (%s): %v", stmt, err)
		}
	}
	row := ids.NewV7()
	if _, err := env.owner.Exec(ctx,
		`INSERT INTO ext.`+probeTable+` (id, lead_id, body, raw, kind) VALUES ($1, $2, 'what the lead wrote', 'x', 'note')`,
		row, lead); err != nil {
		t.Fatal(err)
	}

	err := env.db.Tx(env.ctx, func(tx pgx.Tx) error {
		return EraseUntoldLeadTx(env.ctx, tx, UntoldLead{LeadID: ids.From[ids.LeadKind](lead), Extensions: registry})
	})
	if err != nil {
		t.Fatal(err)
	}
	var body string
	if err := env.owner.QueryRow(ctx, `SELECT body FROM ext.`+probeTable+` WHERE id = $1`, row).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		t.Errorf("the extension row still holds %q after its lead was deleted", body)
	}
	var audited int
	if err := env.owner.QueryRow(ctx, `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = $1 AND entity_id = $2 AND action = 'anonymize'`, probeTable, row).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Errorf("the cleared extension row has %d anonymize entries, want 1", audited)
	}
}

func TestAuditImagesOfAClearedExtensionRowAreWithheld(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareProbeTable(t, env)
	mine, _ := seedProbeRows(t, env)
	err := env.db.Tx(env.ctx, func(tx pgx.Tx) error {
		_, err := storekit.Audit(env.ctx, tx, "update", probeTable, mine,
			map[string]any{"body": "Sara's own words"}, map[string]any{"body": "Sara's later words"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := NewEraser(env.db).WithExtensions(registry).EraseContact(env.ctx, env.contact.UUID, "request"); err != nil {
		t.Fatal(err)
	}

	page, err := ListAuditLog(env.ctx, env.db, AuditFilter{EntityID: &mine})
	if err != nil {
		t.Fatal(err)
	}
	var seen int
	for _, e := range page.Entries {
		if e.Action != "update" {
			continue
		}
		seen++
		if strings.Contains(string(e.Before)+string(e.After), "Sara") {
			t.Errorf("the compliance log still serves the cleared value: before=%s after=%s", e.Before, e.After)
		}
	}
	if seen != 1 {
		t.Fatalf("the update entry is missing from the log (%d seen); the row must stay, only its images go", seen)
	}
}

// seedSubjectActivity gives the subject one linked activity and returns it.
func seedSubjectActivity(t *testing.T, env *sarIdentifierEnv) ids.UUID {
	t.Helper()
	ctx := context.Background()
	activity := ids.NewV7()
	mustExec(ctx, t, env.owner, `INSERT INTO activity (id, kind, occurred_at, source, captured_by)
		VALUES ($1, 'email', now(), 'manual', 'system:test')`, activity)
	mustExec(ctx, t, env.owner, `INSERT INTO activity_link (activity_id, entity_type, contact_id)
		VALUES ($1, 'contact', $2)`, activity, env.contact.UUID)
	return activity
}

// declareMarkerTable adds a linked table with no row id and no personal column,
// the shape a unit keeps for bookkeeping about an activity.
func declareMarkerTable(t *testing.T, env *sarIdentifierEnv, registry extprivacy.Registry) extprivacy.Registry {
	t.Helper()
	ctx := context.Background()
	mustExec(ctx, t, env.owner, `CREATE TABLE ext.ext_probe_marker (activity_id uuid PRIMARY KEY)`)
	mustExec(ctx, t, env.owner, `GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_probe_marker TO margince_app`)
	t.Cleanup(func() {
		if _, err := env.owner.Exec(context.Background(), `DROP TABLE ext.ext_probe_marker`); err != nil {
			t.Errorf("removing ext_probe_marker: %v", err)
		}
	})
	return extprivacy.Declared(append(registry.Tables(), extdecl.Table{
		Name:      "ext_probe_marker",
		Links:     []extdecl.Link{{Column: "activity_id", Record: extdecl.Activity}},
		Retention: jurisdiction.CommercialCorrespondence,
	}))
}

func TestSubjectExportReadsPastALinkedTableWithNoPersonalColumn(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareMarkerTable(t, env, declareProbeTable(t, env))
	seedProbeRows(t, env)
	activity := seedSubjectActivity(t, env)
	mustExec(context.Background(), t, env.owner, `INSERT INTO ext.ext_probe_marker (activity_id) VALUES ($1)`, activity)

	pkg, err := AssembleSARWithExtensions(env.ctx, env.db, env.contact, registry)
	if err != nil {
		t.Fatalf("export with a keyless linked table = %v", err)
	}
	if len(pkg.ExtensionData) != 1 || pkg.ExtensionData[0]["table"] != probeTable {
		t.Errorf("export served %v, want only the probe row that holds personal data", pkg.ExtensionData)
	}
}

func TestErasureRedactsTheStoredHistoryOfAClearedExtensionRow(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareProbeTable(t, env)
	mine, _ := seedProbeRows(t, env)
	var update ids.UUID
	err := env.db.Tx(env.ctx, func(tx pgx.Tx) error {
		var err error
		update, err = storekit.Audit(env.ctx, tx, "update", probeTable, mine,
			map[string]any{"body": "Sara's own words"}, map[string]any{"body": "Sara's later words"})
		if err != nil {
			return err
		}
		return storekit.EmitEventForEntity(env.ctx, tx, update, probeTable, mine,
			retentionAppliedPayload(crmcontracts.RetentionAppliedAnonymize, nil, nil))
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := NewEraser(env.db).WithExtensions(registry).EraseContact(env.ctx, env.contact.UUID, "request"); err != nil {
		t.Fatal(err)
	}

	var images string
	if err := env.owner.QueryRow(context.Background(),
		`SELECT coalesce(before::text, '') || coalesce(after::text, '') FROM audit_log WHERE id = $1`, update).Scan(&images); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(images, "Sara") {
		t.Errorf("the stored audit entry still holds the cleared value: %s", images)
	}
	var queued int
	if err := env.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM event_outbox WHERE envelope->'trace'->>'audit_log_id' = $1`, update.String()).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Errorf("%d events of the redacted entry are still queued", queued)
	}
}

func TestErasingAnActivityContentClearsTheExtensionRowsLinkedToIt(t *testing.T) {
	env := setupSARIdentifiers(t)
	declareProbeTable(t, env)
	activity := seedSubjectActivity(t, env)
	mustExec(context.Background(), t, env.owner, `ALTER TABLE ext.`+probeTable+` ADD COLUMN activity_id uuid`)
	registry := extprivacy.Declared([]extdecl.Table{{
		Name: probeTable, Links: []extdecl.Link{{Column: "activity_id", Record: extdecl.Activity}},
		Personal: []string{"body", "raw"}, Retention: jurisdiction.CommercialCorrespondence,
	}})
	row := ids.NewV7()
	mustExec(context.Background(), t, env.owner,
		`INSERT INTO ext.`+probeTable+` (id, activity_id, body, raw, kind) VALUES ($1, $2, 'the message', 'x', 'note')`, row, activity)

	eraser := NewEraser(env.db).WithRawCapturePurger(&nilRawPurger{}).WithExtensions(registry)
	if err := env.db.Tx(env.ctx, func(tx pgx.Tx) error {
		return eraser.purgeContentDerivedFrom(env.ctx, tx, activity, theClockRanOut)
	}); err != nil {
		t.Fatal(err)
	}
	assertProbeRowCleared(t, env, row)
}

func TestAnonymizingALeadClearsTheExtensionRowsLinkedToIt(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareProbeTable(t, env)
	lead, row := ids.NewV7(), ids.NewV7()
	mustExec(context.Background(), t, env.owner,
		`INSERT INTO lead (id, full_name, source, captured_by) VALUES ($1, 'Aged', 'manual', 'system:test')`, lead)
	mustExec(context.Background(), t, env.owner,
		`INSERT INTO ext.`+probeTable+` (id, lead_id, body, raw, kind) VALUES ($1, $2, 'what the lead wrote', 'x', 'note')`, row, lead)

	retention := &RetentionService{eraser: NewEraser(env.db).WithExtensions(registry)}
	if err := env.db.Tx(env.ctx, func(tx pgx.Tx) error { return retention.anonymizeLead(env.ctx, tx, lead) }); err != nil {
		t.Fatal(err)
	}
	assertProbeRowCleared(t, env, row)
}

// One erasure can reach a row by two paths, as a promise's task is reached
// through its message and through its contact. The row is certified once.
func TestAnExtensionRowTwoClearingPathsReachIsCertifiedOnce(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareProbeTable(t, env)
	lead, row := ids.NewV7(), ids.NewV7()
	mustExec(context.Background(), t, env.owner,
		`INSERT INTO lead (id, full_name, source, captured_by) VALUES ($1, 'Twin', 'manual', 'system:test')`, lead)
	mustExec(context.Background(), t, env.owner,
		`INSERT INTO ext.`+probeTable+` (id, contact_id, lead_id, body, raw, kind) VALUES ($1, $2, $3, 'words', 'x', 'note')`,
		row, env.contact.UUID, lead)

	if err := env.db.Tx(env.ctx, func(tx pgx.Tx) error {
		for _, subject := range []map[extdecl.RecordKind][]ids.UUID{{extdecl.Contact: {env.contact.UUID}}, {extdecl.Lead: {lead}}} {
			if err := clearExtensionRows(env.ctx, tx, registry, "request", causeContactErasure, subject); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertProbeRowCleared(t, env, row)
	var certificates, events int
	if err := env.owner.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM audit_log WHERE entity_type = $1 AND entity_id = $2 AND action = 'anonymize'),
		       (SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id' = $2::text)`,
		probeTable, row).Scan(&certificates, &events); err != nil {
		t.Fatal(err)
	}
	if certificates != 1 || events != 1 {
		t.Errorf("a row two paths reach has %d anonymize certificates and %d events, want 1 and 1", certificates, events)
	}
}

func assertProbeRowCleared(t *testing.T, env *sarIdentifierEnv, row ids.UUID) {
	t.Helper()
	var body string
	if err := env.owner.QueryRow(context.Background(), `SELECT body FROM ext.`+probeTable+` WHERE id = $1`, row).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		t.Errorf("the extension row still holds %q", body)
	}
}
