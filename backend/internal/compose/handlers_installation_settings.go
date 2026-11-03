// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The installation-settings surface (ADR-0090): read the company's
// name, reporting zone and base currency (every role), change them (admin/ops,
// human-only). Thin transport — the identity store owns the RBAC gate, the
// per-setting validation, the base-currency freeze and the audit-only write.

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/currencylock"
)

type installationSettingsHandlers struct {
	store *identity.InstallationSettingsStore
	// afterSave runs once a patch has committed. The meter's ForgetPolicy, so
	// this process binds a saved allowance on the next agent call.
	afterSave func()
	// now reads the window plan in the answer; it is the store's clock in a test
	// that pins one, and the wall clock otherwise.
	now func() time.Time
	// pool reads the jurisdiction page in one transaction
	// (handlers_installation_jurisdiction.go).
	pool *pgxpool.Pool
	// configuredProviders carries the external sign-in providers this DEPLOYMENT
	// mounted, injected by the option that wires them. Empty is the honest
	// answer for a deployment that composed none, and the screen then offers
	// nothing to toggle rather than a list nobody can use. A mounted provider
	// whose client is not there yet — no app stored and none in the
	// environment — is still listed: the app card on the same screen is where
	// it is made to work, and the login screen withholds it until then.
	configuredProviders []identity.OIDCProviderConfig
	// signInScreen is what the login screen draws right now, the identity
	// module's own answer, set by the option that mounts the providers. Nil on
	// a deployment that composed none.
	signInScreen func(context.Context) []identity.OIDCProviderConfig
	// maxUploadBytes is the deployment's attachment ceiling (OPS-CFG-12),
	// published so an upload surface enforces the number THIS installation
	// applies rather than one compiled into the client.
	//
	// It rides this read rather than a read of its own because it answers the
	// same question the rest of the response does — what is true of this
	// installation — and because every role may read it: whoever can upload a
	// file can be told how large a file is worth sending.
	maxUploadBytes int64
}

func (h installationSettingsHandlers) GetInstallationSettings(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "GetInstallationSettings")
		return
	}
	s, err := h.store.GetInstallation(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, h.toContract(s))
}

// GetAuthenticationPolicy answers the sign-in half on its own grant.
//
// Apart from `sign_in_screen`, the values are identical to the aggregate's
// `sign_in_providers`; what differs is who may read them. Every role reads the
// installation's name, timezone and currency, but not who may sign in.
// So this answers to `authentication_policy`, which the store checks first.
func (h installationSettingsHandlers) GetAuthenticationPolicy(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "GetAuthenticationPolicy")
		return
	}
	// Human-only (x-agent-access): an agent never reads how humans sign in.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	policy, err := h.store.SignInPolicy(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// The SAME resolver the aggregate renders and the login screen is served
	// from, so a reader cannot be told a different answer by asking a different
	// surface.
	// A never-written map reads back nil, and the contract requires an object:
	// {} is the honest spelling of "no group grants anything", where null would
	// hand every client a second absent-shape to branch on.
	groupRoleMap := policy.GroupRoleMap
	if groupRoleMap == nil {
		groupRoleMap = map[string]string{}
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.AuthenticationPolicy{
		SignInProviders:  h.signInProviders(policy.Providers),
		RequireSso:       policy.RequireSSO,
		RequireMfa:       policy.RequireMFA,
		OidcGroupRoleMap: groupRoleMap,
		SignInScreen:     h.signInScreenKeys(r.Context()),
	})
}

// signInScreenKeys rides the policy read and not the aggregate. Resolving it
// reads stored sign-in clients, which a rep reading the currency must not cause. Empty, never null, when nothing is drawn.
func (h installationSettingsHandlers) signInScreenKeys(ctx context.Context) []string {
	keys := make([]string, 0)
	if h.signInScreen == nil {
		return keys
	}
	for _, p := range h.signInScreen(ctx) {
		keys = append(keys, p.Key)
	}
	return keys
}

func (h installationSettingsHandlers) UpdateInstallationSettings(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "UpdateInstallationSettings")
		return
	}
	// Human-only (x-agent-access): an agent never renames the company or
	// re-bases its reporting currency. The store re-checks the admin/ops grant.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.UpdateInstallationSettingsRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	patch := identity.InstallationPatch{
		Name:         req.Name,
		Timezone:     req.Timezone,
		BaseCurrency: req.BaseCurrency,
		// The entry refuses a code no compiled-in pack answers; the store runs
		// the floor carry in the same transaction.
		Country:      req.Country,
		CallWhatsApp: req.CallWhatsapp, CallTelegram: req.CallTelegram, CallZalo: req.CallZalo, IndividualPerformance: req.IndividualPerformance, ForecastSelfCorrecting: req.ForecastSelfCorrecting,
	}
	if req.DateFormat != nil {
		value := string(*req.DateFormat)
		patch.DateFormat = &value
	}
	if req.TimeFormat != nil {
		value := string(*req.TimeFormat)
		patch.TimeFormat = &value
	}
	if req.BaseLanguage != nil {
		// The generated enum refuses an unknown value at the edge, so a code
		// that reaches here is one the contract admits. The entry validates it
		// again on the write — the contract and the setting each state the
		// language set, and neither defers to the other.
		if !req.BaseLanguage.Valid() {
			httperr.Write(w, r, httperr.Validation("base_language", "invalid", "a base language is one of en, de, vi"))
			return
		}
		lang := string(*req.BaseLanguage)
		patch.BaseLanguage = &lang
	}
	// No range check here, unlike BaseLanguage above. That one exists because
	// the generated enum type carries a Valid() method worth calling; this
	// field has no such type, and the entry's own refusal is already the better
	// answer — settings.InvalidValue implements apperrors.FieldFault, so an
	// out-of-range month comes back as a 422 naming the setting and carrying
	// identity's own sentence, which quotes the value that was refused. A
	// second check here would name a different field and say less.
	patch.FiscalYearStartMonth = req.FiscalYearStartMonth
	// Same again: the entry's own validator holds the 1..168 bound and names
	// this field when it refuses, so a second check here would say less.
	patch.DeadWorkBannerHours = req.DeadWorkBannerHours
	patch.OAuthAccessTokenTTLMinutes = req.OauthAccessTokenTtlMinutes
	// Same reasoning as the month above: the entry validates against the shared
	// kernel's set, so an unknown measure comes back naming this field and
	// quoting the value. Converted to a plain string because the patch carries
	// what was ASKED for, and the generated enum type would claim it had already
	// been checked against the vocabulary the projection enforces.
	if req.ForecastForwardMeasure != nil {
		measure := string(*req.ForecastForwardMeasure)
		patch.ForecastForwardMeasure = &measure
	}
	// Passed through unvalidated against the deployment's list on purpose. A key
	// this deployment holds no credentials for enables nothing — the effective
	// answer is the intersection — so refusing it would be refusing a request
	// that is already harmless, and would make an admin's saved choice depend on
	// which providers happened to be wired the day they saved it.
	patch.EnabledOidcProviders = req.EnabledOidcProviders
	// A bool the admin either set or omitted; the entry itself needs no
	// validation, and an admin can never strand the installation because the
	// break-glass admin exemption is enforced at login, not stored here.
	patch.RequireSSO = req.RequireSso
	patch.RequireMFA = req.RequireMfa
	// The entry's validator holds the bounds and the role vocabulary, so a
	// refusal names the setting and quotes the group or role it refused —
	// the same division of labour as the fiscal month above.
	patch.OidcGroupRoleMap = req.OidcGroupRoleMap
	// Each switch checks agent_client per field (settings.SetRawTx), so the
	// installation patch gate alone does not let ops turn a method back on.
	patch.DynamicClientRegistration = req.DynamicClientRegistration
	patch.MetadataDocumentClients = req.MetadataDocumentClients
	// Each entry's validator holds its range and names the field it refuses.
	patch.Operations = identity.OperationPatch{
		AgentRunnerIntervalSeconds:       req.AgentRunnerIntervalSeconds,
		CloseDateSweepIntervalSeconds:    req.CloseDateSweepIntervalSeconds,
		FollowUpReconcileIntervalSeconds: req.FollowUpReconcileIntervalSeconds,
		GeocodeBackfillIntervalSeconds:   req.GeocodeBackfillIntervalSeconds,
		GmailWatchRenewWithinHours:       req.GmailWatchRenewWithinHours,
		GmailWatchScanIntervalSeconds:    req.GmailWatchScanIntervalSeconds,
		GraphWatchRenewWithinHours:       req.GraphWatchRenewWithinHours,
		GraphWatchScanIntervalSeconds:    req.GraphWatchScanIntervalSeconds,
		RetentionSweepIntervalSeconds:    req.RetentionSweepIntervalSeconds,
		SendMaxAgeHours:                  req.SendMaxAgeHours,
		SendRateLimit:                    req.SendRateLimit,
		SendRateWindowSeconds:            req.SendRateWindowSeconds,
		TechnicalBackfillIntervalSeconds: req.TechnicalBackfillIntervalSeconds,
		TimeScanIntervalSeconds:          req.TimeScanIntervalSeconds,
		WebhookRetryIntervalSeconds:      req.WebhookRetryIntervalSeconds,
	}
	patch.AgentAllowances = identity.AgentAllowancePatch{
		WritesPerWindow: req.AgentWritesPerWindow,
		EgressPerWindow: req.AgentEgressPerWindow,
		CallsPerWindow:  req.AgentCallsPerWindow,
		WindowHours:     req.AgentWindowHours,
	}
	s, err := h.store.UpdateInstallation(r.Context(), patch)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if h.afterSave != nil {
		h.afterSave()
	}
	httperr.WriteJSON(w, http.StatusOK, h.toContract(s))
}

// toContract maps the stored values onto the wire shape, and adds the one field
// that is not stored at all: the upload ceiling is a deployment fact, so it
// arrives from composition rather than from the settings table — the same way
// base_currency_locked is derived rather than kept.
//
// The lock reason and its facts are sent only when the lock has prose. An empty
// reason, or one off the contract's enum, tells a client nothing.
func (h installationSettingsHandlers) toContract(s identity.InstallationSettings) crmcontracts.InstallationSettings {
	out := crmcontracts.InstallationSettings{
		Name:                 s.Name,
		Timezone:             s.Timezone,
		BaseCurrency:         s.BaseCurrency,
		BaseLanguage:         crmcontracts.InstallationSettingsBaseLanguage(s.BaseLanguage),
		FiscalYearStartMonth: s.FiscalYearStartMonth,
		DeadWorkBannerHours:  s.DeadWorkBannerHours,
		ForecastForwardMeasure: crmcontracts.InstallationSettingsForecastForwardMeasure(
			s.ForecastForwardMeasure),
		OauthAccessTokenTtlMinutes: s.OAuthAccessTokenTTLMinutes,
		BaseCurrencyLocked:         s.BaseCurrencyLocked,
		CallWhatsapp:               s.CallWhatsApp,
		CallTelegram:               s.CallTelegram,
		CallZalo:                   s.CallZalo,
		IndividualPerformance:      s.IndividualPerformance,
		ForecastSelfCorrecting:     s.ForecastSelfCorrecting,
		MaxUploadBytes:             h.maxUploadBytes,
		SignInProviders:            h.signInProviders(s.EnabledOidcProviders),
		Operations:                 crmcontracts.OperationSettings(s.Operations),
		AgentAllowances:            toAgentAllowances(s.AgentAllowances, h.clock()),
		Locks:                      loadedLocks(identity.Settings()),
	}
	dateFormat := crmcontracts.InstallationSettingsDateFormat(s.DateFormat)
	timeFormat := crmcontracts.InstallationSettingsTimeFormat(s.TimeFormat)
	out.DateFormat = &dateFormat
	out.TimeFormat = &timeFormat
	if reason := s.BaseCurrencyLock.Prose(); s.BaseCurrencyLocked && reason != "" {
		out.BaseCurrencyLockedReason = &reason
		out.BaseCurrencyLock = toBaseCurrencyLock(s.BaseCurrencyLock)
	}
	return out
}

// toBaseCurrencyLock sends the facts the prose reason was worded from. An
// absent base stays absent, not empty, so a client words that case without one.
func toBaseCurrencyLock(lock currencylock.Lock) *crmcontracts.BaseCurrencyLock {
	out := &crmcontracts.BaseCurrencyLock{
		Reason: crmcontracts.BaseCurrencyLockReason(lock.Reason),
		Count:  lock.Count,
	}
	if lock.BaseCurrency != "" {
		base := lock.BaseCurrency
		out.BaseCurrency = &base
	}
	return out
}

// signInProviders reports every provider this DEPLOYMENT can offer, each marked
// with whether the installation currently offers it.
//
// The deployment's list is the spine and the setting only marks it, because the
// two answer different questions: credentials decide what is possible, and the
// admin decides what is offered. Rendering the setting alone would show an
// operator a provider they cannot actually use, and rendering it as a free-text
// list would invite them to type one in.
//
// A nil chosen list means the admin has never chosen, which is every configured
// provider. That is the reading compose applies when it resolves the effective
// set, and this screen has to apply it too or it would show an operator a
// provider list their login page does not serve.
func (h installationSettingsHandlers) signInProviders(chosen []string) []crmcontracts.SignInProvider {
	// The SAME resolver the login screen is served from, so this screen cannot
	// answer a different question. Both encode one rule — configured ∩ chosen,
	// with a nil choice meaning all — and spelling it twice is how a settings
	// page comes to show a provider as offered that the login page does not
	// offer, with nothing failing in between.
	offered := make(map[string]bool)
	for _, p := range offeredProviders(h.configuredProviders, chosen) {
		offered[p.Key] = true
	}
	out := make([]crmcontracts.SignInProvider, 0, len(h.configuredProviders))
	for _, p := range h.configuredProviders {
		out = append(out, crmcontracts.SignInProvider{
			Enabled: offered[p.Key],
			Key:     p.Key,
			Label:   p.Label,
		})
	}
	return out
}

func (h installationSettingsHandlers) clock() time.Time {
	if h.now != nil {
		return h.now()
	}
	return time.Now()
}
