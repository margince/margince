// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The open-format export bundle writer (B-E11.10a, features/04 §5):
// completeness + open-format validity of the CSV-per-object + relational
// JSON dump + files manifest + audit_log, and — the headline security
// property — that the bundle is a row-scoped read: a team-scoped caller's
// export excludes every record their lists would hide. An unscoped export
// would be a data breach, so that exclusion is a pinned test.

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// exportReadGrants grants read on every object the bundle members gate on
// — the shared searchReadGrants omits relationship, which the export
// exercises, so this suite carries its own.
func exportReadGrants() map[string]principal.ObjectGrant {
	grants := map[string]principal.ObjectGrant{}
	for _, object := range []string{"person", "organization", "deal", "lead", "activity", "relationship"} {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	return grants
}

func (e *SearchEnv) exportAdmin() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{Objects: exportReadGrants(), RowScope: principal.RowScopeAll},
	})
}

func (e *SearchEnv) exportRep(user, team ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		TeamIDs:     []ids.UUID{team},
		Permissions: principal.Permissions{Objects: exportReadGrants(), RowScope: principal.RowScopeTeam},
	})
}

// exportFixture is the two-tenant-of-one-workspace seed: a rep1 (team1)
// slice and a rep3 (team2) slice, so a team-scoped caller must see its
// own and none of the other's.
type exportFixture struct {
	rep1Person, rep3Person ids.UUID
	rep1Org, rep3Org       ids.UUID
	rep1Deal, rep3Deal     ids.UUID
	rep1Lead, rep3Lead     ids.UUID
	rep1Activity           ids.UUID
	rep3Activity           ids.UUID
}

func (e *SearchEnv) seedExportFixture(t *testing.T) exportFixture {
	t.Helper()
	pipelineID := e.SeedID(t, `INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Sales', true, 0)`)
	stageID := e.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability) VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipelineID)

	var f exportFixture
	// rep1 (team1) carries a social row to prove the child relation
	// travels with its parent.
	f.rep1Person = e.SeedID(t, `INSERT INTO person (id, owner_id, full_name, source, captured_by)
		VALUES ($1, $2, 'Rep1 Person', 'manual', 'human:x')`, e.Rep1)
	e.SeedID(t, `INSERT INTO person_social (id, person_id, platform, handle)
		VALUES ($1, $2, 'linkedin', 'in/rep1')`, f.rep1Person)
	f.rep3Person = e.SeedID(t, `INSERT INTO person (id, owner_id, full_name, source, captured_by)
		VALUES ($1, $2, 'Rep3 Person', 'manual', 'human:x')`, e.Rep3)
	f.rep1Org = e.SeedID(t, `INSERT INTO organization (id, owner_id, display_name, source, captured_by)
		VALUES ($1, $2, 'Rep1 Org', 'manual', 'human:x')`, e.Rep1)
	f.rep3Org = e.SeedID(t, `INSERT INTO organization (id, owner_id, display_name, source, captured_by)
		VALUES ($1, $2, 'Rep3 Org', 'manual', 'human:x')`, e.Rep3)
	f.rep1Deal = e.SeedID(t, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, organization_id, amount_minor, currency, source, captured_by)
		VALUES ($1, $2, 'Rep1 Deal', $3, $4, $5, 100000, 'EUR', 'manual', 'human:x')`, e.Rep1, pipelineID, stageID, f.rep1Org)
	f.rep3Deal = e.SeedID(t, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, organization_id, amount_minor, currency, source, captured_by)
		VALUES ($1, $2, 'Rep3 Deal', $3, $4, $5, 200000, 'EUR', 'manual', 'human:x')`, e.Rep3, pipelineID, stageID, f.rep3Org)
	f.rep1Lead = e.SeedID(t, `INSERT INTO lead (id, owner_id, full_name, source, captured_by)
		VALUES ($1, $2, 'Rep1 Lead', 'manual', 'human:x')`, e.Rep1)
	f.rep3Lead = e.SeedID(t, `INSERT INTO lead (id, owner_id, full_name, source, captured_by)
		VALUES ($1, $2, 'Rep3 Lead', 'manual', 'human:x')`, e.Rep3)

	// Employment edges: each connects a rep's person to that rep's org, so
	// the whole edge is visible only to that rep (both endpoints owned).
	e.SeedID(t, `INSERT INTO relationship (id, kind, person_id, organization_id, source, captured_by)
		VALUES ($1, 'employment', $2, $3, 'manual', 'human:x')`, f.rep1Person, f.rep1Org)
	e.SeedID(t, `INSERT INTO relationship (id, kind, person_id, organization_id, source, captured_by)
		VALUES ($1, 'employment', $2, $3, 'manual', 'human:x')`, f.rep3Person, f.rep3Org)

	// Activities scope through their links.
	f.rep1Activity = e.SeedID(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'note', 'Rep1 note', now(), 'manual', 'human:x')`)
	e.SeedID(t, `INSERT INTO activity_link (id, activity_id, entity_type, person_id) VALUES ($1, $2, 'person', $3)`, f.rep1Activity, f.rep1Person)
	f.rep3Activity = e.SeedID(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'note', 'Rep3 note', now(), 'manual', 'human:x')`)
	e.SeedID(t, `INSERT INTO activity_link (id, activity_id, entity_type, person_id) VALUES ($1, $2, 'person', $3)`, f.rep3Activity, f.rep3Person)

	// Attachments on each rep's person — the files manifest source.
	e.SeedID(t, `INSERT INTO attachment (id, entity_type, entity_id, filename, storage_key, source, captured_by)
		VALUES ($1, 'person', $2, 'rep1.pdf', 'blob/rep1', 'manual', 'human:x')`, f.rep1Person)
	e.SeedID(t, `INSERT INTO attachment (id, entity_type, entity_id, filename, storage_key, source, captured_by)
		VALUES ($1, 'person', $2, 'rep3.pdf', 'blob/rep3', 'manual', 'human:x')`, f.rep3Person)

	// Audit rows targeting each rep's person (audit_log is record-mutations-only).
	e.SeedID(t, `INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id)
		VALUES ($1, 'human', $2, 'create', 'person', $3)`, "human:"+e.Rep1.String(), f.rep1Person)
	e.SeedID(t, `INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id)
		VALUES ($1, 'human', $2, 'create', 'person', $3)`, "human:"+e.Rep3.String(), f.rep3Person)
	return f
}

func TestExportBundleCompleteAndValidOpenFormat(t *testing.T) {
	e := SetupSearch(t)
	f := e.seedExportFixture(t)

	var buf bytes.Buffer
	summary, err := compose.NewExportWriter(e.Pool).WriteBundle(e.exportAdmin(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	entries := BundleEntries(t, buf.Bytes())

	// Every member CSV, the relational dump, the files manifest, and the
	// bundle manifest are present.
	for _, name := range []string{
		"person.csv", "organization.csv", "deal.csv", "lead.csv", "activity.csv",
		"relationship.csv", "pipeline.csv", "stage.csv", "attachment.csv", "audit_log.csv",
		"data.json", "files-manifest.json", "manifest.json",
	} {
		if _, ok := entries[name]; !ok {
			t.Fatalf("bundle is missing %s; got %v", name, keys(entries))
		}
	}

	// The admin (row_scope=all) sees both reps' rows — completeness.
	if got := len(CSVColumn(t, entries["person.csv"], "id")); got != 2 {
		t.Fatalf("person.csv has %d rows, want 2 (both reps)", got)
	}
	if summary.RowCounts["deal"] != 2 || summary.RowCounts["relationship"] != 2 {
		t.Fatalf("summary counts wrong: %+v", summary.RowCounts)
	}

	// The relational JSON dump validates and nests every object; the
	// jsonb column round-trips as a nested object, never base64.
	var dump struct {
		Format  string                      `json:"format"`
		Objects map[string][]map[string]any `json:"objects"`
	}
	if err := json.Unmarshal(entries["data.json"], &dump); err != nil {
		t.Fatalf("data.json is not valid JSON: %v", err)
	}
	if dump.Format == "" || len(dump.Objects["person"]) != 2 {
		t.Fatalf("data.json dump incomplete: format=%q persons=%d", dump.Format, len(dump.Objects["person"]))
	}
	var handle any
	for _, s := range dump.Objects["person_social"] {
		if s["platform"] == "linkedin" {
			handle = s["handle"]
		}
	}
	if handle != "in/rep1" {
		t.Fatalf("the person_social child relation did not travel with its parent: handle=%v", handle)
	}

	// The files manifest lists both attachments.
	var manifest struct {
		Files []map[string]any `json:"files"`
	}
	if err := json.Unmarshal(entries["files-manifest.json"], &manifest); err != nil {
		t.Fatalf("files-manifest.json invalid: %v", err)
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("files manifest has %d files, want 2", len(manifest.Files))
	}
	_ = f
}

// The pinned security property: a team-scoped caller's export contains
// exactly what that caller may read — across every scoped member (records,
// edges, activities, files, audit). Contacts, accounts, deals and leads are
// readable by every seat, so the other team's deal and lead ARE in the
// bundle; what stays out is the other rep's capture-private contact and
// account, and everything reachable only through them.
func TestExportRowScopeExcludesInvisibleRecords(t *testing.T) {
	e := SetupSearch(t)
	f := e.seedExportFixture(t)
	for _, private := range []struct {
		table string
		id    ids.UUID
	}{
		{"person", f.rep3Person}, {"organization", f.rep3Org},
	} {
		if _, err := e.Owner.Exec(context.Background(),
			`UPDATE `+private.table+` SET visibility = 'owner' WHERE id = $1`, private.id); err != nil {
			t.Fatalf("making the other rep's %s capture-private: %v", private.table, err)
		}
	}

	var buf bytes.Buffer
	summary, err := compose.NewExportWriter(e.Pool).WriteBundle(e.exportRep(e.Rep1, e.Team1), &buf)
	if err != nil {
		t.Fatal(err)
	}
	entries := BundleEntries(t, buf.Bytes())

	assertOnlyID := func(file string, want, hidden ids.UUID) {
		rowIDs := CSVColumn(t, entries[file], "id")
		set := map[string]bool{}
		for _, id := range rowIDs {
			set[id] = true
		}
		if !set[want.String()] {
			t.Fatalf("%s dropped the caller's own row %s: got %v", file, want, rowIDs)
		}
		if set[hidden.String()] {
			t.Fatalf("%s LEAKED an invisible row %s: got %v", file, hidden, rowIDs)
		}
	}
	assertBothIDs := func(file string, mine, theirs ids.UUID) {
		rowIDs := CSVColumn(t, entries[file], "id")
		set := map[string]bool{}
		for _, id := range rowIDs {
			set[id] = true
		}
		if !set[mine.String()] || !set[theirs.String()] {
			t.Fatalf("%s is missing a row every seat may read (mine %s, theirs %s): got %v", file, mine, theirs, rowIDs)
		}
	}
	assertOnlyID("person.csv", f.rep1Person, f.rep3Person)
	assertOnlyID("organization.csv", f.rep1Org, f.rep3Org)
	assertBothIDs("deal.csv", f.rep1Deal, f.rep3Deal)
	assertBothIDs("lead.csv", f.rep1Lead, f.rep3Lead)
	assertOnlyID("activity.csv", f.rep1Activity, f.rep3Activity)

	// One employment edge each; the other rep's joins two private endpoints,
	// so the caller sees only its own.
	if got := summary.RowCounts["relationship"]; got != 1 {
		t.Fatalf("row-scope leak: relationship count = %d, want 1", got)
	}
	// The attachment (files manifest) hides the other rep's file.
	entIDs := CSVColumn(t, entries["attachment.csv"], "entity_id")
	if len(entIDs) != 1 || entIDs[0] != f.rep1Person.String() {
		t.Fatalf("row-scope leak in files manifest: attachment entity_ids = %v", entIDs)
	}
	// The audit_log excludes the row about the invisible person.
	auditEntities := CSVColumn(t, entries["audit_log.csv"], "entity_id")
	for _, id := range auditEntities {
		if id == f.rep3Person.String() {
			t.Fatalf("row-scope leak: audit_log exposed an invisible person's row %s", id)
		}
	}
	// Pipeline/stage are workspace-shared reference data — present for
	// every member so exported deals resolve their stage.
	if summary.RowCounts["pipeline"] != 1 || summary.RowCounts["stage"] != 1 {
		t.Fatalf("workspace-shared config missing from a scoped export: %+v", summary.RowCounts)
	}
}

// RBAC bounds what the export contains: an object with no read grant is
// omitted from the bundle entirely, not silently dumped.
func TestExportOmitsObjectsWithoutReadGrant(t *testing.T) {
	e := SetupSearch(t)
	e.seedExportFixture(t)

	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	personOnly := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"person": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	var buf bytes.Buffer
	summary, err := compose.NewExportWriter(e.Pool).WriteBundle(personOnly, &buf)
	if err != nil {
		t.Fatal(err)
	}
	entries := BundleEntries(t, buf.Bytes())

	if _, ok := entries["person.csv"]; !ok {
		t.Fatal("granted object person was omitted")
	}
	for _, denied := range []string{"deal.csv", "organization.csv", "lead.csv", "activity.csv", "relationship.csv"} {
		if _, ok := entries[denied]; ok {
			t.Fatalf("ungranted object %s was exported", denied)
		}
	}
	omitted := strings.Join(summary.Omitted, ",")
	for _, want := range []string{"deal", "organization", "lead", "activity", "relationship"} {
		if !strings.Contains(omitted, want) {
			t.Fatalf("summary.Omitted missing %q: %v", want, summary.Omitted)
		}
	}
}

// keys lists a map's keys for failure messages.
func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The bundle is a reader of the audit spine, and the spine is append-only: an
// Art. 17 erase cannot rewrite the images it certifies gone, so every reader
// stops at the newest scrub tombstone. A boundary the three API reads apply and
// the export does not is the same disclosure through a quieter door — and the
// export is the door that puts the whole table on somebody's disk.
func TestTheBundleWithholdsImagesAnErasureCertifiedGone(t *testing.T) {
	e := SetupSearch(t)
	f := e.seedExportFixture(t)

	const typed = "Sara Typed This"
	e.SeedID(t, `INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, before, after, occurred_at)
		VALUES ($1, 'human', $2, 'update', 'person', $3, $4::jsonb, $5::jsonb, now() - interval '2 hours')`,
		"human:"+e.Rep1.String(), f.rep1Person,
		`{"full_name":"`+typed+`"}`, `{"full_name":"Sara Renamed"}`)
	e.SeedID(t, `INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, occurred_at)
		VALUES ($1, 'human', $2, 'erase', 'person', $3, now() - interval '1 hour')`,
		"human:"+e.Rep1.String(), f.rep1Person)

	var buf bytes.Buffer
	if _, err := compose.NewExportWriter(e.Pool).WriteBundle(e.exportAdmin(), &buf); err != nil {
		t.Fatal(err)
	}
	entries := BundleEntries(t, buf.Bytes())

	if bytes.Contains(entries["audit_log.csv"], []byte(typed)) {
		t.Error("audit_log.csv carries a value the erasure certified gone")
	}
	if bytes.Contains(entries["data.json"], []byte(typed)) {
		t.Error("data.json carries a value the erasure certified gone")
	}
	// The row is still exported: a bundle that dropped it would answer "who
	// touched this record" with a gap, which an erasure does not create.
	if !bytes.Contains(entries["audit_log.csv"], []byte(f.rep1Person.String())) {
		t.Error("the erased record's audit rows vanished from the bundle entirely")
	}
}

// An admin's export carries no limited message's attachment name and no audit
// image of one. The filename states what the message is about
// (`Aufhebungsvertrag_Mueller.pdf`) and the audit image carries its
// before-and-after, so both are that message's content reached by another
// route than its body.
//
// The admin here holds row_scope=all, which is the strongest reader the product
// has: audience does not yield to it (auth.ActivityContentClause), and this is
// the test that says so for the bundle.
func TestAnAdminExportCarriesNoHeldAttachmentOrAuditImage(t *testing.T) {
	e := SetupSearch(t)
	e.seedExportFixture(t)

	seedMail := func(subject, audience, filename string) {
		t.Helper()
		activity := e.SeedID(t, `
			INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by, audience)
			VALUES ($1, 'email', $2, 'the body of it', now(), 'gmail', 'connector:gmail:'||$3::text, $4)`,
			subject, e.Rep1.String(), audience)
		e.SeedID(t, `
			INSERT INTO attachment (id, entity_type, entity_id, filename, storage_key, source, captured_by)
			VALUES ($1, 'activity', $2, $3, 'blob/'||$3, 'gmail', 'connector:gmail')`, activity, filename)
		e.SeedID(t, `
			INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id)
			VALUES ($1, 'human', $2, 'update', 'activity', $3)`, "human:"+e.Rep1.String(), activity)
	}
	// The open message is what tells a working gate from a broken export: both
	// rows travel the same code path and differ only in the audience.
	seedMail("ordinary quote", "workspace", "Angebot_offen.pdf")
	seedMail("Aufhebungsvertrag", "participants", "Aufhebungsvertrag_Mueller.pdf")

	var buf bytes.Buffer
	if _, err := compose.NewExportWriter(e.Pool).WriteBundle(e.exportAdmin(), &buf); err != nil {
		t.Fatal(err)
	}
	entries := BundleEntries(t, buf.Bytes())

	names := CSVColumn(t, entries["attachment.csv"], "filename")
	open, held := false, false
	for _, name := range names {
		switch name {
		case "Angebot_offen.pdf":
			open = true
		case "Aufhebungsvertrag_Mueller.pdf":
			held = true
		}
	}
	if !open {
		t.Fatal("the open message's attachment is missing from the admin bundle — the fixture cannot tell a working gate from a broken export")
	}
	if held {
		t.Errorf("the limited message's attachment name is in an admin export: %v — the filename states what the message is about, to a reader its audience excludes", names)
	}

	// The audit image is the same disclosure by another route: it carries the
	// before and after of a change to the message.
	auditTargets := CSVColumn(t, entries["audit_log.csv"], "entity_type")
	activityImages := 0
	for _, kind := range auditTargets {
		if kind == "activity" {
			activityImages++
		}
	}
	if activityImages != 1 {
		t.Errorf("the admin bundle carried %d activity audit images, want 1 (the open message's) — an image of a limited message is that message's content in the compliance trail", activityImages)
	}
}

// The own-trail arm of the audit scope, which is the route an audience test on
// the entity arm alone does not cover: a colleague who TOUCHED a message before
// it was limited would otherwise export its before-and-after image forever,
// because `actor_id = me` admitted the row without asking about the activity.
//
// The rep here is bounded (row_scope=team) and is the actor on the audit row,
// so the entity arm refuses the row and only the own-trail arm can admit it.
func TestARepsOwnAuditTrailStopsAtALimitedMessage(t *testing.T) {
	e := SetupSearch(t)
	f := e.seedExportFixture(t)

	limited := e.SeedID(t, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by, audience)
		VALUES ($1, 'email', 'Aufhebungsvertrag', 'the body of it', now(), 'gmail', 'connector:gmail:'||$2::text, 'participants')`,
		e.Rep3.String())
	// Rep1 performed the change and is not in the audience: the message belongs
	// to rep3's mailbox and rep1 is not a participant.
	e.SeedID(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id)
		VALUES ($1, 'human', $2, 'update', 'activity', $3)`, "human:"+e.Rep1.String(), limited)

	// The same message's ATTACHMENT is the second route to the same content: an
	// attachment audit image carries the filename, and a filename states what
	// the message is about.
	limitedFile := e.SeedID(t, `
		INSERT INTO attachment (id, entity_type, entity_id, filename, storage_key, source, captured_by)
		VALUES ($1, 'activity', $2, 'Aufhebungsvertrag_Mueller.pdf', 'blob/limited', 'gmail', 'connector:gmail')`, limited)
	e.SeedID(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id)
		VALUES ($1, 'human', $2, 'update', 'attachment', $3)`, "human:"+e.Rep1.String(), limitedFile)

	// And an attachment of an OPEN message, which must still travel: the limit
	// is the message's audience, never the fact that a row names an attachment.
	openMail := e.SeedID(t, `
		INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, audience)
		VALUES ($1, 'email', 'ordinary quote', now(), 'gmail', 'connector:gmail:'||$2::text, 'workspace')`,
		e.Rep3.String())
	openFile := e.SeedID(t, `
		INSERT INTO attachment (id, entity_type, entity_id, filename, storage_key, source, captured_by)
		VALUES ($1, 'activity', $2, 'Angebot_offen.pdf', 'blob/open', 'gmail', 'connector:gmail')`, openMail)
	e.SeedID(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id)
		VALUES ($1, 'human', $2, 'update', 'attachment', $3)`, "human:"+e.Rep1.String(), openFile)

	var buf bytes.Buffer
	if _, err := compose.NewExportWriter(e.Pool).WriteBundle(e.exportRep(e.Rep1, e.Team1), &buf); err != nil {
		t.Fatal(err)
	}
	entries := BundleEntries(t, buf.Bytes())
	exported := map[string]bool{}
	for _, id := range CSVColumn(t, entries["audit_log.csv"], "entity_id") {
		exported[id] = true
	}
	if exported[limited.String()] {
		t.Error("a rep exported the audit image of a message whose audience excludes them, because they were the actor on it — " +
			"having made a change is not permission to read what the message says")
	}
	if exported[limitedFile.String()] {
		t.Error("a rep exported the audit image of a limited message's ATTACHMENT — the filename states what the message is about, " +
			"and excluding only the activity row leaves the same content reachable by the longer route")
	}
	if !exported[openFile.String()] {
		t.Error("an open message's attachment audit image was withheld — the limit is the message's audience, " +
			"not the fact that a row names an attachment")
	}
	_ = f
}
