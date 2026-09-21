// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Carrying the evidence a message was staged on to the phase that sends it.
//
// The transmit phase re-asks the whole question, which is the design: a thread
// can be archived and a deal can close while a delivery waits in the queue, so
// the answer must be taken again against the record as it is now. But re-asking
// needs the same INPUTS. stagedRequestFor rebuilt the question from the claimed
// category and the thread key alone, so a message supported by a named invoice
// or contract arrived at transmit with a zero id, found no document, and fell
// through to whatever the legacy purpose key said.
//
// While the transactional class allowed unconditionally that fall-through hid
// the loss: the invoice went out on the purpose key rather than on the invoice,
// and the two answers agreed by accident. They no longer do.
//
// WHAT IS CARRIED IS THE POINTER, NEVER THE VERDICT. An id is a question to ask
// again — is this invoice still live, is this contact still employed there — and
// every one of those checks runs at transmit exactly as it ran at staging. The
// resolution itself is deliberately not carried, for the reason stagedClaims
// gives: it would let a message ride an answer the record no longer supports.

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// stagedEvidence is the wire shape of communication_decision.evidence.
//
// Written with explicit json tags and omitempty so a row carries only the ids
// that were actually named: an absent key and a zero uuid mean the same thing
// to the reader, and the sparse form is what makes the column readable to a
// human looking at a decision row.
type stagedEvidence struct {
	ActivityID     *ids.UUID `json:"activity_id,omitempty"`
	DealID         *ids.UUID `json:"deal_id,omitempty"`
	InvoiceID      *ids.UUID `json:"invoice_id,omitempty"`
	ContractID     *ids.UUID `json:"contract_id,omitempty"`
	ConsentEventID *ids.UUID `json:"consent_event_id,omitempty"`
	BasisID        *ids.UUID `json:"basis_id,omitempty"`
}

// evidenceJSON renders the evidence a caller named for storage on the decision
// row. An empty Evidence renders as {}, which is the column's own default.
func evidenceJSON(e commsauthz.Evidence) ([]byte, error) {
	out, err := json.Marshal(stagedEvidence{
		ActivityID:     nonZeroID(e.ActivityID),
		DealID:         nonZeroID(e.DealID),
		InvoiceID:      nonZeroID(e.InvoiceID),
		ContractID:     nonZeroID(e.ContractID),
		ConsentEventID: nonZeroID(e.ConsentEventID),
		BasisID:        nonZeroID(e.BasisID),
	})
	if err != nil {
		return nil, fmt.Errorf("consent: render the staged evidence: %w", err)
	}
	return out, nil
}

// evidenceFrom reads back what evidenceJSON wrote.
//
// A row whose evidence cannot be parsed yields NO evidence rather than an
// error. Every id it holds is re-validated at transmit anyway, so the worst an
// unreadable column can do is send the message down the same path it took
// before this file existed — a review naming what could not be evidenced.
// Failing the dispatch instead would turn one malformed row into a stuck
// delivery.
func evidenceFrom(raw []byte) commsauthz.Evidence {
	if len(raw) == 0 {
		return commsauthz.Evidence{}
	}
	var in stagedEvidence
	if err := json.Unmarshal(raw, &in); err != nil {
		return commsauthz.Evidence{}
	}
	return commsauthz.Evidence{
		ActivityID:     derefID(in.ActivityID),
		DealID:         derefID(in.DealID),
		InvoiceID:      derefID(in.InvoiceID),
		ContractID:     derefID(in.ContractID),
		ConsentEventID: derefID(in.ConsentEventID),
		BasisID:        derefID(in.BasisID),
	}
}

func nonZeroID(id ids.UUID) *ids.UUID {
	if id.IsZero() {
		return nil
	}
	return &id
}

func derefID(id *ids.UUID) ids.UUID {
	if id == nil {
		return ids.UUID{}
	}
	return *id
}
