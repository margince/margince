// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What this installation still has to be configured with before it can be used.
//
// One surface answering one question, because the onboarding gate has to make a
// single decision — may this reader proceed — and composing that out of
// /ai/profile plus a Google-app read would put the installation's own policy in
// the client. The moment a posture needs a different rule (a deployment serving
// no mail, a local-model install with no vendor to key) the rule changes here
// and every screen follows.

import (
	"context"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// installationSetupReadActor names the machinery read in the audit trail: the
// steps are read on the installation's behalf to answer one question for a
// reader who may not read the underlying settings themselves.
const installationSetupReadActor = "installation-setup-read"

type installationSetupHandlers struct {
	// routing reads the stored tier→model binding. Nil on a role that composed
	// no AI surface, which reads as "not configured" rather than as an error:
	// the reader still needs to be told what to do about it.
	routing *ai.RoutingStore
	// providerKeys reports which cloud vendors hold a credential.
	providerKeys *ai.ProviderKeyStore
	// googleApp reports whether the installation's OAuth app is held.
	googleApp *capture.GoogleAppStore
	// envGoogleApp records that this PROCESS was composed with a Google app from
	// its environment.
	//
	// It is part of the answer because it is part of what the connect transport
	// resolves: the stored app wins, and the environment is the fallback. An
	// installation that exports the pair has a working Gmail integration, so
	// reporting its step unconfigured would tell an operator to go and store an
	// app the process already resolves.
	//
	// The two are not two sources of truth for STORAGE — the setting is the only
	// place a write lands. They are one honest reading of a two-source
	// resolution, and this field is how the reading stays identical to the
	// transport's.
	envGoogleApp bool
}

func (h installationSetupHandlers) GetInstallationSetup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Gated HERE, once, rather than left to whichever store answers first.
	//
	// Without this the authorization depends on the deployment: with a vault the
	// first store call gates on ai_routing:read, which only admin and ops hold —
	// so every rep is refused at the gate this endpoint exists to serve — and
	// without a vault the nil-store arms return before any gate and answer every
	// seat. One operation must not have two authorization rules.
	//
	// installation_settings is the object every seeded role may read, which is
	// what the onboarding gate needs: the reader being told to finish setup is
	// not always the person entitled to change the model binding.
	if err := auth.Require(ctx, identity.SettingsObject, principal.ActionRead); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// The individual reads run as machinery: the reader is entitled to KNOW what
	// is outstanding, which is not the same as being entitled to read the model
	// binding or the app. Same reasoning the connect transport resolves the
	// credential under.
	if ws, ok := principal.WorkspaceID(ctx); ok {
		ctx = bootCtx(ctx, ws, installationSetupReadActor)
	}
	aiReady, err := h.aiConfigured(ctx)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	googleReady, err := h.googleConfigured(ctx)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}

	// ORDER IS THE CONTRACT. `steps` is declared as "every setup step, in the
	// order a reader should complete them", and a client walks it in the order
	// it arrives. A client sorting these itself would be re-deciding a sequence
	// the server owns, so the sequence is pinned by
	// TestTheSetupReportListsTheStepsInTheOrderOnboardingWalksThem.
	//
	// ONLY THE MODEL BINDING BLOCKS, pinned by
	// TestOnlyTheModelBindingBlocksFirstRun. Without one the product cannot
	// perform the cold-start read that first run is, so there is nothing to let
	// a reader through to. A Google app buys mailbox capture and nothing else:
	// an installation signing in with passwords and no external provider is
	// fully usable without one.
	//
	// The app is configured from settings, where the card carries the redirect
	// URIs Google's console asks for. The step stays in the report because that
	// is what `blocking` is for: reporting it and calling it optional is the
	// answer. Dropping it would leave an installation that has no app and one
	// that has a working Gmail integration reporting the same thing to a reader
	// asking what is left to do.
	steps := []crmcontracts.InstallationSetupStep{
		{Step: crmcontracts.InstallationSetupStepStepAiModels, Configured: aiReady, Blocking: true},
		{Step: crmcontracts.InstallationSetupStepStepGoogleApp, Configured: googleReady, Blocking: false},
	}
	complete := true
	for _, s := range steps {
		if s.Blocking && !s.Configured {
			complete = false
		}
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.InstallationSetup{
		Complete: complete,
		Steps:    steps,
	})
}

// aiConfigured reports whether the installation can actually call a model.
//
// BOTH halves, because either alone is a screen that lies. A binding with no
// credential for the vendor it names fails at the first call with "needs an api
// key"; a credential with nothing bound to its vendor is a key that will never
// be used. Onboarding asking only about the binding is what would let a reader
// through to a company read that cannot run.
func (h installationSetupHandlers) aiConfigured(ctx context.Context) (bool, error) {
	if h.routing == nil || h.providerKeys == nil {
		return false, nil
	}
	bound, err := h.routing.Get(ctx)
	if err != nil {
		return false, err
	}
	if bound.Unconfigured() {
		return false, nil
	}
	keyed, err := h.providerKeys.List(ctx)
	if err != nil {
		return false, err
	}
	// Every CLOUD vendor the binding names needs a credential. A local provider
	// (ollama, vllm, the offline fake) needs none, so a fully local binding is
	// configured with no keys at all — which is the posture a sovereign
	// installation runs, and refusing it would block the deployment that most
	// deliberately chose it.
	held := make(map[string]bool, len(keyed))
	for _, k := range keyed {
		held[k.Provider] = k.Configured
	}
	for _, provider := range bound.CloudProvidersBound() {
		if !held[provider] {
			return false, nil
		}
	}
	return true, nil
}

// googleConfigured reports whether this installation can connect Gmail.
//
// The environment's copy counts, in the same precedence the connect transport
// applies — see envGoogleApp. A nil store is a role that composed no vault, and
// then only the environment can answer: it reads as unconfigured rather than
// erroring, so the report can still name the step outstanding.
func (h installationSetupHandlers) googleConfigured(ctx context.Context) (bool, error) {
	if h.envGoogleApp {
		return true, nil
	}
	if h.googleApp == nil {
		return false, nil
	}
	status, err := h.googleApp.Read(ctx)
	if err != nil {
		return false, err
	}
	return status.Configured, nil
}
