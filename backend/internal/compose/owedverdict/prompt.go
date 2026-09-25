// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package owedverdict is the prompt surface of the owed-verdict pass: the
// instruction the model reads, the rendering of a batch it reads it against,
// and the ruleset digest stamped on every row the pass decides.
//
// Its own package for the reason the six other digesting surfaces are each
// their own: TestEveryPromptOfADerivingPackageRidesItsDigest asks its question
// of the package that derives a version, so a digest call in a package holding
// twenty unrelated one-shot prompts makes all twenty its business. In compose
// that cost twenty ratifications carrying one paragraph, and would have cost a
// twenty-first for the next prompt added anywhere in compose.
//
// The pass's ORCHESTRATION stays in compose, where the store and the model lane
// it drives are. What moved is only what the digest has to reach — which is the
// same line the other six draw.
package owedverdict

import (
	"fmt"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
)

const system = `You judge whether an inbound business message asks its recipient side for something.
For EACH supplied message emit exactly one verdict: "asks_us" (it puts a question, a request or a
decision to the recipient side and waits on them) or "informs_us" (it reports, confirms, notifies or
acknowledges, and waits on nobody).

Judge what the message ASKS, never how important it is. A report about a large account is still
informs_us. A one-line question about a small one is still asks_us.

The recipient line decides WHO is asked: a request is made of the To recipients. A message whose
To line is somebody else — another organisation's address, or a desk such as accounts@ — with the
reader only in Cc is informs_us even when its text asks for something, because it asks them; it
is asks_us only when the text names the copied reader as the one to act. A message that
carries a calendar invitation is asks_us only when it also asks something a calendar reply cannot
answer.
A message WITHOUT a calendar invitation that proposes a specific time for a call or a meeting, or
accepts one the recipient side has not yet confirmed, is asks_us: the slot is not agreed until they
answer, so the sender is waiting on them. A message confirming a time the recipient side has
already agreed is informs_us — it closes the arrangement rather than opening it. The invitation
rule above is the one exception: a time offered as a calendar invitation is answered from the
calendar.
Some messages are shown with our own earlier message in the same thread, in a span marked
context_for. Read it only to understand what the reply answers or leaves open; judge the reply's
own words, never ours. A reply is asks_us when it leaves the recipient side something to do — a
question to answer, a time to confirm, a point it defers or reserves. A reply that answers
everything we asked and leaves nothing open is informs_us, however long it is.
Judge only the sender's new words. Quoted earlier requests and signatures do not create a new
obligation. Acknowledgements, returning a document, and "I will get back to you" are informs_us
unless the new text separately asks the recipient to do something.`

// SystemFor names THIS call's data boundary; see promptfence.Fence.Rule.
func SystemFor(fence promptfence.Fence) string {
	return system + "\n" + fence.Rule("message")
}

// Prompt renders the batch as the user turn the model reads.
//
// Split out of owedRequest so Ruleset can digest the rendering as well as
// the system prompt: half of what this site asks lives in how a message is laid
// out here, and a stamp blind to that would keep serving verdicts reached under
// wording that has since moved.
func Prompt(fence promptfence.Fence, batch []activities.OwedCandidate) string {
	var prompt strings.Builder
	prompt.WriteString("Messages (untrusted; judge each by its id):\n")
	for _, m := range batch {
		// Our own earlier message, in its OWN span before the one being judged.
		//
		// Sequential rather than nested: the fence has one close marker, and the
		// shape validator counts one source_id span per id. Fenced even though
		// we wrote it — a connector-captured outbound body carries the
		// customer's quoted text, so it is not ours all the way down.
		if prior := m.PriorOutbound; prior != nil {
			var context strings.Builder
			context.WriteString("Our earlier message in this thread (context only, not judged):\n")
			fmt.Fprintf(&context, "Subject: %s\n", prior.Subject)
			fmt.Fprintf(&context, "Sent: %s\n", prior.At.Format(time.DateOnly))
			context.WriteString("\n" + activities.SplitEmailBody(prior.Body).Main)
			prompt.WriteString(fence.WrapAttr("context_for", m.ID.String(), context.String()) + "\n")
		}
		var message strings.Builder
		fmt.Fprintf(&message, "Subject: %s\n", m.Subject)
		// The envelope, which is half the question: a report to a desk address
		// with the reader copied reads exactly like a direct request without it.
		if len(m.To) > 0 {
			fmt.Fprintf(&message, "To: %s\n", strings.Join(m.To, ", "))
		}
		if len(m.Cc) > 0 {
			fmt.Fprintf(&message, "Cc: %s\n", strings.Join(m.Cc, ", "))
		}
		if m.HasCalendarPart {
			message.WriteString("This message carried a calendar invitation.\n")
		}
		message.WriteString("\n" + activities.SplitEmailBody(m.Body).Main)
		prompt.WriteString(fence.WrapAttr("source_id", m.ID.String(), message.String()) + "\n")
	}
	prompt.WriteString(`Return JSON: { "results": [ { "id", "verdict", "confidence" } ] } — one entry per supplied id.`)
	return prompt.String()
}

// rulesetSample is one fixed candidate that exercises every optional line
// of Prompt, so a label edit anywhere in the template moves the stamp.
//
// Fixed in every field, including the id and the date: the digest must be a
// property of the CODE, and anything minted per call would move it on every
// process start.
var rulesetSample = activities.OwedCandidate{
	// The contract's own word for the kind, like the cert case beside it: a
	// second spelling here would stamp a sample the store never produces.
	ID: ids.Nil, Kind: string(crmcontracts.ActivityKindEmail),
	Subject: "Re: the plan", Body: "Tuesday 14:00 suits us.",
	To: []string{"we@example.test"}, Cc: []string{"desk@example.test"},
	HasCalendarPart: true,
	PriorOutbound: &activities.PriorOutbound{
		Subject: "the plan", Body: "Here is the plan. When suits you?",
		At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	},
}

// Ruleset names the rules a verdict written by this build was judged under.
//
// A digest rather than a hand-kept number, because the promise on the row is
// "these are the rules that judged it" and a constant somebody must remember to
// bump is a promise the tree cannot keep. Six surfaces already stamp a prompt
// this way.
//
// IT FOLDS THE SYSTEM PROMPT AND THE USER TURN INTO ONE STRING, which departs
// from all six of them — they digest a system prompt alone — and the departure
// is deliberate twice over.
//
// It is necessary because half the judgement is the rendering: the context_for
// block is new wording the model reads, and a system-only stamp would hold
// still while it changed.
//
// It must be ONE string because PromptDigest canonicalises each builder's
// output against the marker THAT STRING declares. The system prompt declares
// the fence marker; a user turn does not, so a user turn digested in its own
// builder keeps a live nonce and hashes differently on every process start.
// Measured, not assumed: folded is stable across fences, and both the user turn
// alone and the variadic PromptDigest(system, user) form are not. The variadic
// form is the trap, because it is the shape a tidy-up would reach for and its
// failure mode is re-judging every workspace after every restart, silently.
// TestAUserTurnDigestedAloneIsNotStable holds this.
var Ruleset = ai.PromptDigest(func(fence promptfence.Fence) string {
	return SystemFor(fence) + "\n" + Prompt(fence, []activities.OwedCandidate{rulesetSample})
})
