// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mailDealRepairActor is the identity the preview decides under, as the data
// task's executor does.
const mailDealRepairActor = "system:datatask:core/1792680000_captured_mail_filed_on_its_deal"

// PreviewMailDealRepair implements POST /admin/mail-deal-repair/preview: the
// registered repair, run on one page and rolled back, so an operator sees what
// it resolves and what it leaves.
func (h dataTaskHandlers) PreviewMailDealRepair(w http.ResponseWriter, r *http.Request) {
	if !admitOperator(w, r, "data_task", principal.ActionRead) {
		return
	}
	var in crmcontracts.PreviewMailDealRepairJSONBody
	if !httperr.Decode(w, r, &in) {
		return
	}
	after := ids.Nil
	if in.After != nil {
		after = ids.UUID(*in.After)
	}
	ctx, cancel := context.WithTimeout(r.Context(), healthReadTimeout)
	defer cancel()
	system := principal.SystemActing(ctx, mailDealRepairActor)
	var preview capture.DealRepairPreview
	err := database.WithWorkspaceTx(system, h.pool, func(tx pgx.Tx) error {
		var err error
		preview, err = mailDealRepair(h.pool).PreviewTx(system, tx, after, in.Limit)
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, mailDealRepairWire(preview))
}

func mailDealRepairWire(preview capture.DealRepairPreview) crmcontracts.MailDealRepairPreview {
	out := crmcontracts.MailDealRepairPreview{
		Inspected: preview.Inspected, Complete: preview.Complete, Outcomes: []crmcontracts.MailDealRepairOutcomeCount{},
	}
	for outcome, count := range preview.Outcomes {
		out.Outcomes = append(out.Outcomes, crmcontracts.MailDealRepairOutcomeCount{
			Count: count, Outcome: crmcontracts.MailDealRepairOutcomeCountOutcome(outcome),
		})
	}
	slices.SortFunc(out.Outcomes, func(a, b crmcontracts.MailDealRepairOutcomeCount) int {
		return strings.Compare(string(a.Outcome), string(b.Outcome))
	})
	if !preview.Last.IsZero() {
		last := openapi_types.UUID(preview.Last)
		out.NextAfter = &last
	}
	return out
}

// MailDealRepairPreviewHandler is the operator preview as the API serves it.
func MailDealRepairPreviewHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return dataTaskHandlers{pool: pool}.PreviewMailDealRepair
}
