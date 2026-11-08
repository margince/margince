// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The model cites a context item and a product by the refs the prompt shows.
// The orchestrator reads them back to ids and refuses a ref it never showed.
func TestOfferDraftNamesRecordsByRefAndReadsThemBack(t *testing.T) {
	product := crmcontracts.Product{
		Id: openapi_types.UUID(ids.NewV7()), Name: "Support Plan", UnitPriceMinor: 5000, Currency: "EUR",
	}
	source := "activity:" + ids.NewV7().String()
	dealContext := []dealContextItem{{SourceID: source, Snippet: "Client wants the support plan."}}
	brain := &replyBrainStub{response: model.Response{Text: `{"lines":[
		{"description":"Support","quantity":"1","tax_rate":"19.00","evidence_snippet":"wants the support plan","source_id":"s1","product_id":"p1"},
		{"description":"Other","quantity":"1","tax_rate":"19.00","evidence_snippet":"wants the support plan","source_id":"s2","product_id":"p2"}]}`}}

	candidates, err := offerDrafter{brain: brain}.draftCandidates(
		context.Background(), dealContext, []crmcontracts.Product{product}, string(textlang.English))
	if err != nil {
		t.Fatalf("draftCandidates: %v", err)
	}

	prompt := brain.request.Messages[0].Content
	if strings.Contains(prompt, source) || strings.Contains(prompt, product.Id.String()) {
		t.Errorf("the prompt shows a record id:\n%s", prompt)
	}
	if got := candidates[0]; got.SourceID != source || got.ProductID != product.Id.String() {
		t.Errorf("s1/p1 read back as %q/%q, want %q/%q", got.SourceID, got.ProductID, source, product.Id)
	}
	if got := candidates[1]; got.SourceID != "" || got.ProductID != "" {
		t.Errorf("s2/p2 were never shown, yet read back as %q/%q", got.SourceID, got.ProductID)
	}
}
