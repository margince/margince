// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The win-evidence read against the advance that enforces the same rule.
//
// The two cannot disagree. What the read calls unsigned, the advance refuses,
// and what it calls signed, the advance lets through. The agreement comes from
// the real contract writer and its status from the real transition. The file's
// parent is set directly, as seedDocument does, because the subject is the read.

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

	if _, err := e.Contracts.ChangeStatus(e.Admin(), ids.From[ids.ContractKind](contract), contracts.StatusActive, nil); err != nil {
		t.Fatalf("activating the agreement: %v", err)
	}
	if !signed() {
		t.Fatal("an active agreement with a signing date and a current file did not count")
	}

	// Archive leaves the file in place; both must look past it.
	e.WsExec(t, `UPDATE attachment SET archived_at = now() WHERE id = $1`, file)
	if signed() || !refused() {
		t.Fatal("an archived file still counted as the evidence of a win")
	}

	// With the file back, the advance lands, which ends the sequence.
	e.WsExec(t, `UPDATE attachment SET archived_at = NULL WHERE id = $1`, file)
	if !signed() || refused() {
		t.Fatal("restoring the file did not bring the evidence back for both")
	}
}

func TestTheWinEvidenceReadFollowsTheDealReadsReach(t *testing.T) {
	e := Setup(t)
	deal, _, _, _ := dealWithSignedDraft(t, e)
	dealID := ids.From[ids.DealKind](deal)

	// A deal is readable by every seat holding the grant, so another team's rep
	// is answered as the deal read itself would be.
	for _, rep := range []struct{ who, team ids.UUID }{{e.Rep2, e.Team1}, {e.Rep3, e.Team2}} {
		if _, err := e.Deals.DealWinEvidence(e.As(rep.who, []ids.UUID{rep.team}, RepPerms), dealID); err != nil {
			t.Errorf("a rep holding the deal grant: %v, want an answer", err)
		}
	}

	// A member with no deal grant is refused before any row is looked at.
	noGrant := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
	if _, err := e.Deals.DealWinEvidence(noGrant, dealID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a member without deal read: err = %v, want permission denied", err)
	}

	e.WsExec(t, `UPDATE deal SET archived_at = now() WHERE id = $1`, deal)
	if _, err := e.Deals.DealWinEvidence(e.Admin(), dealID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an archived deal: err = %v, want not found", err)
	}
}
