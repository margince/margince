// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The consent flow of a unit's OAuth app. Connect answers the vendor's consent
// URL; the callback keeps the grant and hands the unit the vendor's extras.
//
// It reuses the capture flow's signer, CSRF nonce, state TTL and live-authority
// check. The grant is the workspace's, so only an admin may make it.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/pkg/extension"
)

// extOAuthMounted reports whether this deployment can sign a state and build a
// callback URL, which every consent flow needs.
func (h connectorHandlers) extOAuthMounted() bool {
	return h.signer.usable() && h.publicBaseURL != ""
}

func (h connectorHandlers) grantsFor(unit extension.Name) extOAuthGrants {
	if h.extOAuthGrants != nil {
		return h.extOAuthGrants(string(unit))
	}
	return newExtOAuthGrants(string(unit), boundExtensionRuntime())
}

func (h connectorHandlers) extCallbackURL(provider string) string {
	return extOAuthCallbackURL(h.apiBaseURL, h.publicBaseURL, provider)
}

// ConnectExtOauth implements (POST /ext-oauth/{provider}/connect).
func (h connectorHandlers) ConnectExtOauth(w http.ResponseWriter, r *http.Request, provider string) {
	ctx := r.Context()
	if err := auth.RequireHuman(ctx); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := auth.Require(ctx, objectIntegrations, principal.ActionCreate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	unit, app, declared := composedOAuthApp(provider)
	if !declared {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	actor, ws, ok := signedInHuman(w, r)
	if !ok {
		return
	}
	if !h.extOAuthMounted() {
		httperr.NotImplemented(w, r, "ConnectExtOauth")
		return
	}
	client, err := h.grantsFor(unit.Name).client(ctx, app)
	if errors.Is(err, errExtOAuthAppMissing) {
		httperr.NotImplementedBecause(w, r, errExtOAuthAppMissing.Error())
		return
	}
	if err != nil {
		writeConnectorAppFailure(w, r, err)
		return
	}
	flow := extOAuthStateProvider(provider)
	nonce := setCSRFNonce(w, csrfCookieName(flow), extOAuthPathPrefix)
	state := h.signer.sign(connectState{
		Workspace: ws, User: actor.UserID, Provider: flow, Nonce: nonce, Version: stateVersionNamespacedCSRF,
	}, time.Now().Add(connectStateTTL))
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ExtOAuthConnectResponse{
		AuthorizeUrl: client.AuthCodeURL(state, h.extCallbackURL(provider)),
	})
}

// ExtOauthCallback implements (GET /ext-oauth/{provider}/callback). Every
// answer past the two route checks is a redirect to the unit's screen, and none
// carries vendor text.
func (h connectorHandlers) ExtOauthCallback(
	w http.ResponseWriter, r *http.Request, provider string, params crmcontracts.ExtOauthCallbackParams,
) {
	unit, app, ok := composedOAuthApp(provider)
	if !ok {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	if !h.extOAuthMounted() {
		httperr.NotImplemented(w, r, "ExtOauthCallback")
		return
	}
	outcome := h.completeExtOAuth(w, r, unit, app, params)
	http.Redirect(w, r, extOAuthLandingURL(h.publicBaseURL, unit.Name, provider, outcome), http.StatusFound)
}

// completeExtOAuth runs the callback and answers the landing outcome.
func (h connectorHandlers) completeExtOAuth(
	w http.ResponseWriter, r *http.Request, unit extension.Extension, app extension.OAuthApp,
	params crmcontracts.ExtOauthCallbackParams,
) string {
	ctx := r.Context()
	flow := extOAuthStateProvider(app.Provider)
	st, err := h.signer.verify(params.State, time.Now())
	if err != nil || st.Provider != flow {
		slog.WarnContext(ctx, "extension oauth callback: the state does not verify", "err", err, "provider", app.Provider)
		return outcomeError
	}
	if params.Error != nil && *params.Error != "" {
		return outcomeDenied
	}
	if params.Code == nil || *params.Code == "" || !consumeNonceCookie(w, r, csrfCookieName(flow), extOAuthPathPrefix, st) {
		return outcomeError
	}
	runCtx, err := h.liveAdminContext(grantorContext(ctx, st), st)
	if err != nil {
		slog.WarnContext(ctx, "extension oauth callback: the admin no longer holds the grant to connect",
			"err", err, "provider", app.Provider)
		return outcomeError
	}
	grants := h.grantsFor(unit.Name)
	if err := grants.exchange(runCtx, app, *params.Code, h.extCallbackURL(app.Provider)); err != nil {
		slog.ErrorContext(ctx, "extension oauth callback: completing consent failed", "err", err, "provider", app.Provider)
		return extExchangeOutcome(err)
	}
	if err := runOnGrant(runCtx, unit, app, boundExtensionRuntime(), extraParams(r)); err != nil {
		slog.ErrorContext(ctx, "extension oauth callback: the unit refused the grant", "err", err, "provider", app.Provider)
		if forgetErr := grants.forget(runCtx, app.Provider); forgetErr != nil {
			slog.ErrorContext(ctx, "extension oauth callback: forgetting the refused grant", "err", forgetErr, "provider", app.Provider)
		}
		return outcomeError
	}
	return outcomeOK
}

// liveAdminContext re-reads the human the state names and refuses one who no
// longer holds the grant to connect an integration. Minutes of vendor UI pass
// between the connect and here, and the grant is the whole workspace's.
func (h connectorHandlers) liveAdminContext(ctx context.Context, st connectState) (context.Context, error) {
	if err := h.requireLiveGrantor(ctx, st); err != nil {
		return nil, err
	}
	rbac, err := h.authority.EffectiveRBAC(ctx, st.Workspace, st.User)
	if err != nil {
		return nil, err
	}
	actor, _ := principal.Actor(ctx)
	actor.Permissions = rbac.Permissions
	ctx = principal.WithActor(ctx, actor)
	if err := auth.Require(ctx, objectIntegrations, principal.ActionCreate); err != nil {
		return nil, err
	}
	return ctx, nil
}

// extExchangeOutcome is connectFailureOutcome without the Google-only branch:
// a refused client is fixed on the app card, a refused code by connecting
// again.
func extExchangeOutcome(err error) string {
	switch {
	case oauthflow.Misconfigured(err), errors.Is(err, errExtOAuthAppMissing):
		return outcomeBadClient
	case errors.Is(err, connector.ErrAuthRejected):
		return outcomeRejected
	default:
		return outcomeError
	}
}

// The query parameters RFC 6749 §4.1.2 gives the authorization-code callback.
const (
	callbackParamCode  = "code"
	callbackParamState = "state"
	callbackParamError = "error"
)

var callbackOwnParams = []string{callbackParamCode, callbackParamState, callbackParamError}

// extraParams are the callback's query parameters other than the three the
// flow itself reads, first value each.
func extraParams(r *http.Request) map[string]string {
	out := map[string]string{}
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			out[key] = values[0]
		}
	}
	for _, own := range callbackOwnParams {
		delete(out, own)
	}
	return out
}

// runOnGrant hands the unit the grant's extras on a Runtime answering as the
// admin who consented. A unit with no OnGrant has nothing to keep.
func runOnGrant(
	ctx context.Context, unit extension.Extension, app extension.OAuthApp, deps extensionRuntimeBinding,
	params map[string]string,
) error {
	if app.OnGrant == nil {
		return nil
	}
	rt := runtimeFor(ctx, string(unit.Name), string(unit.Version), extOAuthPathPrefix+"/"+app.Provider+"/callback", deps)
	defer rt.release()
	return app.OnGrant(ctx, rt, extension.OAuthGrant{Params: params})
}
