// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"context"
	"log/slog"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func participantPathID(id openapi_types.UUID) ids.DealRoomParticipantID {
	return ids.DealRoomParticipantID{UUID: ids.UUID(id)}
}

// ListDealRoomParticipants returns the room's roster.
func (h Handlers) ListDealRoomParticipants(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ListDealRoomParticipantsParams) {
	activeOnly := params.ActiveOnly != nil && *params.ActiveOnly
	contacts, page, err := h.store.ListParticipants(r.Context(), pathID(id), activeOnly)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.DealRoomParticipantListResponse{
		Data: contacts,
		Page: pageInfo(page),
	})
}

// InviteDealRoomParticipant admits a named contact and returns their credential.
func (h Handlers) InviteDealRoomParticipant(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.InviteDealRoomParticipantRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in, err := inviteInput(req)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	issued, err := h.store.InviteParticipant(r.Context(), pathID(id), in)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, h.issuedBody(r, issued))
}

// ResendDealRoomInvitation issues a fresh credential, retiring the previous one.
func (h Handlers) ResendDealRoomInvitation(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, participantID openapi_types.UUID) {
	issued, err := h.store.ResendInvitation(r.Context(), pathID(id), participantPathID(participantID))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, h.issuedBody(r, issued))
}

// RevokeDealRoomParticipant takes a contact's access away.
func (h Handlers) RevokeDealRoomParticipant(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, participantID openapi_types.UUID) {
	participant, err := h.store.RevokeParticipant(r.Context(), pathID(id), participantPathID(participantID))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, participant)
}

// UpdateDealRoomParticipant corrects a participant's details.
func (h Handlers) UpdateDealRoomParticipant(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, participantID openapi_types.UUID) {
	var req crmcontracts.UpdateDealRoomParticipantRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in, err := participantUpdateInput(req)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	participant, err := h.store.UpdateParticipant(r.Context(), pathID(id),
		participantPathID(participantID), in)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, participant)
}

// issuedBody renders a freshly minted credential and hands it to the sender.
//
// Sending never fails the write. A relay outage leaves the seller a link to
// pass on by hand. `queued` says whether the relay took the mail within
// inviteRelayWait; its answer also lands on the roster, as sent or failed.
func (h Handlers) issuedBody(r *http.Request, issued IssuedInvitation) crmcontracts.DealRoomInvitationIssued {
	return crmcontracts.DealRoomInvitationIssued{
		Participant:         issued.Participant,
		Credential:          credentialFor(r, issued.Credential),
		CredentialExpiresAt: issued.ExpiresAt,
		Queued:              h.sendInviteAndWait(r, issued),
	}
}

// credentialFor is the buyer's link as the caller may see it. An agent never
// holds it, since the link signs in to the room as the buyer. It learns only
// whether the mail was queued.
func credentialFor(r *http.Request, credential string) string {
	if p, ok := principal.Actor(r.Context()); ok && p.Type == principal.PrincipalAgent {
		return ""
	}
	return credential
}

// recordSendOutcome stamps what the relay did onto the invitation it carried.
// It runs after the issuing transaction committed, so a slow relay never rolls
// back a participant. A failure to record is logged and swallowed: the
// credential is valid either way, and only the roster chip is lost.
func (h Handlers) recordSendOutcome(ctx context.Context, issued IssuedInvitation, sendErr error) {
	if err := h.inviteMail().RecordInvitationSend(ctx,
		participantPathID(issued.Participant.Id), issued.Credential, sendErr); err != nil {
		slog.ErrorContext(ctx, "recording deal room invitation delivery failed",
			"participant_id", issued.Participant.Id, "err", err)
	}
}

// PreviewDealRoom hands the seller a one-time credential for their own
// preview seat. The credential is in the body exactly once and nowhere else.
func (h Handlers) PreviewDealRoom(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	issued, err := h.store.PreviewRoom(r.Context(), pathID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, crmcontracts.DealRoomPreviewIssued{
		Credential:          issued.Credential,
		CredentialExpiresAt: issued.ExpiresAt,
	})
}
