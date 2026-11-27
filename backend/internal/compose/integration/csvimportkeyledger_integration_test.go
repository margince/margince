// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The run's key ledger outlives one commit slice. Each slice builds fresh
// writers. A key repeated past a slice boundary, or past a failed attempt, is
// met through the rows that landed before it.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

var companyDomainParts = []AnyMap{{"file": "companies.csv", "object": "company", "mapping": AnyMap{
	"ref": "ref", "name": "display_name", "domain": "domain",
}}}

func TestADomainRepeatedPastACommitSliceIsRefusedAsTheTestRunSaid(t *testing.T) {
	t.Parallel()
	e := setupImportApp(t)
	var file strings.Builder
	file.WriteString("ref,name,domain\nk0,Twin Holder,twin-boundary.test\n")
	for i := 1; i < 500; i++ {
		fmt.Fprintf(&file, "k%d,Filler %d,f%d.boundary.test\n", i, i, i)
	}
	file.WriteString("k500,Twin Again,twin-boundary.test\n")
	files := map[string]string{"companies.csv": file.String()}
	run := stageBundle(t, e, uploadZip(t, e, "slices.zip", zipBytes(t, files)).SourceRef, "slices", companyDomainParts)
	predicted := verdicts(readImportReport(t, e, run.ID))
	want := []string{"created=500 updated=0 unchanged=0 skipped=1", "companies.csv:502:domain_held"}
	if !slices.Equal(predicted, want) {
		t.Fatalf("test run = %v, want %v", predicted, want)
	}
	approveAndRead(t, e, run.ID)
	if committed := verdicts(readImportReport(t, e, run.ID)); !slices.Equal(committed, predicted) {
		t.Fatalf("import = %v, test run said %v", committed, predicted)
	}
}

// failAfterTheFirstCreate leaves the run as an attempt that created its first
// company and died past it.
func failAfterTheFirstCreate(t *testing.T, e *apptest.AppEnv, runID, sourceName, ref, company string) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), apptest.InstallationWorkspaceUUID(context.Background(), t, e.Pool))
	user := ids.NewV7()
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{migration.ImportRunObject: {Create: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	run, err := ids.Parse(runID)
	if err != nil {
		t.Fatal(err)
	}
	native, err := ids.Parse(company)
	if err != nil {
		t.Fatal(err)
	}
	system := provenance.ReservedSourceSystemPrefix + migration.ConnectorCSV + ":" + sourceName
	if err := database.WithWorkspaceTx(ctx, e.DB().Pool(), func(tx pgx.Tx) error {
		if err := migration.NewRunStore(e.DB()).RecordIdentityTx(ctx, tx, migration.RunID(run),
			system, migration.ObjectCompany, ref, native, migration.OriginCreated); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE import_run SET status = 'failed', checkpoint = 1, approved_at = now() WHERE id = $1`, runID)
		return err
	}); err != nil {
		t.Fatalf("failing the run after its first row: %v", err)
	}
}

func TestAResumedCommitRefusesADomainAnEarlierAttemptLanded(t *testing.T) {
	t.Parallel()
	e := setupImportApp(t)
	files := map[string]string{"companies.csv": "ref,name,domain\nk1,Resume One,resume.test\nk2,Resume Two,resume.test\n"}
	run := stageBundle(t, e, uploadZip(t, e, "resume.zip", zipBytes(t, files)).SourceRef, "resume", companyDomainParts)
	if predicted := readImportReport(t, e, run.ID); !hasReasonCode(predicted, "domain_held") {
		t.Fatalf("test run issues = %+v, want the second row refused as domain_held", predicted.Issues)
	}
	first := createdID(t, e, "/v1/companies", AnyMap{
		"source": "manual", "display_name": "Resume One", "domains": []AnyMap{{"domain": "resume.test", "is_primary": true}},
	})
	failAfterTheFirstCreate(t, e, run.ID, "resume", "k1", first)
	approveAndRead(t, e, run.ID)
	committed := readImportReport(t, e, run.ID)
	if !hasReasonCode(committed, "domain_held") || hasReasonCode(committed, "company_domain_matched") {
		t.Fatalf("import issues = %+v, want the second row refused, not filed onto the first", committed.Issues)
	}
}

// A lead's or contact's LinkedIn URL keeps the case of its path in the store,
// so the test run must not fold it before asking.
func TestALeadRowMeetsAMixedCaseLinkedInURLAsTheStoreDoes(t *testing.T) {
	t.Parallel()
	e := setupImportApp(t)
	createdID(t, e, "/v1/leads", AnyMap{
		"source": "manual", "full_name": "Stored Lee", "email": "stored-lee@case.test",
		"linkedin_url": "https://www.linkedin.com/in/Mixed-Case-Lee",
	})
	createdID(t, e, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Stored Con",
		"linkedin_url": "https://www.linkedin.com/in/Mixed-Case-Con",
	})
	files := map[string]string{"leads.csv": "ref,full_name,email,linkedin\n" +
		"l1,Lee File,lee-file@case.test,https://www.linkedin.com/in/Mixed-Case-Lee\n" +
		"l2,Con File,con-file@case.test,https://www.linkedin.com/in/Mixed-Case-Con\n"}
	parts := []AnyMap{{"file": "leads.csv", "object": "lead", "mapping": AnyMap{
		"ref": "ref", "full_name": "full_name", "email": "email", "linkedin": "linkedin_url",
	}}}
	run := stageBundle(t, e, uploadZip(t, e, "case.zip", zipBytes(t, files)).SourceRef, "case", parts)
	predicted := verdicts(readImportReport(t, e, run.ID))
	want := []string{"created=0 updated=0 unchanged=0 skipped=2", "leads.csv:2:linkedin_held", "leads.csv:3:contact_holds_lead"}
	if !slices.Equal(predicted, want) {
		t.Fatalf("test run = %v, want %v", predicted, want)
	}
	approveAndRead(t, e, run.ID)
	if committed := verdicts(readImportReport(t, e, run.ID)); !slices.Equal(committed, predicted) {
		t.Fatalf("import = %v, test run said %v", committed, predicted)
	}
}
