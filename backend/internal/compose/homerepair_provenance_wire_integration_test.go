// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReviewedLegacyCorrectionsCrossTheRealPreviewProtocol(t *testing.T) {
	f := seedLegacyRepair(t, false)
	operator := repairOperator(t, f.e)
	handler := dataTaskHandlers{pool: f.e.Pool, clock: func() time.Time { return f.now }}
	response := httptest.NewRecorder()
	handler.PreviewHomeRepair(response, httptest.NewRequest(http.MethodPost, "/v1/admin/home-repair/preview", strings.NewReader(`{"source":"task","limit":100}`)).WithContext(operator))
	if response.Code != http.StatusOK {
		t.Fatalf("preview response %d: %s", response.Code, response.Body.String())
	}
	var wire crmcontracts.HomeRepairReview
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Counts.Normalize == nil || *wire.Counts.Normalize != 1 || len(wire.Tasks) != 1 || wire.Tasks[0].Action != "normalize_automatic_task" {
		t.Fatalf("wire lost the exact proposed correction: %+v", wire)
	}
	if wire.Tasks[0].Reason != "legacy_source_and_generated_date" {
		t.Fatalf("wire hid the proposed fields: %+v", wire.Tasks[0])
	}
	review, err := homeRepairReviewInput(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.e.DB().Tx(operator, func(tx pgx.Tx) error {
		digest, err := validateHomeRepairTx(operator, tx, review)
		if err == nil && digest != wire.Digest {
			t.Fatal("wire round trip changed the executed page")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if wire.Tasks[0].Id != crmcontracts.Id(f.task.UUID) || ids.UUID(wire.Tasks[0].Id).IsZero() {
		t.Fatal("preview lost the reviewed identity")
	}
}
