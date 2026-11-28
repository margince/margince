// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// GetMessengerService reports the same workspace boundary enforced by both credential writers.
func (s Server) GetMessengerService(w http.ResponseWriter, r *http.Request) {
	if _, ok := signedInHuman(w, r); !ok {
		return
	}
	if s.links == nil {
		httperr.ServiceUnavailable(w, r, "Team messaging is unavailable.")
		return
	}
	providers, err := s.links.MessengerProviders(r.Context())
	if err != nil {
		chatlinks.WriteMessengerFailure(w, r, err)
		return
	}
	selected, err := s.links.SelectedMessengerProvider(r.Context())
	if err != nil {
		chatlinks.WriteMessengerFailure(w, r, err)
		return
	}
	out := crmcontracts.MessengerService{Providers: []crmcontracts.MessengerProvider{}}
	if selected == chatlinks.ProviderSlack && s.historyProfile != nil {
		profile, err := s.historyProfile(r.Context(), selected)
		if err != nil {
			chatlinks.WriteMessengerFailure(w, r, err)
			return
		}
		out.HistoryProfile = chatlinks.WireHistoryProfile(profile)
	}
	if selected != "" {
		active := crmcontracts.MessengerProvider(selected)
		out.ActiveProvider = &active
	}
	for _, provider := range providers {
		out.Providers = append(out.Providers, crmcontracts.MessengerProvider(provider))
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// DisconnectMessengerService revokes shared and personal access through their existing owners.
func (s Server) DisconnectMessengerService(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider) {
	if _, ok := signedInHuman(w, r); !ok {
		return
	}
	if s.links == nil {
		httperr.ServiceUnavailable(w, r, "Team messaging is unavailable.")
		return
	}
	ctx := r.Context()
	if !auth.SeatAdmitsWrites(ctx) {
		httperr.Write(w, r, apperrors.ErrSeatTierInsufficient)
		return
	}
	if err := auth.Require(ctx, "channel_connection", principal.ActionDelete); err != nil {
		httperr.Write(w, r, err)
		return
	}

	selected, err := s.links.MessengerSelection(ctx)
	if err != nil {
		chatlinks.WriteMessengerFailure(w, r, err)
		return
	}
	if selected.Provider != "" && selected.Provider != string(provider) {
		httperr.Write(w, r, apperrors.ErrConflict)
		return
	}
	selection, err := s.links.SelectMessengerProvider(ctx, "", selected)
	if err != nil {
		chatlinks.WriteMessengerFailure(w, r, err)
		return
	}
	if err := s.DisconnectTeamChatProvider(ctx, string(provider), selection); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := s.links.DisconnectProviderMembers(ctx, string(provider), selection); err != nil {
		chatlinks.WriteMessengerFailure(w, r, err)
		return
	}
	if err := s.links.EndProviderLinks(ctx, string(provider), selection); err != nil {
		chatlinks.WriteMessengerFailure(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SelectMessengerService fixes the choice first so stale callbacks cannot reconnect the previous service.
func (s Server) SelectMessengerService(w http.ResponseWriter, r *http.Request) {
	if _, ok := signedInHuman(w, r); !ok {
		return
	}
	if s.links == nil {
		httperr.ServiceUnavailable(w, r, "Team messaging is unavailable.")
		return
	}
	if !auth.SeatAdmitsWrites(r.Context()) {
		httperr.Write(w, r, apperrors.ErrSeatTierInsufficient)
		return
	}
	var in crmcontracts.SelectMessengerServiceJSONRequestBody
	if !httperr.Decode(w, r, &in) {
		return
	}
	if !validMessengerServiceProfile(in, s.chatSourceHandlers.now()) {
		httperr.Write(w, r, apperrors.ErrInvalidArgument)
		return
	}
	selection, err := s.links.SelectMessengerProvider(r.Context(), string(in.Provider))
	if err != nil {
		chatlinks.WriteMessengerFailure(w, r, err)
		return
	}
	if in.SlackDistribution != nil {
		if err := s.links.SetWorkspaceHistoryProfile(r.Context(), selection, *in.SlackDistribution, s.chatSourceHandlers.now()); err != nil {
			chatlinks.WriteMessengerFailure(w, r, err)
			return
		}
	}
	providers, err := s.links.MessengerProviders(r.Context())
	if err != nil {
		chatlinks.WriteMessengerFailure(w, r, err)
		return
	}
	for _, provider := range providers {
		if provider == string(in.Provider) {
			continue
		}
		if err := s.DisconnectTeamChatProvider(r.Context(), provider, selection); err != nil {
			httperr.Write(w, r, err)
			return
		}
		if err := s.links.DisconnectProviderMembers(r.Context(), provider, selection); err != nil {
			chatlinks.WriteMessengerFailure(w, r, err)
			return
		}
		if err := s.links.EndProviderLinks(r.Context(), provider, selection); err != nil {
			chatlinks.WriteMessengerFailure(w, r, err)
			return
		}

	}
	w.WriteHeader(http.StatusNoContent)
}

func validMessengerServiceProfile(in crmcontracts.SelectMessengerServiceJSONRequestBody, at time.Time) bool {
	if in.SlackDistribution == nil {
		return true
	}
	if in.Provider != crmcontracts.MessengerProvider("slack") {
		return false
	}
	_, err := chatwire.SlackHistoryProfile(*in.SlackDistribution, at)
	return err == nil
}
