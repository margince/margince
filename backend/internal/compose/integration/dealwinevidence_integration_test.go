// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The win-evidence read against the advance that enforces the same rule.
//
// The two cannot disagree. What the read calls unsigned, the advance refuses,
// and what it calls signed, the advance lets through. The agreement comes from
// the real contract writer. Its status and its file's parent are set directly,
// as seedDocument does, because the subject is the read.

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dealWithSignedDraft seeds a deal with an agreement that carries a signing
// date and a current contract file, and is still a draft.
func dealWithSignedDraft(t *testing.T, e *Env) (deal ids.UUID, contract, file ids.UUID, won ids.StageID) {
	t.Helper()
	company := e.SeedCompany(t, "Acme", nil)
	pipeline, open, won := DealFixture(t, e)
	deal = e.SeedDeal(t, "Warehouse rollout", pipeline, open, &e.Rep1)
	anchorOnCompany(t, e, deal, company)

	dealID := ids.From[ids.DealKind](deal)
	signed := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	created, err := e.Contracts.CreateContract(e.Admin(), contracts.CreateContractInput{
		CompanyID:  ids.From[ids.CompanyKind](company),
		DealID:     &dealID,
		Title:      "Master agreement",
		ValueBasis: contracts.BasisTotal,
		Source:     "manual",
		SignedOn:   &signed,
	})
	if err != nil {
		t.Fatalf("creating the agreement: %v", err)
	}
	contract = ids.UUID(created.Id)
	file = seedDocument(t, e, company, "deal", deal, "msa.pdf", "contract", false)
	e.WsExec(t, `UPDATE attachment SET contract_id = $2, doc_state = 'current' WHERE id = $1`, file, contract)
	return deal, contract, file, won
}

func TestTheWinEvidenceReadAgreesWithTheAdvanceAtEveryStepOfAnAgreement(t *testing.T) {
	e := Setup(t)
	deal, contract, file, won := dealWithSignedDraft(t, e)
	dealID := ids.From[ids.DealKind](deal)

	signed := func() bool {
		t.Helper()
		got, err := e.Deals.DealWinEvidence(e.Admin(), dealID)
		if err != nil {
			t.Fatalf("reading the win evidence: %v", err)
		}
		return got
	}
	refused := func() bool {
		t.Helper()
		_, err := e.Deals.AdvanceDeal(e.Admin(), dealID, deals.AdvanceDealInput{ToStageID: won})
		var missing *deals.WinEvidenceMissingError
		return errors.As(err, &missing)
	}

	// A draft has asserted nothing, even with a signing date and a file.
	if signed() || !refused() {
		t.Fatal("a draft agreement counted as the evidence of a win")
	}

	e.WsExec(t, `UPDATE contract SET status = 'active' WHERE id = $1`, contract)
	if !signed() {
		t.Fatal("an active agreement with a signing date and a current file did not count")
	}

	// Archive leaves the file in place; the read must look past it.
	e.WsExec(t, `UPDATE attachment SET archived_at = now() WHERE id = $1`, file)
	if signed() || !refused() {
		t.Fatal("an archived file still counted as the evidence of a win")
	}

	e.WsExec(t, `UPDATE attachment SET archived_at = NULL WHERE id = $1`, file)
	if !signed() || refused() {
		t.Fatal("restoring the file did not bring the evidence back for both")
	}
}

func TestTheWinEvidenceReadRefusesADealTheCallerCannotSee(t *testing.T) {
	e := Setup(t)
	deal, _, _, _ := dealWithSignedDraft(t, e)
	e.WsExec(t, `UPDATE deal SET archived_at = now() WHERE id = $1`, deal)

	_, err := e.Deals.DealWinEvidence(e.Admin(), ids.From[ids.DealKind](deal))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an archived deal's win evidence: err = %v, want not found", err)
	}
}
