// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
)

var updateDealPolicy = agentPolicy{Op: "updateDeal", Tool: "update_record", RecordType: recordTypeDeal}

// The joined line cannot be split back on a value holding ", " or "=".
// The parts carry such a value as one field, whole.
func TestTheSummaryPartsKeepAValueTheJoinedLineCannotSplit(t *testing.T) {
	body := []byte(`{"name":"Acme, Inc. renewal","notes":"a=b"}`)
	parts := restParts(inEnglish, updateDealPolicy, summaryRequest("PATCH", "/v1/deals/x"), body)

	want := []crmcontracts.ApprovalSummaryField{{Field: "name", Value: "Acme, Inc. renewal"}, {Field: "notes", Value: "a=b"}}
	if !slices.Equal(parts.Fields, want) {
		t.Fatalf("fields = %+v, want %+v", parts.Fields, want)
	}
	if parts.Sentence != "Update record deal" {
		t.Errorf("sentence = %q, want the act alone", parts.Sentence)
	}
	if got, line := summaryLine(inEnglish, parts), "Update record deal: name=Acme, Inc. renewal, notes=a=b"; got != line {
		t.Errorf("joined line = %q, want %q", got, line)
	}
}

// The parts list what the line lists, including the count past the limit, and
// the line is their join.
func TestTheSummaryPartsCountTheFieldsTheLineLeavesOut(t *testing.T) {
	body := []byte(`{"a":1,"b":2,"c":3,"d":4,"e":5,"f":6,"g":7,"h":8,"i":9,"j":10}`)
	parts := restParts(inEnglish, updateDealPolicy, summaryRequest("PATCH", "/v1/deals/x"), body)
	if len(parts.Fields) != summaryFieldLimit || parts.MoreFields != 2 {
		t.Fatalf("parts hold %d fields and %d more, want %d and 2", len(parts.Fields), parts.MoreFields, summaryFieldLimit)
	}
	want := "Update record deal: a=1, b=2, c=3, d=4, e=5, f=6, g=7, h=8, +2 more"
	if got := summaryLine(inEnglish, parts); got != want {
		t.Errorf("joined line = %q, want %q", got, want)
	}
}

// A nested create names its parent first, as a field like any other.
func TestTheSummaryPartsNameTheParentOfANestedCreate(t *testing.T) {
	pol := agentPolicy{Op: "createOffer", Tool: "create_record", RecordType: recordTypeOffer}
	req := summaryRequest("POST", "/v1/deals/deal-7/offers", [2]string{"id", "deal-7"})
	parts := restParts(inEnglish, pol, req, []byte(`{"currency":"EUR"}`))
	want := []crmcontracts.ApprovalSummaryField{{Field: inEnglish.createdUnder, Value: "deal-7"}, {Field: "currency", Value: "EUR"}}
	if !slices.Equal(parts.Fields, want) {
		t.Errorf("fields = %+v, want %+v", parts.Fields, want)
	}
}

// A resolver sentence that already says every value stores no parts: the
// summary is a sentence alone and every surface shows it whole.
func TestASentenceThatSaysEveryValueStoresNoParts(t *testing.T) {
	info := agents.StageInfo{Summary: "Rename deal to Acme renewal"}
	parts := stagedParts(info, inEnglish, updateDealPolicy, summaryRequest("PATCH", "/v1/deals/x"), []byte(`{"name":"Acme renewal"}`))
	if stored := stagedSummaryParts(parts); stored != nil {
		t.Errorf("stored parts = %+v, want none", stored)
	}
	if got := summaryLine(inEnglish, parts); got != info.Summary {
		t.Errorf("joined line = %q, want the sentence %q", got, info.Summary)
	}

	withField := stagedParts(info, inEnglish, updateDealPolicy, summaryRequest("PATCH", "/v1/deals/x"), []byte(`{"name":"Acme renewal","owner_id":null}`))
	stored := stagedSummaryParts(withField)
	if stored == nil || stored.Sentence != info.Summary || len(stored.Fields) != 1 || stored.Fields[0].Value != jsonNull {
		t.Errorf("stored parts = %+v, want the sentence and owner_id=null", stored)
	}
}
