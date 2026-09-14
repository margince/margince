// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestEmploymentWorkerReconcilesRetainedClaimsWithoutVendorAccess(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	created, err := store.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Worker sample", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	contact := ids.From[ids.ContactKind](ids.UUID(created.Id))
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		args := []any{contact}
		var run string
		if err := tx.QueryRow(e.Admin(), storekit.SQLf(`INSERT INTO provider_run(subject_kind,contact_id,provider,trigger,state,input_fingerprint,external_correlation_id,connection_version,connection_epoch,configuration_snapshot,requested_categories,completed_at)
 VALUES('contact',$%d,'surfe','manual','completed','worker-test',gen_random_uuid(),1,1,'{}',ARRAY['job_history'],now()) RETURNING id::text`, len(args)), args...).Scan(&run); err != nil {
			return err
		}
		return contacts.WriteProviderClaims(e.Admin(), tx, run, contact.String(), "surfe", []provider.Claim{{Key: provider.ClaimCurrentEmployment, Value: []byte(`{"company_name":"Worker Employer","company_domain":"worker-employer.example","job_title":"Lead"}`)}}, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	}); err != nil {
		t.Fatal(err)
	}
	worker := employmentImportWorker{pool: e.Pool}
	if err := worker.Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	report, err := store.PreviewEmploymentImport(e.Admin(), contact)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 || report.Items[0].State != "linked" {
		t.Fatalf("worker did not link retained evidence: %+v", report)
	}
	if err := worker.Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	replay, err := store.PreviewEmploymentImport(e.Admin(), contact)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.Items) != 1 || replay.Items[0].RelationshipId == nil || *replay.Items[0].RelationshipId != *report.Items[0].RelationshipId {
		t.Fatalf("worker replay changed the link: %+v", replay)
	}
}
