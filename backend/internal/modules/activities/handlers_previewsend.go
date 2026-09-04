// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The two preview transports, one per send door.
//
// Two handlers rather than one with a nullable anchor, because the two send
// doors are two operations in the contract and a preview that did not mirror
// them would answer about a message shape no endpoint accepts.

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// PreviewSendAuthorization implements POST /activities/{id}/send-email:preview.
func (h Handlers) PreviewSendAuthorization(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.PreviewSendRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in, err := previewInputFrom(req.To, (*string)(req.CommunicationContext), req.MarketingPurpose, req.ConsentPurpose, req.Evidence)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	h.answerPreview(w, r, FromActivity(pathID[ids.ActivityKind](id)), in)
}

// PreviewAccountSendAuthorization implements POST /emails:preview.
func (h Handlers) PreviewAccountSendAuthorization(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.PreviewAccountSendRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	links := linkInputsOf(&req.Links)
	if len(links) == 0 {
		// The same refusal the account send gives, for the same reason: nothing
		// in this stack validates a request against the schema's minItems, and
		// an origin with no links resolves to NoSendOriginError — which reads
		// as a composition defect rather than the caller's missing field.
		writeStoreErr(w, r, &RequiredFieldError{Field: fieldLinks})
		return
	}
	in, err := previewInputFrom(req.To, (*string)(req.CommunicationContext), req.MarketingPurpose, req.ConsentPurpose, req.Evidence)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	h.answerPreview(w, r, FromAccount(links), in)
}

// answerPreview is the half both doors share.
func (h Handlers) answerPreview(w http.ResponseWriter, r *http.Request, origin SendOrigin, in PreviewSendInput) {
	set, err := h.store.PreviewSend(r.Context(), origin, in, h.preview)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, previewResponse(set))
}

// previewInputFrom decodes the claim through sendContextFrom, the same decoder
// the real send uses, so a category this preview accepts is one the send
// accepts and a controller-only category is refused identically.
func previewInputFrom(to []openapi_types.Email, category, marketing, purpose *string, evidence *crmcontracts.CommunicationEvidence) (PreviewSendInput, error) {
	claimed, err := sendContextFrom(category, marketing, nil, evidence)
	if err != nil {
		return PreviewSendInput{}, err
	}
	recipients := make([]string, 0, len(to))
	for _, addr := range to {
		recipients = append(recipients, string(addr))
	}
	return PreviewSendInput{
		Recipients:       recipients,
		Context:          claimed.category,
		LegacyPurposeKey: legacyPurposeOf(purpose),
		MarketingPurpose: claimed.marketing,
		Evidence:         claimed.evidence,
	}, nil
}

// previewResponse renders the decision set onto the wire.
//
// The mode travels with each recipient because it changes what the answer
// MEANS: under observe a deny still sends, and a composer that drew it as a
// refusal would be showing a rollout position as a rule.
func previewResponse(set commsauthz.DecisionSet) crmcontracts.SendAuthorizationPreview {
	out := crmcontracts.SendAuthorizationPreview{
		Allowed:    set.Allowed(),
		Recipients: make([]crmcontracts.SendAuthorizationPreviewRecipient, 0, len(set.Decisions)),
	}
	for _, d := range set.Decisions {
		entry := crmcontracts.SendAuthorizationPreviewRecipient{
			Address:    openapi_types.Email(d.Recipient.Email),
			Verdict:    crmcontracts.SendAuthorizationPreviewRecipientVerdict(d.Verdict),
			ReasonCode: d.ReasonCode,
		}
		if d.Resolved != "" {
			resolved := crmcontracts.CommunicationContext(d.Resolved)
			entry.ResolvedCategory = &resolved
		}
		if d.Basis != "" {
			basis := string(d.Basis)
			entry.Basis = &basis
		}
		if d.Mode != "" {
			mode := crmcontracts.SendAuthorizationPreviewRecipientMode(d.Mode)
			entry.Mode = &mode
		}
		out.Recipients = append(out.Recipients, entry)
	}
	return out
}
