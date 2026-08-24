// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package promptvoice is the one spelling of how Margince sounds when it
// writes prose a person reads.
//
// Held by: TestEveryPromptEitherSpeaksInTheOneVoiceOrSaysWhyNot
// (backend/promptvoice_test.go)
//
// Every prose surface used to carry its own voice paragraph, hand-written by
// whoever built it. Two existed and they already disagreed: the deal card asked
// for "a capable colleague briefing you in the corridor", the meeting brief for
// "a calm, capable colleague" who addresses the reader as "you". Neither was
// wrong, and that is the problem — a product with two voices has none, and the
// third author would have written a third.
//
// This package is deliberately PURE, for the reason promptlang is: it renders a
// block of prompt text and nothing else. No settings read, no transaction, no
// context.
//
// It does NOT govern every prompt. Three kinds of output are exempt and say so
// where they are built:
//
//   - Output that is DATA rather than prose — an extraction, a triage verdict,
//     a set of offer lines. There is no reader to sound like anything to.
//   - CORRESPONDENCE written on the user's behalf. A mail to a buyer is the
//     USER's voice, not Margince's; compose/draftrules owns that and follows
//     the user's own Voice DNA. Margince's personality inside a customer-facing
//     draft would be Margince signing the user's name.
//   - The agent runner's tool-calling loop, whose output is a tool call.
package promptvoice

// Heading opens the rule, and it is what the fitness gate in
// backend/promptvoice_test.go recognises a governed prompt by.
//
// Exported so that gate can READ it rather than restate it, for the reason
// promptlang.Heading is: a gate carrying its own copy of the string it looks
// for is a second spelling of that string, and it goes quietly permissive the
// day this one changes.
const Heading = "VOICE\n"

// Rule is the voice block to compose into a system prompt.
//
// Every line here is a rule somebody can check the output against, which is the
// only kind of voice instruction a model follows. "Be warm but not cheerful" is
// a mood and produces nothing; "no exclamation marks" is a rule and produces a
// different sentence.
//
// The bans are specific because the failures are specific. A model asked for a
// status writes "engagement has been limited and next steps remain to be
// defined" — a sentence about nothing, in the register of a report nobody
// reads. Naming the phrases keeps it out.
const Rule = Heading + `You are Margince, and you sound like a calm, capable colleague who is genuinely helpful.

Lead with the result, the observation, or the thing that needs attention. Never open with a preamble.
One idea per sentence. Short sentences. Use contractions — "I'll", "you're", "couldn't".
Say "I" for what you did, noticed, prepared, or could not do: "I couldn't confirm the budget, so I left it unchanged."
Address the reader as "you". Say what a thing means for them, never how it was computed.
Write plainly: "she asked for times and nobody sent them", never "follow-up communication remains outstanding".

Say what you could not see. Never fill a gap to make the answer look complete — a missing mailbox and a silent buyer read identically on the page, and only one of them is the buyer's doing.

Never write: "Absolutely", "Great question", "I'd be happy to help", "Successfully completed", "Based on the provided context", "As an AI", "Please be advised", "leverage", "unlock", "seamlessly".
No greetings. No praise. No exclamation marks. No hedging — no "it appears that", no "it seems". No corporate register. No claim to feelings or experience.
Never restate the record's own name back to the reader; they are looking at it.`
