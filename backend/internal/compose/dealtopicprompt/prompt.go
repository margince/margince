// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealtopicprompt is the capture deal topic check's prompt and the
// digest that names it. A model reads it before capture files a mail on a deal
// it only suggests.
package dealtopicprompt

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

// Input is what the model reads: one mail, and the deals it might be about,
// each described by what the workspace already knows of it.
type Input struct {
	Subject string
	Body    string
	Deals   []Deal
}

// Deal is one candidate deal as the prompt shows it.
type Deal struct {
	ID      ids.UUID
	Name    string
	Project string
	Recent  []string
}

const system = `You decide whether one captured email is about each of a few sales deals. For EACH
supplied deal id emit exactly one verdict: "about" (the email concerns this deal's business:
its offer, scope, contract, delivery, price or decision), "not_about" (it concerns something
else: an invoice for other work, a newsletter, an invitation, small talk, another project), or
"unsure" (it could be either). "unsure" is for a mail that names no subject of its own, such as
a bare request to talk; a mail that names other work is "not_about", even from the deal's own
company. A sender who writes to a customer we sell to is not by that alone writing about a deal. Judge the email's content, not who sent it.

Each deal is described by its name, its project and the subjects of mail already filed on it.
"confidence" is how sure you are of that one verdict, from 0 to 1. A wrong "about" files
unrelated mail on a deal a colleague then has to clean up, so give "unsure" when you cannot tell.`

// SystemFor names this call's data boundary; see promptfence.Fence.Rule.
func SystemFor(fence promptfence.Fence) string {
	return system + "\n" + fence.Rule("message")
}

// Prompt renders the mail and the candidate deals as the user turn. The mail
// and the subjects already filed are untrusted text inside the fence; a
// deal's and project's names are a colleague's own words.
func Prompt(fence promptfence.Fence, in Input) string {
	var prompt strings.Builder
	prompt.WriteString("Email (untrusted):\n")
	prompt.WriteString(fence.Wrap(fmt.Sprintf("Subject: %s\n%s", in.Subject, in.Body)) + "\n")
	prompt.WriteString("Deals (judge each by its id):\n")
	refs := Refs(in)
	for _, deal := range in.Deals {
		fmt.Fprintf(&prompt, "deal %s: %s", refs.For(deal.ID.String()), fence.WrapAuthored(deal.Name))
		if deal.Project != "" {
			prompt.WriteString(" (project: " + fence.WrapAuthored(deal.Project) + ")")
		}
		prompt.WriteString("\n")
		for _, subject := range deal.Recent {
			prompt.WriteString("  filed: " + fence.Wrap(subject) + "\n")
		}
	}
	prompt.WriteString(`Return JSON: { "results": [ { "deal_id", "verdict", "confidence" } ] } with one entry per supplied deal id.`)
	return prompt.String()
}

// Refs names each deal d1, d2, and so on, in the order the prompt lists them.
// The model answers by these refs; a record id never reaches it.
func Refs(in Input) *promptref.Refs {
	refs := promptref.New("d")
	for _, deal := range in.Deals {
		refs.For(deal.ID.String())
	}
	return refs
}

// Ruleset names the prompt this build asks, so a changed prompt asks again. It
// is one string for the reason capturelabel.Ruleset gives: the digest
// canonicalises the fence marker the system prompt declares.
var Ruleset = ai.PromptDigest(func(fence promptfence.Fence) string {
	return SystemFor(fence) + "\n" + Prompt(fence, rulesetSample)
})

// rulesetSample exercises every line of the user turn, so a wording change
// anywhere in the template moves the digest.
var rulesetSample = Input{
	Subject: "Re: the rollout", Body: "Can we start on Monday?",
	Deals: []Deal{{ID: ids.Nil, Name: "Rollout", Project: "ERP", Recent: []string{"Kickoff"}}},
}
