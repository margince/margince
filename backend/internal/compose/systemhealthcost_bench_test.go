// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package compose

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestSystemHealthReadCosts(t *testing.T) {
	for _, count := range []int{100, 10_000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			e := integration.Setup(t)
			integration.ApplyRiverSchema(t)
			system := principal.SystemActing(e.Admin(), "system:health_benchmark")
			var email string
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(t.Context(), `SELECT email FROM app_user WHERE id=$1`, e.Rep1).Scan(&email)
			}); err != nil {
				t.Fatal(err)
			}
			if err := SeedSystemHealthFixture(system, e.Pool, e.Rep1, email, min(count, 1000), clocktest.Now(t)); err != nil {
				t.Fatal(err)
			}
			inserter, err := jobs.NewTxInserter(quietTestLogger())
			if err != nil {
				t.Fatal(err)
			}
			if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
				for range count {
					if err := inserter.EnqueueTx(system, tx, CaptureClassifyArgs{}, &river.InsertOpts{ScheduledAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
						return err
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			server := newServer(e.Pool, quietTestLogger(), authHandlers{}, dealsHandlers{})
			reads := []struct {
				name string
				call func(http.ResponseWriter, *http.Request)
			}{
				{"compact", server.GetSystemHealth}, {"jobs_detail", server.GetJobHealth}, {"capture_detail", server.GetCaptureHealth}, {"intake_detail", server.GetExtensionIngestHealth}, {"recovery_detail", server.GetRecoveryHealth},
			}
			for _, read := range reads {
				ctx, meter := database.WithStatementMeter(e.Admin())
				start := time.Now()
				rec := httptest.NewRecorder()
				read.call(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))
				elapsed := time.Since(start)
				if rec.Code != http.StatusOK {
					t.Fatalf("%s returned %d: %s", read.name, rec.Code, rec.Body.String())
				}
				var compressed bytes.Buffer
				writer := gzip.NewWriter(&compressed)
				if _, err := writer.Write(rec.Body.Bytes()); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				cost := meter.Cost()
				if cost.Statements == 0 {
					t.Fatal("statement tracer did not observe the production read")
				}
				t.Logf("fixture=%d read=%s elapsed=%s statements=%d db=%s pool=%s bytes=%d gzip=%d", count, read.name, elapsed, cost.Statements, cost.DBTime, cost.PoolWait, rec.Body.Len(), compressed.Len())
			}
		})
	}
}
