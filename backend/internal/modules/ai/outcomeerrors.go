// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// How the adapters build the port's two outcome sentinels,
// model.ErrOutputWithheld and model.ErrRequestRejected: a provider that
// declined to deliver the answer, or refused the request itself.

import (
	"net/http"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// finishRefusal is the terminal every wire that names a model refusal spells
// the same way: Anthropic's stop_reason and OpenAI's content part type.
const finishRefusal = "refusal"

// withheldError is a withholding terminal that still NAMES itself, so the
// stored call says which filter fired and not merely that one did.
type withheldError struct {
	wire   string
	reason string
	// detail is text the provider chose, already passed through
	// safeProviderText where it came from a remote party.
	detail string
}

func (e withheldError) Error() string {
	msg := "ai: " + e.wire + ": answer withheld: " + e.reason
	if e.detail != "" {
		msg += ": " + e.detail
	}
	return msg
}

// FinishReason satisfies the accessor finishReasonFor probes for.
func (e withheldError) FinishReason() string { return e.reason }

func (e withheldError) Unwrap() error { return model.ErrOutputWithheld }

// rejectsTheRequest reports whether an HTTP status is the provider's verdict on
// the request's own content.
//
// A positive list rather than "any 4xx": 401 and 403 are the credentials, 404 a
// model id, 402 an account and 408 a timeout. None of those is a statement
// about what was asked, and reading one as such would record a misconfigured
// binding as a model that cannot do the task.
func rejectsTheRequest(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return true
	default:
		return false
	}
}
