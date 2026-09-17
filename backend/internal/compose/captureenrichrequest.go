// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The signature-enrich REQUEST: what one candidate's model call is made of, and
// the window of their mail it reads.
//
// Its own file because it is a pure function of the candidate — the
// certification lane issues the same request the pass does, and a request
// re-created beside the lane would certify a call this product never makes.

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// signatureEnrichRequest builds the ONE model call that reads one candidate's
// signature. It is a pure function of the candidate and the window their mail
// yielded so the same request can be issued outside the pass — by the
// certification lane — without re-creating it, because a re-creation certifies a
// copy rather than the prompt that ships.
//
// The lines arrive already derived by signatureBlock rather than being derived
// here: the evidence gate matches every quote against the SAME window the model
// was shown, so one derivation feeds both readers and neither can drift.
//
// The fence is minted here, per request: a boundary reused across calls is one a
// previous sender has already been shown, and every field of this prompt is
// their own writing.
//
//promptlang:exempt the fields are a title and a phone number copied out of the contact's own signature block, each carrying an evidence_snippet checked against those lines — a job title is written the way its holder writes it, and translating one would both change the fact and fail the snippet check.
//promptvoice:exempt the fields are a title and a phone number copied out of a signature block, each checked against those lines; there is no sentence of ours here to have a voice.
func signatureEnrichRequest(cand contacts.SignatureCandidate, lines string) model.Request {
	fence := promptfence.New()
	var prompt strings.Builder
	// The contact's own name and address are theirs to write, so they go INSIDE
	// the boundary like the signature does. Interpolated into a header line they
	// would be reading in the prompt's own voice, which is the whole attack.
	prompt.WriteString("Contact (untrusted):\n")
	prompt.WriteString(fence.Wrap(fmt.Sprintf("Name: %s\nEmail: %s", cand.FullName, cand.Email)) + "\n")
	// Everything the vocabulary admits, and no statement about what the record
	// already holds. The pass reads a signature to find out whether what it
	// holds is still true, so naming the empty fields would ask the narrower
	// question and miss the number that changed.
	prompt.WriteString("Fields to extract when stated: [\"title\",\"phone\",\"linkedin\",\"company_name\",\"address\",\"website\"]\n")
	prompt.WriteString("Signature block (untrusted; the trailing lines of their last email):\n")
	prompt.WriteString(fence.WrapAttr("source_id", cand.ActivityID.String(), lines) + "\n")
	prompt.WriteString(`Return JSON: { "fields": [ { "field", "value", "evidence_snippet", "confidence" } ] }`)

	return model.Request{
		System:         signatureEnrichSystemFor(fence),
		Messages:       []model.Message{{Role: chatRoleUser, Content: prompt.String()}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: signatureEnrichSchema(),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// signatureBlock returns the trailing signatureLineCount non-quoted,
// non-empty-tail lines of a stored email body — the §2.9 source window.
// Quoted history (">"-prefixed) is not identity evidence and is excluded.
func signatureBlock(body string) string {
	lines := strings.Split(body, "\n")
	var kept []string
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), ">") {
			continue
		}
		kept = append(kept, l)
	}
	// Trim trailing blank lines so the window holds content, not padding.
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	if len(kept) > signatureLineCount {
		kept = kept[len(kept)-signatureLineCount:]
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}
