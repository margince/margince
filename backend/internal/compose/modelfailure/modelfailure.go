// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package modelfailure answers an HTTP request whose model lane ended without
// an answer. It is a package of its own so a compose subpackage, which cannot
// import the root, can answer the same way. Every handler in compose or any
// package beneath it that can reach a model is held to it by
// TestEveryHandlerReachingAModelAnswersThroughModelFailure.
package modelfailure

import (
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
	if answered(w, r, err) {
		return
	}
	httperr.Write(w, r, err)
}

// answered answers a model lane that produced no answer, and reports whether
// it did.
//
// A model this installation cannot use is a DEPENDENCY that is down, not a
// fault in the request, and httperr.Write has no sentinel for it, so it would
// fall through to an opaque 500 whose body names nothing. The answer names the
// way through instead: nothing that waits on the assistant needs it, because
// what it would have drafted can be entered by hand.
func answered(w http.ResponseWriter, r *http.Request, err error) bool {
	if !unanswered(err) {
		return false
	}
	// Our own request refused is a defect the reader can do nothing about, and
	// this answer hides it from them, so the log is where it is found.
	if errors.Is(err, model.ErrRequestRejected) {
		slog.ErrorContext(r.Context(), "the model provider rejected the assistant's request", "err", err)
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
	httperr.Unavailable(w, r, codeAssistantUnavailable,
		"the assistant did not answer — an administrator can check the model binding under "+
			"Settings → AI. Nothing here needs the assistant; the details can be entered by hand")
	return true
}

// unanswered reports whether err is the model lane ending without an answer
// rather than this request being wrong.
//
// Matched by sentinel, never by message: ai.ErrAllTiersFailed when the walk
// reached the end of the bound rungs, and the outcomes that stop it sooner — a
// reply the models declined or the validator refused (ai.ModelDeclined), a
// request rejected, an account out of budget — and the embed lane, which has no
// walk to end, failing to answer (ai.ErrEmbedLaneFailed).
func unanswered(err error) bool {
	return errors.Is(err, ai.ErrAllTiersFailed) ||
		errors.Is(err, ai.ErrEmbedLaneFailed) ||
		ai.ModelDeclined(err) ||
		errors.Is(err, model.ErrRequestRejected) ||
		errors.Is(err, ai.ErrProviderQuota)
}

// codeAssistantUnavailable is the problem code the client reads to pick its
// own copy, rather than rendering this package's English detail at a reader
// who set another language.
//
// Held by: TestEveryReaderFacingProblemCodeHasClientCopy
// (backend/gates/frontendoauthoutcomes_test.go)
const codeAssistantUnavailable = "assistant_unavailable"
