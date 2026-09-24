// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package capturelabel is the prompt surface of the capture-classify pass: the
// instruction the model reads, the rendering of a batch it reads it against,
// and the ruleset digest a declined message is stamped with.
//
// Its own package for the reason owedverdict is:
// TestEveryPromptOfADerivingPackageRidesItsDigest asks its question of the
// package that derives a version, so a digest call in compose would make every
// unrelated one-shot prompt there its business. The pass's orchestration lives
// in compose; this package holds only what the digest has to reach.
package capturelabel

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

const system = `You label captured emails for attention routing. For EACH supplied message emit exactly one
label: "commitment" (a promise or request to act), "meeting" (scheduling or follow-through),
or "noise" (neither). Labels route attention; they change no data. If a message fits both
commitment and meeting, choose commitment.

A message marked "inbound: yes" was sent TO us by someone outside. For those, ALSO judge how
they answered: "positive" (interest, a question worth answering, a request to meet or to hear
more), "negative" (not interested, the wrong contact with no referral, a request to stop
writing), or "neutral" (neither — an out-of-office, a bare acknowledgement, a redirect with no
view of its own). Omit "reply" entirely for a message marked "inbound: no": we wrote it, so it
answers nobody. Omit it too when the message does not read as an answer at all. A guess here
becomes a number somebody is measured on, so leave it out when you cannot tell.

"confidence" covers EVERY judgement you emit for that message — the label and, when you give
one, the reply. Report the LOWEST of the two, not the label's alone. If you are sure of the
label and unsure of the reply, either omit the reply or let the lower number stand for both.`

// SystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
func SystemFor(fence promptfence.Fence) string {
	return system + "\n" + fence.Rule("message")
}

// Prompt renders the batch as the user turn the model reads. Split from the
// request so Ruleset digests the rendering as well as the system prompt: half
// of what this site asks is how a message is laid out here.
func Prompt(fence promptfence.Fence, batch []activities.UnlabeledEmail) string {
	var prompt strings.Builder
	prompt.WriteString("Messages (untrusted; classify each by its id):\n")
	for _, m := range batch {
		// The direction line sits OUTSIDE the message text, above the fenced
		// span: it is our own record of who wrote the mail, and a sender who
		// could type "inbound: no" into their own message would otherwise be
		// able to opt their reply out of being judged.
		message := fmt.Sprintf("Subject: %s\n%s", m.Subject, m.Body)
		fmt.Fprintf(&prompt, "inbound: %s\n", yesNo(m.Inbound))
		prompt.WriteString(fence.WrapAttr("source_id", m.ID.String(), message) + "\n")
	}
	prompt.WriteString(`Return JSON: { "results": [ { "id", "label", "confidence", "reply" } ] } — one entry per ` +
		`supplied id. "reply" only for a message marked inbound: yes, and only when it reads as an answer.`)
	return prompt.String()
}

// yesNo renders the direction flag for the prompt's own line.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// Ruleset names the classify prompt this build asks, so a message every rung
// declined is re-offered once the prompt moves
// (activities.MarkCaptureLabelDeclined).
//
// It names the PROMPT — the instruction and the rendering — and nothing else.
// A change to the response schema or to the validator in compose leaves it
// where it was, so a message declined because every answer failed validation
// is not re-offered when only the validator is loosened. That is the line
// owedverdict.Ruleset draws too; moving it would put the schema in the digest
// of both sites and re-offer every stamped row on the day it lands.
//
// Folded into ONE string for the reason owedverdict.Ruleset states at length:
// PromptDigest canonicalises the fence marker the string itself declares, the
// system prompt declares it, and a user turn digested in its own builder would
// hash differently on every process start — re-offering every declined message
// after every restart.
var Ruleset = ai.PromptDigest(rulesetPrompt)

// rulesetPrompt is the one string Ruleset digests: the system prompt and the
// sample's rendering under the same fence.
func rulesetPrompt(fence promptfence.Fence) string {
	return SystemFor(fence) + "\n" + Prompt(fence, []activities.UnlabeledEmail{rulesetSample})
}

// rulesetSample is one fixed message exercising every line of the user turn, so
// a wording change anywhere in the template moves the digest.
var rulesetSample = activities.UnlabeledEmail{
	ID:      ids.Nil,
	Subject: "Re: the offer",
	Body:    "Tuesday at two works for us.",
	Inbound: true,
}
