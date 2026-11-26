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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

const system = `You label captured messages for attention routing: emails, and chat messages from a messaging
channel. For EACH supplied message emit exactly one
label: "commitment" (a promise to deliver or do something, or a request for a deliverable, a
decision or a payment), "meeting" (a request for, offer of or confirmation of time together, such
as a call, a visit or a slot, or follow-through on one), or "other" (neither a commitment nor a meeting; says nothing about importance). Asking to meet is
"meeting", not a request to act. A decision the sender reports, such as declining to go ahead,
requests nothing: it is "other" unless the message also promises or asks something of us. Labels route attention; they change no data. If a message both
promises or requests something of the first kind AND asks for time, choose commitment.

An email opens with its subject line, then the text under "Sender's own words:", which is
what its sender wrote in that mail; the older mails it quoted have been cut away. The subject
helps the label only. In a reply it is usually the old subject of the thread, which anybody may
have written. A chat message has a "channel:" line before it naming the chat service, and usually
no subject. It is one message of a conversation you do not see, often a single short line: label
it by what that line itself asks, offers or promises, the same way you would label an email.

A message marked "inbound: yes" was sent TO us by someone outside. For an inbound EMAIL, ALSO
judge how the sender answered, from their own words alone and never from the subject: "positive" (interest, a question worth
answering, a request to meet or to hear more), "negative" (the sender declines to go ahead, or
asks us to stop writing), or "neutral" (neither — an out-of-office, a bare acknowledgement, a
request to wait, the wrong recipient, a redirect). Set "reply" to null for a message marked
"inbound: no": we wrote it, so it answers nobody. Set it to null for every chat message: a chat
line answers the line before it, which you do not see, so its "no" may turn down a time rather
than the deal. Set it to null too when the message has no
text of its own or does not read as an answer at all. A "negative" can close the customer's deal
as lost, so give it only when the sender says no plainly, and null when you cannot tell.

"confidence" covers EVERY judgement you emit for that message — the label and, when you give
one, the reply. Report the LOWEST of the two, not the label's alone. If you are sure of the
label and unsure of the reply, either set the reply to null or let the lower number stand for both.`

// SystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
func SystemFor(fence promptfence.Fence) string {
	return system + "\n" + fence.Rule("message")
}

// Prompt renders the batch as the user turn the model reads. Split from the
// request so Ruleset digests the rendering as well as the system prompt: half
// of what this site asks is how a message is laid out here.
func Prompt(fence promptfence.Fence, batch []activities.UnlabeledMessage) string {
	refs := Refs(batch)
	var prompt strings.Builder
	prompt.WriteString("Messages (untrusted; classify each by its id):\n")
	for _, m := range batch {
		// The channel and direction lines are our own record, so they sit above
		// the fenced span. Inside it, a sender could type "inbound: no" and opt
		// their reply out of being judged.
		if m.IsChat() {
			fmt.Fprintf(&prompt, "channel: %s\n", m.ChannelProvider)
		}
		fmt.Fprintf(&prompt, "inbound: %s\n", yesNo(m.Inbound))
		prompt.WriteString(fence.WrapAttr("source_id", refs.For(m.ID.String()), messageText(m)) + "\n")
	}
	prompt.WriteString(`Return JSON: { "results": [ { "id", "label", "confidence", "reply" } ] } — one entry per ` +
		`supplied id. "reply" only for an email marked inbound: yes, and only when it reads as an answer.`)
	return prompt.String()
}

// messageText is the fenced part of one message. An email always shows its
// subject line; a chat shows one only when its channel carried a subject.
func messageText(m activities.UnlabeledMessage) string {
	words := "Sender's own words:\n" + m.Body
	if m.IsChat() && m.Subject == "" {
		return words
	}
	return "Subject: " + m.Subject + "\n" + words
}

// Refs names each message of a batch m1, m2, … in batch order: the ids the
// prompt shows and the reply answers by.
func Refs(batch []activities.UnlabeledMessage) *promptref.Refs {
	refs := promptref.New("m")
	for _, m := range batch {
		refs.For(m.ID.String())
	}
	return refs
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
	return SystemFor(fence) + "\n" + Prompt(fence, rulesetSamples)
}

// rulesetSamples are one email and one chat that use every line of the user
// turn. A wording change anywhere in the template then moves the digest.
var rulesetSamples = []activities.UnlabeledMessage{{
	ID:      ids.Nil,
	Kind:    string(crmcontracts.ActivityKindEmail),
	Subject: "Re: the offer",
	Body:    "Tuesday at two works for us.",
	Inbound: true,
}, {
	ID:              ids.MustParse("00000000-0000-0000-0000-000000000001"),
	Kind:            activities.KindMessage,
	ChannelProvider: "openchannel",
	Body:            "Are you free at 8:00 PM Monday?",
	Inbound:         true,
}}
