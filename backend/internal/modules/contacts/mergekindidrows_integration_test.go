// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What a merge does with the rows a reader owns about a record. They are the
// access shared on it, an unsent draft, a verdict on an AI claim, a notice and
// a plan line. The author of an inherited value is covered here too.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *dedupeEnv) user(ctx context.Context, t *testing.T, email string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.execAs(ctx, t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Reader')`, id, email)
	return id
}

func (e *dedupeEnv) grant(ctx context.Context, t *testing.T, recordID, subject ids.UUID, access string) {
	t.Helper()
	e.execAs(ctx, t, `INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
		VALUES ('company', $1, 'user', $2, $3, $4)`, recordID, subject, access, e.rep)
}

// A reader who could open the retired company can open the survivor.
func TestMergingACompanyCarriesTheAccessSharedOnIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := asAdmin(e)
	source, survivor := e.companyPair(ctx, t)
	both := e.user(ctx, t, "both@grant.test")
	onlyRetired := e.user(ctx, t, "only@grant.test")
	e.grant(ctx, t, source.UUID, both, "write")
	e.grant(ctx, t, survivor.UUID, both, "read")
	e.grant(ctx, t, source.UUID, onlyRetired, "read")

	if _, err := e.store.MergeCompany(ctx, source, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	const held = `SELECT count(*) FROM record_grant WHERE record_type = 'company' AND record_id = $1`
	if n := e.countAs(ctx, t, held, source); n != 0 {
		t.Errorf("%d grants still name the retired company", n)
	}
	if n := e.countAs(ctx, t, held, survivor); n != 2 {
		t.Errorf("the survivor holds %d grants, want 2", n)
	}
	if n := e.countAs(ctx, t, held+` AND subject_id = $2 AND access = 'write'`, survivor, both); n != 1 {
		t.Errorf("a subject holding both keeps the stronger access: got %d write grants", n)
	}
}

// A draft, a verdict, a notice and a plan line follow the survivor.
func TestMergingACompanyMovesDraftsVerdictsNoticesAndPlanLines(t *testing.T) {
	e := setupDedupe(t)
	ctx := asAdmin(e)
	source, survivor := e.companyPair(ctx, t)
	other := e.user(ctx, t, "other@owned.test")
	for _, anchor := range []ids.CompanyID{source, survivor} {
		e.execAs(ctx, t, `INSERT INTO mail_draft (id, anchor_type, anchor_id, author_id)
			VALUES ($1, 'company', $2, $3)`, ids.NewV7(), anchor, e.rep)
	}
	e.execAs(ctx, t, `INSERT INTO mail_draft (id, anchor_type, anchor_id, author_id)
		VALUES ($1, 'company', $2, $3)`, ids.NewV7(), source, other)
	for _, row := range []struct {
		subject ids.CompanyID
		key     string
	}{{source, "a"}, {source, "b"}, {survivor, "a"}} {
		e.execAs(ctx, t, `INSERT INTO ai_feedback (subject_type, subject_id, claim_kind, claim_key, verdict, source, captured_by)
			VALUES ('company', $1, 'signal', $2, 'confirmed', 'human', 'human:x')`, row.subject, row.key)
	}
	e.execAs(ctx, t, `INSERT INTO notice (kind, subject, recipient_user_id, captured_by, target_type, target_id)
		VALUES ('automation', 'look', $1, 'human:x', 'company', $2)`, e.rep, source)
	plan := ids.NewV7()
	e.execAs(ctx, t, `INSERT INTO weekly_plan (id, owner_id, local_week_start, captured_by)
		VALUES ($1, $2, '2026-10-05', 'human:x')`, plan, e.rep)
	e.execAs(ctx, t, `INSERT INTO weekly_plan_commitment (plan_id, label, captured_by, linked_record_type, linked_record_id)
		VALUES ($1, 'call them', 'human:x', 'company', $2)`, plan, source)

	if _, err := e.store.MergeCompany(ctx, source, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	for _, c := range []struct {
		name, sql string
		want      int
	}{
		{"drafts", `SELECT count(*) FROM mail_draft WHERE anchor_type = 'company' AND anchor_id = $1`, 2},
		{"verdicts", `SELECT count(*) FROM ai_feedback WHERE subject_type = 'company' AND subject_id = $1`, 2},
		{"notices", `SELECT count(*) FROM notice WHERE target_type = 'company' AND target_id = $1`, 1},
		{"plan lines", `SELECT count(*) FROM weekly_plan_commitment WHERE linked_record_type = 'company' AND linked_record_id = $1`, 1},
	} {
		if n := e.countAs(ctx, t, c.sql, source); n != 0 {
			t.Errorf("%d %s still name the retired company", n, c.name)
		}
		if n := e.countAs(ctx, t, c.sql, survivor); n != c.want {
			t.Errorf("the survivor holds %d %s, want %d", n, c.name, c.want)
		}
	}
}

// An inherited industry keeps the author it had on the retired company.
func TestMergingACompanyCarriesTheAuthorOfAnInheritedValue(t *testing.T) {
	e := setupDedupe(t)
	ctx := asAdmin(e)
	source, survivor := e.companyPair(ctx, t)
	industry := "Glazing"
	if _, err := e.store.UpdateCompany(ctx, source, UpdateCompanyInput{Industry: &industry}); err != nil {
		t.Fatal(err)
	}
	e.execAs(ctx, t, `INSERT INTO field_provenance (object_type, object_id, field_name, source, captured_by)
		VALUES ('company', $1, 'industry', 'manual', 'human:author')`, source)

	if _, err := e.store.MergeCompany(ctx, source, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	n := e.countAs(ctx, t, `SELECT count(*) FROM field_provenance
		WHERE object_type = 'company' AND object_id = $1 AND field_name = 'industry'
		  AND captured_by = 'human:author'`, survivor)
	if n != 1 {
		t.Errorf("the survivor holds %d industry claims by the original author, want 1", n)
	}
}

// A file filed under the retired contact is filed under the survivor now.
func TestMergingAContactMovesTheFilesNamingIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	survivor, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "keeper@files.test", "Keeper", "files.test"))
	if err != nil {
		t.Fatal(err)
	}
	retired, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "dupe@files.test", "Dupe", "files.test"))
	if err != nil {
		t.Fatal(err)
	}
	e.execAs(ctx, t, `INSERT INTO attachment (id, entity_type, entity_id, filename, storage_key, source, captured_by)
		VALUES ($1, 'contact', $2, 'nda.pdf', 'k/nda', 'upload', 'human:x')`, ids.NewV7(), retired.ContactID)

	if _, err := e.store.MergeContact(ctx, retired.ContactID, survivor.ContactID, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	const filed = `SELECT count(*) FROM attachment WHERE entity_type = 'contact' AND entity_id = $1`
	if n := e.countAs(ctx, t, filed, survivor.ContactID); n != 1 {
		t.Errorf("the survivor holds %d files, want 1", n)
	}
	if n := e.countAs(ctx, t, filed, retired.ContactID); n != 0 {
		t.Errorf("%d files still name the retired contact", n)
	}
}
