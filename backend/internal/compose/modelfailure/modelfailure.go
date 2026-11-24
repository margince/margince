// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package modelfailure answers an HTTP request whose model lane ended without
// an answer. It is a package of its own so a compose subpackage, which cannot
// import the root, can answer the same way. Every handler in compose or any
// package beneath it that can reach a model is held to it by
// TestEveryHandlerReachingAModelAnswersThroughModelFailure.
package modelfailure

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Write answers err: a model lane that produced no answer as a 503 the client
// has copy for, and anything else through httperr.Write.
func Write(w http.ResponseWriter, r *http.Request, err error) {
	httperr.Write(w, r, Refusal(r.Context(), err))
}

// Refusal is err as a caller should read it. A model lane that produced no
// answer becomes a 503 with a code the client has copy for. Anything else is
// returned unchanged. A handler outside compose, which cannot import this
// package, is handed its reader's errors through it.
//
// A model this installation cannot use is a dependency that is down, not a
// fault in the request. httperr would answer it as an opaque 500.
func Refusal(ctx context.Context, err error) error {
	if !Unanswered(err) {
		return err
	}
	if errors.Is(err, ai.ErrBudgetDeferred) {
		return httperr.UnavailableProblem(codeAllowanceUsedUp,
			"the monthly AI allowance is used up — an administrator can raise it under Settings → AI, "+
				"or it renews at the start of the next budget window")
	}
	if down, ok := errors.AsType[*ai.ProviderDownError](err); ok {
		return providerDown(down)
	}
	// Our own request refused is a defect the reader can do nothing about, and
	// this answer hides it from them, so the log is where it is found.
	if errors.Is(err, model.ErrRequestRejected) {
		slog.ErrorContext(ctx, "the model provider rejected the assistant's request", "err", err)
	}
	// A CODE, because this sentence has a reader: the client renders it in
	// their language, and a detail written here would arrive in English. The
	// detail stays for a caller with no catalog.
	//
	// It says the assistant did not ANSWER, and does not say why: a provider
	// that is down, a credential it refused, a model nobody bound, an answer it
	// withheld or that failed validation, a request it rejected. Naming one
	// would be a guess most of the time, so Settings → AI is offered as the
	// place to look, not the diagnosis.
	return httperr.UnavailableProblem(codeAssistantUnavailable,
		"the assistant did not answer — an administrator can check the model binding under "+
			"Settings → AI. Nothing here needs the assistant; the details can be entered by hand")
}

// providerDown answers a call the router refused because the provider is
// known to be blocked.
//
// Unlike the generic answer this one knows the cause, so it names it and the
// remedy: only the installation's administrator can top up an account, replace
// a credential or wait out an outage. The detail carries no provider text, key
// or host, and each code is a separate call because the client copy gate reads
// the code at the call site.
func providerDown(down *ai.ProviderDownError) error {
	switch down.Health {
	case model.HealthOutOfCredit:
		return httperr.UnavailableProblem(codeProviderOutOfCredit,
			"the AI provider has no credit left — contact your system administrator")
	case model.HealthUnauthorized:
		return httperr.UnavailableProblem(codeProviderUnauthorized,
			"the AI provider refused the configured credential — contact your system administrator")
	default:
		return httperr.UnavailableProblem(codeProviderUnavailable,
			"the AI provider is not answering — try again later or contact your system administrator")
	}
}

// Unanswered reports whether err is the model lane ending without an answer
// rather than this request being wrong.
//
// Matched by sentinel, never by message. ai.ErrAllTiersFailed is a walk that
// reached the end of the bound rungs. Other outcomes stop it sooner. A reply
// is declined or refused (ai.ModelDeclined), or a request rejected. Or an
// account is out of budget, or the router deferred the call. The embed lane
// has no walk, and fails with ai.ErrEmbedLaneFailed.
func Unanswered(err error) bool {
	return errors.Is(err, ai.ErrAllTiersFailed) ||
		errors.Is(err, ai.ErrEmbedLaneFailed) ||
		ai.ModelDeclined(err) ||
		errors.Is(err, model.ErrRequestRejected) ||
		errors.Is(err, ai.ErrProviderQuota) ||
		ai.IsDeferral(err)
}

// codeAssistantUnavailable is the problem code the client reads to pick its
// own copy, rather than rendering this package's English detail at a reader
// who set another language.
//
// Held by: TestEveryReaderFacingProblemCodeHasClientCopy
// (backend/gates/frontendoauthoutcomes_test.go)
const codeAssistantUnavailable = "assistant_unavailable"

// The reason-specific answers for the budget stop and for a provider the
// router knows is blocked; the client has its own copy for each.
//
// Held by: TestEveryReaderFacingProblemCodeHasClientCopy
const (
	codeAllowanceUsedUp      = "ai_allowance_used_up"
	codeProviderOutOfCredit  = "provider_out_of_credit"
	codeProviderUnauthorized = "provider_unauthorized"
	codeProviderUnavailable  = "provider_unavailable"
)
