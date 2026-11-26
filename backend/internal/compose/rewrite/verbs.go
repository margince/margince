// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package rewrite

// Verb is one rewrite a rep asks for with one press.
type Verb string

// The verbs crm.yaml's RewriteRequest names.
const (
	VerbPolish   Verb = "polish"
	VerbShorter  Verb = "shorter"
	VerbWarmer   Verb = "warmer"
	VerbFormal   Verb = "formal"
	VerbDeadline Verb = "deadline"
)

// instructions are what each verb asks of the model. They live here, never in
// the request, so no caller's text reaches the prompt as an instruction.
var instructions = map[Verb]string{
	VerbPolish:   "Improve the wording and flow. Keep the meaning, the facts and the length.",
	VerbShorter:  "Keep the meaning and use fewer words.",
	VerbWarmer:   "Warmer in tone, without getting familiar.",
	VerbFormal:   "More formal in tone.",
	VerbDeadline: "Ask for an answer by a named date.",
}

// Envelope says which words are rewritten. It is the one thing the two
// callers of this path differ in.
type Envelope string

// The envelopes crm.yaml's RewriteRequest names.
const (
	EnvelopeEmail Envelope = "email"
	EnvelopeNote  Envelope = "note"
)

// envelopeRules tells the model what the words are and what their form keeps.
var envelopeRules = map[Envelope]string{
	EnvelopeEmail: `The words are the body of an email the salesperson is about to send. There is no subject to rewrite: return an empty subject.
Keep the greeting line as its own line, then a blank line. Write plain text, with a blank line between paragraphs. Write no sign-off and no signature: sending adds the sender's own.`,
	EnvelopeNote: `The words are a task or note the salesperson typed into a CRM, so a colleague reads it clearly at a glance.
Add no greeting, no sign-off and no comment on the text. The subject is one short line. The body is markdown: keep its lists, emphasis and links as markdown.`,
}

// Admits reports whether the envelope offers the verb. A note offers Polish
// alone; the composer offers every verb.
func (e Envelope) Admits(v Verb) bool {
	if _, known := instructions[v]; !known {
		return false
	}
	switch e {
	case EnvelopeEmail:
		return true
	case EnvelopeNote:
		return v == VerbPolish
	default:
		return false
	}
}
