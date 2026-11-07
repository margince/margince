// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	chi "github.com/go-chi/chi/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// contactCaseMergeCommand decodes POST /v1/dedupe/contact-cases/{contact_id}/merge.
//
//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func contactCaseMergeCommand(_ agentPolicy, deps restCommandDeps, r *http.Request, body []byte) (agents.GovernedCall, error) {
	return contactCaseCall(deps, r, body, false)
}

// contactCasePreviewCommand decodes POST /v1/dedupe/contact-cases/{contact_id}/preview.
//
//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func contactCasePreviewCommand(_ agentPolicy, deps restCommandDeps, r *http.Request, body []byte) (agents.GovernedCall, error) {
	return contactCaseCall(deps, r, body, true)
}

// contactCaseCall binds one case decision. The routed contact only addresses
// the case; the approval binds to the survivor the body names.
//
//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func contactCaseCall(deps restCommandDeps, r *http.Request, body []byte, preview bool) (agents.GovernedCall, error) {
	contactID, err := ids.Parse(chi.URLParam(r, "contact_id"))
	if err != nil {
		return nil, apperrors.ErrNotFound
	}
	in, err := commandBody[crmcontracts.ContactCaseDecisionRequest](body)
	if err != nil {
		return nil, err
	}
	survivorID := ids.UUID(in.SurvivorId)
	if err := httperr.RequireBodyID("survivor_id", survivorID); err != nil {
		return nil, err
	}
	memberIDs := make([]ids.UUID, 0, len(in.MemberIds))
	for _, id := range in.MemberIds {
		memberIDs = append(memberIDs, ids.UUID(id))
	}
	return agents.NewContactCaseCall(deps.records, deps.language, agents.ContactCaseCommand{
		ContactID: contactID, Revision: in.Revision, MemberIDs: memberIDs,
		SurvivorID: survivorID, OwnerID: ids.UUID(in.OwnerId), Preview: preview,
	}), nil
}
