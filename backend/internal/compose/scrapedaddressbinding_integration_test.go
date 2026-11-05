// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestWebsiteAddressWritersQueueThroughTheConfiguredProductionBinding(t *testing.T) {
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	inserter, err := jobs.NewTxInserter(logger)
	if err != nil {
		t.Fatal(err)
	}
	geocodeBinding.mu.Lock()
	previous := geocodeBinding.enqueue
	geocodeBinding.enqueue = nil
	geocodeBinding.mu.Unlock()
	t.Cleanup(func() {
		geocodeBinding.mu.Lock()
		geocodeBinding.enqueue = previous
		geocodeBinding.mu.Unlock()
	})
	server := Server{contactsStore: e.Contacts, contactsHandlers: contacts.NewHandlers(e.DB()), log: logger}
	server.wireOnboardingSurface(e.Pool)
	WithDeepRead(inserter, nil)(&server, e.Pool)
	stores := map[string]*contacts.Store{
		"approval":          newCounterpartyStore(e.Pool),
		"company form":      server.companyHandlers.store,
		"site confirmation": server.siteReadHandlers.engine.contacts,
	}
	// Options can be applied after their stores were constructed.
	WithGeocoding(inserter)(&server, e.Pool)
	for name, store := range stores {
		row, err := store.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: name + " address", Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.CompanyKind](ids.UUID(row.Id))
		const address = "Example Street 1, 12345 Example City, Germany"
		if err := store.ApplyEnrichment(e.Admin(), id, contacts.ApplyColdStartProfileInput{
			SourceURL: "https://binding.example.test/imprint",
			Fields: []contacts.ColdStartFieldInput{{
				Field: "registered_address", Value: address,
				SourceURL: "https://binding.example.test/imprint", EvidenceSnippet: address, Confidence: 0.9,
			}},
		}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var queued int
		if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(), `SELECT count(*) FROM river_job
				WHERE kind = 'geocode_company' AND args->>'company_id' = $1`, id.String()).Scan(&queued)
		}); err != nil {
			t.Fatal(err)
		}
		if queued != 1 {
			t.Errorf("%s queued %d geocode jobs, want one", name, queued)
		}
	}
}
