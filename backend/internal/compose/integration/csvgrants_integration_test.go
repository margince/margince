// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// actWithGrant makes the signed-in admin hold a role of its own: the admin's
// document with one object's grant replaced, so a refusal can only come from
// that grant.
func actWithGrant(t *testing.T, e *apptest.AppEnv, object, grant string) {
	t.Helper()
	if _, err := e.Owner.Exec(t.Context(), `
		INSERT INTO role (key, name, is_system, permissions)
		SELECT 'importer_narrowed', 'Importer', false, jsonb_set(r.permissions, ARRAY['objects', $1], $2::jsonb)
		FROM role r JOIN role_assignment ra ON ra.role_id = r.id
		JOIN app_user u ON u.id = ra.user_id WHERE u.email = $3`,
		object, grant, importAdminEmail); err != nil {
		t.Fatalf("writing the narrowed role: %v", err)
	}
	if _, err := e.Owner.Exec(t.Context(), `
		UPDATE role_assignment SET role_id = (SELECT id FROM role WHERE key = 'importer_narrowed')
		WHERE user_id = (SELECT id FROM app_user WHERE email = $1)`, importAdminEmail); err != nil {
		t.Fatalf("assigning the narrowed role: %v", err)
	}
}

const (
	readOnlyGrant = `{"create": false, "read": true, "update": false, "delete": false}`
	noGrant       = `{"create": false, "read": false, "update": false, "delete": false}`
)

func TestATagTheRoleCannotCreateIsAProblemAndTheCompaniesLandWithoutIt(t *testing.T) {
	e := setupImportApp(t)
	actWithGrant(t, e, "tag", readOnlyGrant)
	tagsBefore := tagCount(t, e)
	profile, _ := uploadCSV(t, e, "company", "Name,Tags\nFresh GmbH,Brand New\n")
	run, status := createRunWithMapping(t, e, "company", profile.SourceRef, map[string]string{"Name": "display_name", "Tags": "tags"})
	if status != http.StatusAccepted {
		t.Fatalf("stage → %d, want 202", status)
	}
	report := readImportReport(t, e, run.ID)
	issue := issueOf(report, "permission", "tag:create")
	if issue == nil || !strings.Contains(issue.Reason, "create tags") {
		t.Fatalf("issues = %+v, want a permission issue naming tag creation", report.Issues)
	}
	if hasIssue(report, "new_tag", "Brand New") {
		t.Fatal("the report promises a tag the role cannot create")
	}
	approveAndRead(t, e, run.ID)
	if tags := companyTags(t, e, "Fresh GmbH"); len(tags) != 0 {
		t.Fatalf("tags = %v, want none: the role could not create the one named", tags)
	}
	if tagCount(t, e) != tagsBefore {
		t.Fatal("the commit created a tag the role holds no grant to create")
	}
}

func TestARoleWithoutTagReadStagesAndIsToldTheTagsAreNotImported(t *testing.T) {
	e := setupImportApp(t)
	actWithGrant(t, e, "tag", noGrant)
	profile, _ := uploadCSV(t, e, "company", "Name,Tags\nUntagged GmbH,Partner\n")
	run, status := createRunWithMapping(t, e, "company", profile.SourceRef, map[string]string{"Name": "display_name", "Tags": "tags"})
	if status != http.StatusAccepted {
		t.Fatalf("stage → %d, want 202: a missing tag grant is a problem, not a refusal", status)
	}
	report := readImportReport(t, e, run.ID)
	if issue := issueOf(report, "permission", "tag:read"); issue == nil || !strings.Contains(issue.Reason, "read tags") {
		t.Fatalf("issues = %+v, want a permission issue naming tag reading", report.Issues)
	}
	approveAndRead(t, e, run.ID)
	if id := recordField(t, e, "/v1/companies", "display_name", "Untagged GmbH", "id"); id == "" {
		t.Fatal("the company did not land")
	}
}

func TestARoleThatCannotCreateLeadsHasEveryLeadRowSkipped(t *testing.T) {
	e := setupImportApp(t)
	actWithGrant(t, e, "lead", readOnlyGrant)
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	run, status := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	if status != http.StatusAccepted {
		t.Fatalf("stage → %d, want 202", status)
	}
	report := readImportReport(t, e, run.ID)
	if report.Disposition.Skipped != 3 || report.Disposition.Created != 0 {
		t.Fatalf("disposition = %+v, want all three rows skipped", report.Disposition)
	}
	if !hasIssue(report, "permission", "lead:create") {
		t.Fatalf("issues = %+v, want the missing lead grant named", report.Issues)
	}
	approveAndRead(t, e, run.ID)
	if n := leadCount(t, e); n != 0 {
		t.Fatalf("%d leads landed, want none", n)
	}
}

func TestAnOwnerTheImporterCannotNameIsAProblemAndTheImporterOwnsTheRow(t *testing.T) {
	e := setupImportApp(t)
	const runner = "runner@seat.test"
	if _, err := e.Owner.Exec(t.Context(), `
		INSERT INTO app_user (id, email, display_name, status, is_agent) VALUES ($1, $2, 'Runner', 'active', true)`,
		ids.NewV7(), runner); err != nil {
		t.Fatalf("seeding an agent seat: %v", err)
	}
	profile, _ := uploadCSV(t, e, "company", "Name,Owner\nRunner Owned GmbH,"+runner+"\n")
	run, status := createRunWithMapping(t, e, "company", profile.SourceRef, map[string]string{"Name": "display_name", "Owner": "owner_email"})
	if status != http.StatusAccepted {
		t.Fatalf("stage → %d, want 202", status)
	}
	report := readImportReport(t, e, run.ID)
	if issue := issueOf(report, "permission", runner); issue == nil || hasIssue(report, "unknown_owner", runner) {
		t.Fatalf("issues = %+v, want one permission issue naming the seat, and no unknown owner", report.Issues)
	}
	approveAndRead(t, e, run.ID)
	var admin string
	if err := e.Owner.QueryRow(t.Context(), `SELECT id::text FROM app_user WHERE email = $1`, importAdminEmail).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	if owner := recordField(t, e, "/v1/companies", "display_name", "Runner Owned GmbH", "owner_id"); owner != admin {
		t.Fatalf("owner = %q, want the importer %q", owner, admin)
	}
}
