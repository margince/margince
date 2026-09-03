// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What this installation still has to be configured with before it can be used.
//
// One surface answering one question, because the onboarding gate has to make a
// single decision — may this reader proceed — and composing that out of
// /ai/profile plus an OAuth-app read would put the installation's own policy in
// the client. The moment a posture needs a different rule (a deployment serving
// no mail, a local-model install with no vendor to key) the rule changes here
// and every screen follows.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	// connectorApps reports whether the installation holds a vendor's OAuth app.
	connectorApps *capture.ConnectorAppStore
	// envConnectorApp records that this PROCESS was composed with a connector
	// OAuth app from its environment, for either vendor.
	//
	// It is part of the answer because it is part of what the connect transport
	// resolves: the stored app wins, and the environment is the fallback. An
	// installation that exports a pair has a working mail integration, so
	// reporting its step unconfigured would tell an operator to go and store an
	// app the process already resolves.
	//
	// The two are not two sources of truth for STORAGE — the setting is the only
	// place a write lands. They are one honest reading of a two-source
	// resolution, and this field is how the reading stays identical to the
	// transport's.
	envConnectorApp bool
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
	steps, err := h.steps(ctx)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.InstallationSetup{
		Complete: stepsComplete(steps),
		Steps:    steps,
	})
}

// steps reads the setup sources once, in the fixed order the contract
// promises a reader. GetInstallationSetup and firstRunAnswer both read
// through here, so a step's configured bit is asked once no matter which
// caller wants the answer.
//
// ORDER IS THE CONTRACT. `steps` is declared as "every setup step, in the
// order a reader should complete them", and a client walks it in the order
// it arrives. A client sorting these itself would be re-deciding a sequence
// the server owns, so the sequence is pinned by
// TestTheSetupReportListsTheStepsInTheOrderOnboardingWalksThem.
//
// ONLY THE MODEL BINDING BLOCKS, pinned by
// TestOnlyTheModelBindingBlocksFirstRun. Without one the product cannot
// perform the cold-start read that first run is, so there is nothing to let
// a reader through to. A connector OAuth app buys mailbox capture and
// nothing else: an installation signing in with passwords and no external
// provider is fully usable without one.
//
// The app is configured from settings, where each vendor's card carries the
// redirect URIs that vendor's console asks for. The step stays in the report
// because that is what `blocking` is for: reporting it and calling it
// optional is the answer. Dropping it would leave an installation that has no
// app and one that has a working mail integration reporting the same thing to
// a reader asking what is left to do.
func (h installationSetupHandlers) steps(ctx context.Context) ([]crmcontracts.InstallationSetupStep, error) {
	aiReady, err := h.aiConfigured(ctx)
	if err != nil {
		return nil, err
	}
	appReady, err := h.connectorAppConfigured(ctx)
	if err != nil {
		return nil, err
	}
	return []crmcontracts.InstallationSetupStep{
		{Step: crmcontracts.AiModels, Configured: aiReady, Blocking: true},
		{Step: crmcontracts.OauthApp, Configured: appReady, Blocking: false},
	}, nil
}

// stepsComplete is exactly "no blocking step is unconfigured" — the contract's
// own definition of InstallationSetup.complete, kept in one place so a client
// and a second server-side reading of it can never drift apart.
func stepsComplete(steps []crmcontracts.InstallationSetupStep) bool {
	for _, s := range steps {
		if s.Blocking && !s.Configured {
			return false
		}
	}
	return true
}

// firstRunAnswer resolves AuthCapabilities.first_run: !complete, read through
// the SAME steps GetInstallationSetup reads, so the anonymous probe and a
// signed-in reader's full report can never disagree about one installation.
//
// It reads s.installationSetupHandlers at request time, through the closure,
// rather than copying it now — the same reason resolveOverlayIncumbent reads
// s.vault lazily. WithKeyvault installs the underlying AI/connector stores and
// WithGmailCapture/WithGraphCapture set envConnectorApp, and New wires this
// only after every option has run; reading through the pointer keeps that
// true even if the wiring ever moves ahead of an option.
//
// svc is the ONE identity.Service New() builds for the whole process, passed
// in rather than constructed again here, so this shares its singleton
// workspace cache with every other reader of it.
//
// It resolves the singleton workspace itself, because the anonymous
// capabilities probe carries no session and so no workspace of its own
// (A107/ADR-0061: no tenant selector) — unlike GetInstallationSetup, which
// takes the workspace the cookie session already bound.
func (s *Server) firstRunAnswer(svc *identity.Service) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		wsID, err := svc.InstallationWorkspace(ctx)
		if errors.Is(err, identity.ErrNotBootstrapped) {
			// No organization has been claimed yet (ADR-0105): the most "first
			// run" an installation gets, and every other boot-time reader of
			// this state treats it as "not yet" rather than as an error.
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("resolving the installation for the first-run signal: %w", err)
		}
		readCtx := principal.WithCorrelationID(
			principal.WithActor(principal.WithWorkspaceID(ctx, wsID.UUID), principal.Principal{
				Type: principal.PrincipalSystem,
				ID:   installationSetupReadActor,
			}), ids.NewV7(),
		)
		steps, err := s.steps(readCtx)
		if err != nil {
			return false, err
		}
		return !stepsComplete(steps), nil
	}
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

// connectorAppConfigured reports whether this installation can connect a mailbox
// at all — through EITHER vendor, because either one makes the step done and
// requiring both would report an installation running happily on Google as
// unfinished forever.
//
// The environment's copy counts, in the same precedence the connect transport
// applies — see envConnectorApp. A nil store is a role that composed no vault,
// and then only the environment can answer: it reads as unconfigured rather than
// erroring, so the report can still name the step outstanding.
func (h installationSetupHandlers) connectorAppConfigured(ctx context.Context) (bool, error) {
	if h.envConnectorApp {
		return true, nil
	}
	if h.connectorApps == nil {
		return false, nil
	}
	// Every vendor the capture module holds an app for, asked FROM it: a list
	// spelled here would go on reporting two after a third was added, and the
	// step would read as unconfigured on an installation that had configured it.
	// A read failure does NOT end the scan. The question is whether ANY vendor
	// is configured, so one unreadable row cannot answer it — and failing the
	// whole setup report over Microsoft's row would tell an installation that
	// had configured Google it had configured nothing. The error is kept and
	// raised only if no vendor answered yes, where it is the difference between
	// "not configured" and "we could not tell".
	var unreadable error
	for _, p := range capture.AppProviders() {
		status, err := h.connectorApps.Read(ctx, p)
		if err != nil {
			unreadable = errors.Join(unreadable, err)
			continue
		}
		if status.Configured {
			return true, nil
		}
	}
	return false, unreadable
}
