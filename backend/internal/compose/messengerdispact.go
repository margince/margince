// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"net/http"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// ConnectDispactParticipant verifies personal identity before sealing a Dispact token.
func (s Server) ConnectDispactParticipant(w http.ResponseWriter, r *http.Request) {
	if _, ok := signedInHuman(w, r); !ok {
		return
	}
	if !auth.SeatAdmitsWrites(r.Context()) {
		httperr.Write(w, r, apperrors.ErrSeatTierInsufficient)
		return
	}
	h := s.messengerOAuthHandlers
	if h.dispact == nil || h.store == nil {
		chatlinks.WriteMessengerFailure(w, r, messaging.ErrUnsupported)
		return
	}
	var in crmcontracts.ConnectDispactParticipantJSONRequestBody
	if !httperr.Decode(w, r, &in) {
		return
	}
	if in.PersonalToken == nil || !strings.HasPrefix(*in.PersonalToken, "pat_") || len(*in.PersonalToken) < 5 || len(*in.PersonalToken) > 8192 {
		httperr.Write(w, r, apperrors.ErrInvalidArgument)
		return
	}
	installation, err := h.dispact(r.Context())
	if err != nil {
		chatlinks.WriteMessengerFailure(w, r, messaging.ErrUnsupported)
		return
	}
	identity, err := dispact.NewParticipant(h.http, installation.Address, *in.PersonalToken, installation.TeamID, "").Identity(r.Context())
	if err != nil {
		chatlinks.WriteMessengerFailure(w, r, err)
		return
	}
	credential, err := json.Marshal(messaging.Credential{AccessToken: *in.PersonalToken, InstallationID: installation.ConnectionID.String()}) // #nosec G117 -- Encoded only for encrypted credential-vault storage.
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	_, err = h.store.ConnectMember(r.Context(), chatlinks.MemberGrant{Provider: chatlinks.ProviderDispact, Workspace: identity.Workspace, Member: identity.Member.ID, Address: identity.Member.Email}, credential)
	if err != nil {
		chatlinks.WriteMessengerFailure(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
