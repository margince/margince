// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The extraction call itself, split from enrichextract.go at the 500-line file
// ceiling. Building the request is one concept — what the model is shown and
// where this call's data boundary falls — and the gate that judges the reply is
// another; the ceiling is what made the seam worth drawing, but that is where it
// belonged anyway.

import (
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// companyFactsRequest builds the ONE extraction call for one source text. It is
// a pure function of that source, not a method, so the certification lane issues
// the request the extractor issues rather than a copy — a copy stays green
// through the change that breaks the original. The text arrives already bounded,
// so the model and the gate read one text; the fence is minted per request.
//
//promptlang:exempt gateEvidence drops any field whose text is not in the page's own bytes, so translating a foreign-language page's facts would drop every one of them.
//promptvoice:exempt the reply is field values checked against the page's own bytes; a value we phrased would be a value gateEvidence drops.
func companyFactsRequest(sourceLabel, sourceText, sourceURL string) model.Request {
	// The page goes in exactly as it was fetched. A verbatim-markdown page can
	// carry a literal </untrusted>, and it is welcome to: the boundary is this
	// call's nonce, which the page's author has never seen. Passing the bytes
	// through is what lets the evidence gate match a quote against the page as
	// WRITTEN — the gate and the model read the same text.
	fence := promptfence.New()
	// The URL names the source, so it belongs in the prompt — INSIDE the
	// boundary, like the page it points at. An attacker publishes the link that
	// put it here, and only its host is pinned: the path and query are theirs to
	// write, and a path reads as prose just as well as a paragraph does.
	header := sourceLabel
	if sourceURL != "" {
		header += " " + fence.Wrap(sourceURL)
	}
	return model.Request{
		System: companyFactsSystemFor(fence),
		Messages: []model.Message{{
			Role:    "user",
			Content: header + ":\n" + fence.Wrap(sourceText),
		}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: companyFactsSchema,
		SecretStripper: ai.NewSecretStripper(),
	}
}
