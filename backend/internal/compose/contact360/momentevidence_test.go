// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A follow-up card cites the row the last-touch query picked, by id. A
// timeline row at the same instant is a different message and must not be
// cited; the picked row need not be on the timeline page at all.
func TestTheFollowUpCardCitesThePickedRowNotATimeMatch(t *testing.T) {
	sent := now.AddDate(0, 0, -20)
	picked := openapi_types.UUID{1}
	sameInstant := crmcontracts.Activity{Id: openapi_types.UUID{2}, OccurredAt: sent, Subject: new("Another mail at the same second")}
	page := &crmcontracts.Contact360{
		LastOutboundAt: &sent, LastOutboundActivityId: &picked,
		Activities: &struct {
			Data []crmcontracts.Activity `json:"data"`
			Page crmcontracts.PageInfo   `json:"page"`
		}{Data: []crmcontracts.Activity{sameInstant}},
	}
	quiet, ok := momentCopyFor("en").goneQuietMoment(readerCtx(), now, page)
	if !ok {
		t.Fatal("no gone-quiet moment")
	}
	cited := quiet.Evidence[0]
	if cited.Id == nil || *cited.Id != picked {
		t.Fatalf("evidence cites %v, want the picked row %v", cited.Id, picked)
	}
	if cited.Label == *sameInstant.Subject {
		t.Errorf("evidence borrowed the subject of a different row at the same instant")
	}

	// The picked row on the page lends its subject; the fingerprint stays the
	// same whether or not the row is on the page.
	onPage := crmcontracts.Activity{Id: picked, OccurredAt: sent, Subject: new("Renewal")}
	page.Activities.Data = append(page.Activities.Data, onPage)
	shown, _ := momentCopyFor("en").goneQuietMoment(readerCtx(), now, page)
	if shown.Evidence[0].Label != "Renewal" {
		t.Errorf("label = %q, want the picked row's subject", shown.Evidence[0].Label)
	}
	if shown.EvidenceFingerprint != quiet.EvidenceFingerprint {
		t.Error("the fingerprint changed with the timeline page; a dismissal would re-arm on pagination")
	}
}
