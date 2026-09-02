// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The per-provider OAuth capture surface (RC-8; capture.md CAP-WIRE-1):
// listConnectors / connectConnector / connectorOAuthCallback /
// disconnectConnector, for the standing (persisted) mail connectors —
// distinct from the direct-credential /connectors/imap/connect (connectors_imap.go),
// which is itself a standing connection, just OAuth-less. connect returns the
// provider consent URL carrying a signed state; the session-less callback
// verifies that state, exchanges the code, reconstructs the granting human's
// authority from the (trusted) state, and persists the connection through the
// capture Registry; the background poller then syncs it. Gmail, Microsoft
// Graph, and Google Calendar (gcal) share this flow, dispatched by provider;
// gcal reuses the same Google OAuth app as Gmail, differing only in scope.
//
// connectorHandlers is embedded in Server as a zero value; a role that wires
// neither OAuth app (no --gmail-client-id / --graph-client-id) leaves
// oauth/registry nil, and every operation answers the repo's standard 501
// rather than nil-derefing — capture stays declared-but-absent by omission.

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// connectStateTTL bounds the consent round-trip: generous for a human to
// click through Google, short enough that a leaked state is quickly useless.
const connectStateTTL = 10 * time.Minute

// codeUnauthorized is the RFC 7807 code for connector/backfill ops that
// require a signed-in human principal — the contract's documented 401
// machine code (crm.yaml's normative Unauthorized example), matching the
// platform 401 writer.
const codeUnauthorized = "unauthorized"

type connectorHandlers struct {
	registry *capture.Registry
	// publicOrigin reports the address this installation puts in outgoing
	// links. Nil when none is configured, and then the field is absent
	// rather than reported as broken.
	publicOrigin func(ctx context.Context) PublicOriginStatus
	// authority is identity's live resolver. The callback re-reads the
	// granting human through it before it spends the authorization code: the
	// signed state proves who STARTED the consent, and minutes of provider
	// UI can pass before it comes back.
	authority authz.Resolver
	// imapAuthenticate probes+seals IMAP credentials; nil means the
	// production standing connector. Injectable so the transport's own
	// branches are testable without a live mail server.
	imapAuthenticate func(ctx context.Context, req connector.AuthRequest) (connector.Auth, error)
	oauth            gmail.OAuth
	gmailAPI         gmail.API
	gcalOAuth        gcal.OAuth
	gcalAPI          gcal.API
	graphOAuth       graph.OAuth
	graphAPI         graph.API
	graphCalOAuth    graphcal.OAuth
	graphCalAPI      graphcal.API
	signer           stateSigner
	// publicBaseURL is the canonical public/front origin (the SPA): where the
	// browser lands after consent, and — for a same-origin deployment — the
	// default base for the callback redirect_uri too.
	publicBaseURL string
	// apiBaseURL is the api's externally-reachable base, used ONLY for the
	// callback redirect_uri (which must resolve to where the api serves it).
	// Empty for a same-origin deployment (the callback then rides
	// publicBaseURL/v1); a split dev stack (SPA :5173, api :8080) sets it.
	apiBaseURL string
	// googleCredentials and microsoftCredentials resolve the installation's
	// STORED app for each vendor, on each call rather than once at boot.
	//
	// It exists so an app set through Settings works without restarting the api.
	// The boot-composed `oauth` below is the environment's copy, and it stays as
	// the fallback: an installation that has always exported the pair keeps
	// working with no action, exactly as provider keys did when they moved into
	// the vault.
	//
	// A func rather than the store, so this file needs neither the pool nor the
	// system-principal context the read runs on — compose binds both where it
	// wires this, and a test can hand over a fake without a database.
	googleCredentials    appResolver
	microsoftCredentials appResolver
}

// wired reports whether the Gmail OAuth app is composed for this role.
func (h connectorHandlers) wired() bool { return h.registry != nil && h.oauth != nil }

func (h connectorHandlers) callbackURL(provider string) string {
	return connectorCallbackURL(h.apiBaseURL, h.publicBaseURL, provider)
}

// connectorCallbackURL builds a connector's redirect_uri from the two bases
// rather than from a built handler, so the value shown to an OPERATOR and the
// value sent to the provider are produced by the same code — the settings screen
// has to name this URL before any handler exists to ask.
//
// The api's own origin wins and the SPA's is the fallback, because the callback
// must resolve to where the API serves it: on a split deployment those are
// different hosts, which is why this cannot be derived from the sign-in URI.
func connectorCallbackURL(apiBaseURL, publicBaseURL, provider string) string {
	base := apiBaseURL
	if base == "" {
		base = publicBaseURL
	}
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/v1/connectors/" + provider + "/callback"
}

// oauthApp is one composed OAuth provider seen through the shared
// connect/callback flow: the consent-URL builder and the code-for-credential
// exchange, so the flow itself stays provider-agnostic.
type oauthApp struct {
	authCodeURL  func(state, redirectURI string) string
	authenticate func(ctx context.Context, code, redirectURI string) (connector.Auth, error)
}

func (h connectorHandlers) ListConnectors(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		httperr.NotImplemented(w, r, "ListConnectors")
		return
	}
	views, err := h.registry.Connections(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	resp := crmcontracts.CaptureConnectionListResponse{
		Data: make([]crmcontracts.CaptureConnection, 0, len(views)),
	}
	for _, v := range views {
		resp.Data = append(resp.Data, toContractConnection(v))
	}
	// Absent when no origin is configured, which is itself the answer a
	// screen needs: there is nothing to report rather than something broken.
	if h.publicOrigin != nil {
		status := h.publicOrigin(r.Context())
		resp.PublicOrigin = &crmcontracts.PublicOriginStatus{
			Origin:    status.Origin,
			Reachable: status.Reachable,
			CheckedAt: status.CheckedAt,
			Detail:    &status.Detail,
		}
	}
	httperr.WriteJSON(w, http.StatusOK, resp)
}

func (h connectorHandlers) ConnectConnector(w http.ResponseWriter, r *http.Request, provider crmcontracts.CaptureProvider) {
	// The standing IMAP connect needs only the registry (credentials are
	// per-connection, vault-sealed) — never the Gmail OAuth app.
	if string(provider) == providerIMAP {
		if h.registry == nil {
			httperr.NotImplemented(w, r, "ConnectConnector")
			return
		}
		h.connectIMAP(w, r)
		return
	}
	if !isOAuthProvider(string(provider)) {
		if h.registry == nil {
			httperr.NotImplemented(w, r, "ConnectConnector")
			return
		}
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity,
			Code:   "connector_unsupported",
			Detail: "Only the " + strings.Join(oauthProviders, ", ") + " and imap connectors can be connected here.",
		})
		return
	}
	app, ok, err := h.oauthApp(r.Context(), string(provider))
	if err != nil {
		// A stored app that will not resolve is a different answer from one that
		// was never configured: the installation HAS an app and cannot open its
		// secret, and reporting 501 would send an operator to create a second.
		//
		// Classified rather than handed to httperr.Write raw — these are plain
		// errors it does not recognise, so the caller would get a bare 500 with
		// less to act on than the 501 this branch exists to avoid.
		writeConnectorAppFailure(w, r, err)
		return
	}
	if h.registry == nil || !ok {
		httperr.NotImplementedBecause(w, r, noConnectorAppDetail(string(provider)))
		return
	}
	actor, ok := principal.Actor(r.Context())
	ws, hasWS := principal.WorkspaceID(r.Context())
	if !ok || actor.Type != principal.PrincipalHuman || !hasWS {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnauthorized,
			Code:   codeUnauthorized,
			// Provider-neutral: this guard serves the calendars as well as the
			// mailboxes, and naming a mailbox mislabels half the flows it refuses.
			Detail: "Connecting an account is a signed-in human action.",
		})
		return
	}
	// The body is optional for OAuth providers (they submit nothing), so an
	// absent one is not a failure — only a malformed one is. ContentLength ==
	// 0 is the reliable "definitely no body" signal; -1 (chunked, length
	// unknown until read) must still be decoded, or a chunked request's
	// return_to is silently dropped and a malformed chunked body skips
	// rejection entirely.
	returnTo := returnToOnboarding
	if r.ContentLength != 0 {
		var req crmcontracts.ConnectConnectorRequest
		if !httperr.Decode(w, r, &req) {
			return
		}
		if req.ReturnTo != nil && string(*req.ReturnTo) == returnToSettings {
			returnTo = returnToSettings
		}
	}
	// CSRF: a random nonce goes into both a SameSite=Lax cookie and the signed
	// state; the callback requires them to match, so a victim can't complete an
	// attacker-initiated flow (account-linking CSRF).
	nonce := rand.Text()
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName(string(provider)),
		Value:    nonce,
		Path:     "/v1/connectors",
		MaxAge:   int(connectStateTTL / time.Second),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	state := h.signer.sign(
		connectState{
			Workspace: ws, User: actor.UserID, Provider: string(provider),
			Nonce: nonce, ReturnTo: returnTo, Version: stateVersionNamespacedCSRF,
		},
		time.Now().Add(connectStateTTL),
	)
	authURL := app.authCodeURL(state, h.callbackURL(string(provider)))
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ConnectConnectorResponse{AuthorizeUrl: &authURL})
}

func (h connectorHandlers) ConnectorOAuthCallback(w http.ResponseWriter, r *http.Request, provider crmcontracts.CaptureProvider, params crmcontracts.ConnectorOAuthCallbackParams) {
	// Availability first, and it costs nothing: no database read, no unseal. It
	// reveals exactly what the declared 501 always revealed — whether this
	// deployment serves the provider at all — while the CREDENTIAL is resolved
	// further down, after the signed state has authenticated the request.
	if h.registry == nil || !h.providerWired(string(provider)) {
		httperr.NotImplemented(w, r, "ConnectorOAuthCallback")
		return
	}
	ctx := r.Context()
	// The signed state is the only trustworthy carrier here (no session cookie
	// on the cross-site redirect), and it is what names the surface the human
	// started from. Verify it BEFORE branching on the outcome: a denial that
	// began in Settings has to land back in Settings, or the person never sees
	// the note explaining what happened. An unverifiable state yields no
	// trustworthy ReturnTo, so those paths keep the default.
	st, err := h.signer.verify(params.State, time.Now())
	stateTrusted := err == nil && st.Provider == string(provider)
	returnTo := returnToOnboarding
	if stateTrusted {
		returnTo = st.ReturnTo
	}
	// The user denied consent at the provider — surface it honestly, never as
	// an error.
	if params.Error != nil && *params.Error != "" {
		http.Redirect(w, r, h.landingURL(outcomeDenied, returnTo, string(provider)), http.StatusFound)
		return
	}
	// A bad/expired/mismatched state or a missing code cannot proceed —
	// redirect with an honest error, details logged only.
	if !stateTrusted || params.Code == nil || *params.Code == "" {
		slog.WarnContext(ctx, "connector callback rejected", "err", err, "provider", string(provider))
		http.Redirect(w, r, h.landingURL(outcomeError, returnTo, string(provider)), http.StatusFound)
		return
	}
	// CSRF: the nonce cookie must match the nonce in the signed state, proving
	// the browser completing the flow is the one that started it. Without this,
	// an attacker could trick a victim into completing the attacker's flow and
	// link the victim's mailbox to the attacker's account (account-linking
	// CSRF). The signed state is already verified by this point — what this
	// establishes is the BROWSER's identity — so the redirect can honor the
	// surface the flow started from.
	if !consumeCSRFNonce(w, r, string(provider), st) {
		http.Redirect(w, r, h.landingURL(outcomeError, returnTo, string(provider)), http.StatusFound)
		return
	}

	runCtx := grantorContext(ctx, st)
	// The grant needs LIVE authority, resolved before the code is spent: an
	// exchanged authorization code is a real provider credential, and one
	// minted for a human who may no longer hold it has to be revoked at the
	// provider rather than simply discarded. Registry.Connect enforces the same
	// invariant; this is the cheaper, earlier half of it.
	if err := h.requireLiveGrantor(runCtx, st); err != nil {
		slog.WarnContext(ctx, "connector callback: the granting human no longer holds live authority",
			"err", err, "provider", string(provider))
		http.Redirect(w, r, h.landingURL(outcomeError, returnTo, string(provider)), http.StatusFound)
		return
	}

	// The credential is resolved HERE, after the signed state and the granting
	// human have both been checked — not at the top of the handler.
	//
	// This route is deliberately session-less: its authentication IS the signed
	// state. Resolving above would mean an anonymous request unseals a live
	// client secret before anything has authenticated it, on a path with no rate
	// limit. It also answers a browser redirect with a JSON error, where every
	// other failure here lands the person back on a page that explains itself.
	// runCtx, not ctx: a stored app is per-workspace and this route is
	// session-less, so the raw request context has no workspace to read it
	// under. Under ctx the lookup finds nothing and falls back to the
	// deployment's app, spending the code against the wrong client.
	app, ok, err := h.oauthApp(runCtx, string(provider))
	if err != nil || !ok {
		if err != nil {
			logConnectFailure(ctx, string(provider), err)
		}
		http.Redirect(w, r, h.landingURL(outcomeError, returnTo, string(provider)), http.StatusFound)
		return
	}

	auth, err := app.authenticate(runCtx, *params.Code, h.callbackURL(string(provider)))
	if err != nil {
		logConnectFailure(ctx, string(provider), err)
		http.Redirect(w, r, h.landingURL(connectFailureOutcome(string(provider), err), returnTo, string(provider)), http.StatusFound)
		return
	}

	if _, err := h.registry.Connect(runCtx, string(provider), auth); err != nil {
		slog.ErrorContext(ctx, "connector callback: persisting connection", "err", err, "provider", string(provider))
		http.Redirect(w, r, h.landingURL(outcomeError, returnTo, string(provider)), http.StatusFound)
		return
	}
	http.Redirect(w, r, h.landingURL(outcomeOK, returnTo, string(provider)), http.StatusFound)
}

// grantorContext reconstructs the granting human's authority from the trusted
// (signed) state: workspace + user id. A minimal read-scoped human principal is
// what Registry.Connect needs — it stamps granted_by and checks the connector's
// read scope; the live authority behind it is resolved separately.
func grantorContext(ctx context.Context, st connectState) context.Context {
	runCtx := principal.WithWorkspaceID(ctx, st.Workspace)
	return principal.WithActor(runCtx, principal.Principal{
		Type:   principal.PrincipalHuman,
		ID:     "human:" + st.User.String(),
		UserID: st.User,
		Scopes: principal.NewScopeSet(principal.ScopeRead),
	})
}

// requireLiveGrantor resolves the human named by the signed state against
// identity's live authority. A missing resolver is a wiring fault and fails
// closed: an unchecked grant is the hole this exists to close, and silently
// skipping the check would look exactly like passing it.
func (h connectorHandlers) requireLiveGrantor(ctx context.Context, st connectState) error {
	if h.authority == nil {
		return errors.New("connector callback: no authority resolver wired — a grant cannot be checked against live authority")
	}
	if _, err := h.authority.EffectiveRBAC(ctx, st.Workspace, st.User); err != nil {
		return err
	}
	_, err := h.authority.SeatType(ctx, st.Workspace, st.User)
	return err
}

func (h connectorHandlers) DisconnectConnector(w http.ResponseWriter, r *http.Request, provider crmcontracts.CaptureProvider) {
	if h.registry == nil {
		httperr.NotImplemented(w, r, "DisconnectConnector")
		return
	}
	if err := h.registry.Disconnect(r.Context(), string(provider)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetConnectorSignatureEnrichment: PUT /connectors/{provider}/signature-enrichment.
// The mailbox owner's own switch over the nightly signature pass; the registry
// owns the scoping and the audit, and this is wire-only.
func (h connectorHandlers) SetConnectorSignatureEnrichment(w http.ResponseWriter, r *http.Request, provider crmcontracts.CaptureProvider) {
	if h.registry == nil {
		httperr.NotImplemented(w, r, "SetConnectorSignatureEnrichment")
		return
	}
	var req crmcontracts.SetSignatureEnrichmentRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	view, err := h.registry.SetSignatureEnrichment(r.Context(), string(provider), req.Enabled)
	// A mailbox this caller has not connected is not theirs to configure, and
	// answers as absent — the registry's sentinel is a plain error and would
	// otherwise reach the client as a 500 about a state the product reaches
	// whenever somebody opens a settings page for a provider they never linked.
	if errors.Is(err, capture.ErrNoConnection) {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractConnection(view))
}

// SetConnectorMailPosture: PUT /connectors/{provider}/mail-posture.
// What this mailbox asks of the mail it brings in; the registry owns the
// scoping, the workspace opt-in and the audit, and this is wire-only.
func (h connectorHandlers) SetConnectorMailPosture(w http.ResponseWriter, r *http.Request, provider crmcontracts.CaptureProvider) {
	if h.registry == nil {
		httperr.NotImplemented(w, r, "SetConnectorMailPosture")
		return
	}
	var req crmcontracts.SetMailPostureRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	applyToHistory := req.ApplyToHistory != nil && *req.ApplyToHistory
	view, err := h.registry.SetMailPosture(r.Context(), string(provider), string(req.Posture), applyToHistory)
	// A mailbox this caller has not connected is not theirs to configure, and
	// answers as absent, for the reason the sibling above says.
	if errors.Is(err, capture.ErrNoConnection) {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractConnection(view))
}

// postureOnWire renders a connection's posture as the contract's optional
// field. Empty means the row predates the column on a read path that did not
// select it, which is absence rather than a posture — the alternative, coercing
// it to a word, would tell a client this mailbox asked for something it never
// did.
func postureOnWire(posture string) *crmcontracts.CaptureConnectionMailPosture {
	if posture == "" {
		return nil
	}
	p := crmcontracts.CaptureConnectionMailPosture(posture)
	return &p
}

// toContractConnection maps a registry connection row onto the wire shape.
// Storage now uses the contract's own status vocabulary (CAP-DDL-2 reconciled
// capture_connection to it), so status is a straight cast — no translation. The
// credential is never present.
func toContractConnection(v capture.ConnectionView) crmcontracts.CaptureConnection {
	c := crmcontracts.CaptureConnection{
		Id:             openapi_types.UUID(v.ID),
		Provider:       crmcontracts.CaptureConnectionProvider(v.Provider),
		Status:         crmcontracts.CaptureConnectionStatus(v.Status),
		Scopes:         v.ProviderScopes,
		WatchExpiresAt: v.WatchExpiresAt,
		AccountLabel:   v.AccountLabel,
		// Carried as the pointer it is: null on the wire is this mailbox
		// following the tenant default, not a field the read forgot.
		SignatureEnrichEnabled: v.SignatureEnrichEnabled,
		MailPosture:            postureOnWire(v.MailPosture),
	}
	if c.Scopes == nil {
		c.Scopes = []string{}
	}
	if len(v.Cursor) > 0 {
		s := string(v.Cursor)
		c.SyncCursor = &s
	}
	c.LastSyncedAt = v.LastSyncedAt
	c.LastSyncErrorClass = v.LastErrorClass
	c.NextSyncDueAt = v.NextSyncDueAt
	bf := backfillStatusPayload(v.Backfill)
	c.Backfill = &bf
	return c
}
