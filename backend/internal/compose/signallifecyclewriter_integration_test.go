// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestExtractedLifecycleSignalStoresSourceProofWithoutCopyingTheQuote(t *testing.T) {
	e := setupScout(t)
	company := e.SeedCompany(t, "Maple Studio", nil)
	subject, body, direction := "Contract notice", "We are ending all services with your company.", "inbound"
	at := e.daysAgo(1)
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: company}},
	})
	if err != nil {
		t.Fatal(err)
	}
	source := ids.UUID(logged.Id)
	thread := settledThread{CompanyID: company, Messages: []threadMessage{{ID: source, Body: body, At: at}}}
	event := extractedEvent{
		Kind: signalKindContractEnded, MessageID: source.String(),
		Summary: "Maple ended its account relationship.", Quote: body, LifecycleScope: lifecycleScopeCompany,
	}
	extractor := &SignalExtractor{}
	if err := database.WithWorkspaceTx(e.system(), e.Pool, func(tx pgx.Tx) error {
		raised, err := extractor.recordExtractedEvent(e.system(), tx, thread, event, e.now)
		if err == nil && !raised {
			t.Fatal("the extracted event raised no signal")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var evidence []struct {
		Snippet        string             `json:"snippet"`
		LifecycleScope string             `json:"lifecycle_scope"`
		SourceQuote    *values.QuoteProof `json:"source_quote"`
	}
	stored := e.WsScalar(t, `SELECT evidence::text FROM signal WHERE kind = 'contract_ended' AND resolved_company_id = $1`, company)
	if err := json.Unmarshal([]byte(stored), &evidence); err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 || evidence[0].SourceQuote == nil || evidence[0].Snippet != event.Summary ||
		evidence[0].LifecycleScope != lifecycleScopeCompany {
		t.Fatalf("stored evidence=%+v", evidence)
	}
	if quote, ok := values.VerifyQuote(body, *evidence[0].SourceQuote); !ok || quote != body {
		t.Fatalf("source proof reconstructed %q, valid=%v", quote, ok)
	}
}
