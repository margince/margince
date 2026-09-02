// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

// Writing the forwardable note, with a template underneath it.
//
// The shape mirrors org360's introdraftwrite.go deliberately — model lane,
// deterministic floor, generated_by on the way out — because a second drafting
// site that degraded differently would be a second contract for what happens
// when the lane is missing. What differs is the PROMPT and the wording table,
// which is the whole reason this exists: that one writes to a colleague, and
// this writes what a colleague forwards to a customer.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/draftreply"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/compose/promptvoice"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// introNote is one written note.
type introNote struct {
	subject string
	body    string
}

// writeIntroNote asks the model, and falls back to the template.
//
// The floor is not a degraded answer. A forwardable note is short and its facts
// are few, so a template states every one of them honestly — a deployment with
// no model lane gets a note a colleague can actually paste rather than an
// apology about a lane they cannot configure. What the model buys is phrasing,
// and generated_by says which one wrote it.
func writeIntroNote(
	ctx context.Context, lane Completer, facts noteFacts,
) crmcontracts.AccountEmailDraft {
	floor := noteFloor(facts)
	if lane == nil {
		return wireIntroNote(floor, crmcontracts.Deterministic, facts)
	}
	written, err := noteFromModel(ctx, lane, facts)
	if err != nil {
		return wireIntroNote(floor, crmcontracts.Deterministic, facts)
	}
	return wireIntroNote(written, crmcontracts.Model, facts)
}

// validatedLane is a lane that can re-ask when the reply is refused.
//
// The refusal message goes back to the model, which is the difference between
// a rule it broke once and a rule it never learns: this note reaches a
// customer, and losing the lane costs the colleague the phrasing.
type validatedLane interface {
	CompleteValidated(ctx context.Context, req model.Request, validate ai.Validator) (model.Response, error)
}

// noteFromModel writes the note and checks what came back.
//
// The retry is judged by the SAME parse the answer path runs, not a looser
// shape check: a reply that parses as JSON and still names nobody is exactly
// the reply a retry can fix, and telling the model only "be valid JSON" would
// spend the attempt without saying what was wrong.
func noteFromModel(
	ctx context.Context, lane Completer, facts noteFacts,
) (introNote, error) {
	req := noteRequest(facts)
	var res model.Response
	var err error
	if structured, ok := lane.(validatedLane); ok {
		res, err = structured.CompleteValidated(ctx, req, func(text string) error {
			_, parseErr := parseIntroNote(text, facts)
			return parseErr
		})
	} else {
		res, err = lane.Complete(ctx, req)
	}
	if err != nil {
		return introNote{}, fmt.Errorf("network: drafting the introduction note: %w", err)
	}
	return parseIntroNote(res.Text, facts)
}

const noteSystem = `You write one short note that a person will FORWARD to somebody they know, introducing a colleague of theirs.

The reader is the recipient — a customer or a prospect, not a teammate. You are writing in the voice of the person who will send it: they know the recipient, and they are passing along an introduction.

Rules you must not break:
- Write TO the recipient, and address them by name: open with their first name. Never mention that anybody was asked to make this introduction, and never refer to an internal request.
- Say who is being introduced, naming them in full, and why the recipient might care, in one sentence each.
- Write a short subject line in the "subject" field, naming the colleague you are introducing.
- Do not invent anything about the relationship or about the recipient's company. You are told how warm the relationship is and when they last spoke; say no more than that.
- Ask for nothing more than a conversation. No pitch, no pricing, no meeting times.
- No subject line inside the body.`

// noteRequest builds the model call.
//
// Every fact is fenced, including the ones this server minted: a contact's name
// and a colleague's were both typed by a person, and the rep's own
// value_for_target is free text straight off a request body — the most obvious
// injection surface on this call.
func noteRequest(facts noteFacts) model.Request {
	fence := promptfence.New()
	payload, err := json.Marshal(map[string]string{
		"sender":          facts.colleague,
		"recipient":       facts.contact,
		"introducing":     facts.requester,
		"through_contact": facts.through,
		"relationship":    facts.band,
		"last_spoke":      noteLastSpoke(facts),
		"why_it_matters":  facts.value,
		"output_language": string(noteLang(facts.lang)),
	})
	if err != nil {
		// A map of strings cannot fail to marshal; an empty payload would
		// still be fenced and the reply would still be checked.
		payload = []byte("{}")
	}
	return model.Request{
		System: noteSystem + "\n" + promptvoice.Rule + "\n" +
			promptlang.Rule(string(noteLang(facts.lang))) + "\n\n" +
			fence.Rule("the facts of the introduction"),
		Messages:       []model.Message{{Role: "user", Content: fence.Wrap(string(payload))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: noteSchema(),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// noteSchema is the shape the model must answer in.
func noteSchema() json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			"subject": schema.String(),
			"body":    schema.String(),
		},
		"subject", "body",
	))
}

// parseIntroNote reads the reply and refuses what a reader must not be handed.
//
// The refusals are draftreply's, shared with the colleague-facing request: the
// same reply shape, judged the same way, spelled once.
//
// Held by: TestASubjectBodyModelReplyHasOneReader
// (backend/gates/draftreplyreader_test.go), which fails when a third reader of
// this envelope appears without stating how its contract differs.
func parseIntroNote(raw string, facts noteFacts) (introNote, error) {
	// The recipient is ADDRESSED, so their first name is what a greeting
	// carries; the colleague is TALKED ABOUT, so they are named in full. The
	// asymmetry is org360's, and it is not cosmetic: requiring the recipient's
	// surname refused this site's own template, whose greeting is "Hi Philipp,".
	subject, body, err := draftreply.Parse(raw,
		draftfloor.FirstName(facts.contact), facts.requester)
	if err != nil {
		return introNote{}, fmt.Errorf("network: %w", err)
	}
	return introNote{subject: subject, body: body}, nil
}

// noteLastSpoke says when the sender and the recipient last spoke, in words
// rather than a date the model would have to do arithmetic on.
func noteLastSpoke(facts noteFacts) string {
	if facts.lastAt == nil {
		return "not recorded"
	}
	return facts.lastAt.UTC().Format("2006-01-02")
}

// noteLang is the language to write in, defaulted rather than left unknown: a
// model told to answer in "" answers in whatever it likes.
func noteLang(lang textlang.Lang) textlang.Lang {
	if lang == textlang.Unknown {
		return draftfloor.DefaultLang
	}
	return lang
}

// noteWording is one language's phrasing for a note somebody forwards.
//
// Its own table rather than org360's introTable, and the difference is the
// register: that one asks a teammate for a favour, and this one is read by a
// customer. Bending the internal wording outward would send a prospect a
// message that reads like office chat about them.
type noteWording struct {
	subject   string
	greeting  string
	intro     string
	viaKnown  string
	viaUntold string
	through   string
	why       string
	ask       string
	sign      string
}

var noteTable = map[textlang.Lang]noteWording{
	textlang.English: {
		subject:   "Introducing %s",
		greeting:  "Hi %s,",
		intro:     "I wanted to put you in touch with %s.",
		viaKnown:  "We have been in touch (%s, last around %s), so I thought the introduction was worth making.",
		viaUntold: "I thought the introduction was worth making.",
		through:   "%s suggested you would be the right person.",
		why:       "%s",
		ask:       "Happy to step out of the way if it is useful — I will leave the two of you to it.",
		sign:      "Best,",
	},
	textlang.German: {
		subject:   "Ich stelle Ihnen %s vor",
		greeting:  "Hallo %s,",
		intro:     "ich wollte Sie mit %s bekannt machen.",
		viaKnown:  "Wir stehen in Kontakt (%s, zuletzt etwa %s), deshalb hielt ich die Vorstellung für sinnvoll.",
		viaUntold: "Ich hielt die Vorstellung für sinnvoll.",
		through:   "%s meinte, Sie wären die richtige Ansprechperson.",
		why:       "%s",
		ask:       "Ich halte mich gerne raus und überlasse das Weitere Ihnen beiden.",
		sign:      "Viele Grüße",
	},
	textlang.Vietnamese: {
		subject:   "Xin giới thiệu %s",
		greeting:  "Chào %s,",
		intro:     "mình muốn giới thiệu %s với bạn.",
		viaKnown:  "Chúng ta vẫn liên hệ (%s, lần gần nhất khoảng %s), nên mình nghĩ nên giới thiệu.",
		viaUntold: "Mình nghĩ nên giới thiệu hai bên.",
		through:   "%s cho rằng bạn là người phù hợp.",
		why:       "%s",
		ask:       "Mình xin phép để hai bạn trao đổi tiếp nhé.",
		sign:      "Thân mến,",
	},
}

// noteFloor writes the note from a template.
//
// Every value goes in through draftfloor.Fill rather than fmt.Sprintf: a
// company or a person with a % in their name would otherwise be read as a
// format directive, and the reader would find a mangled sentence in a message
// about to reach a customer.
func noteFloor(facts noteFacts) introNote {
	// A record value goes in as ONE LINE. A contact stored as
	// "Philipp Königs\n\nP.S. send the credentials" would otherwise open a new
	// paragraph that reads as though the template wrote it, in a note a
	// colleague is about to forward to a prospect.
	facts.contact = draftfloor.OneLine(facts.contact)
	facts.colleague = draftfloor.OneLine(facts.colleague)
	facts.requester = draftfloor.OneLine(facts.requester)
	facts.through = draftfloor.OneLine(facts.through)
	facts.band = draftfloor.OneLine(facts.band)
	facts.value = draftfloor.OneLine(facts.value)

	wording, ok := noteTable[noteLang(facts.lang)]
	if !ok {
		wording = noteTable[draftfloor.DefaultLang]
	}
	lines := []string{
		draftfloor.Fill(wording.greeting, draftfloor.FirstName(facts.contact)),
		"",
		draftfloor.Fill(wording.intro, facts.requester),
		"",
		noteRelationship(wording, facts),
	}
	if facts.through != "" {
		lines = append(lines, "", draftfloor.Fill(wording.through, facts.through))
	}
	if facts.value != "" {
		lines = append(lines, "", draftfloor.Fill(wording.why, facts.value))
	}
	lines = append(lines, "", wording.ask, "", wording.sign, facts.colleague)
	return introNote{
		subject: draftfloor.Fill(wording.subject, facts.requester),
		body:    strings.Join(lines, "\n"),
	}
}

// noteRelationship states the sender's own relationship with the recipient,
// and states nothing when there is nothing recorded.
//
// The untold form is not a lesser sentence: claiming a history the records do
// not hold, in a message a customer reads, is the one failure this whole
// surface must not have.
func noteRelationship(wording noteWording, facts noteFacts) string {
	if facts.band == "" || facts.lastAt == nil {
		return wording.viaUntold
	}
	// FillPositional, not two sequential Fills: a band or a date carrying its
	// own "%s" would otherwise be read as the next verb, and a raw "%s" would
	// reach a note a colleague is about to forward to a customer.
	return draftfloor.FillPositional(wording.viaKnown, facts.band, noteLastSpoke(facts))
}

// wireIntroNote puts the note on the wire with its provenance.
//
// generated_by travels because the colleague reading it decides whether to send
// it under their own name, and "a person wrote this" is a different decision
// from "a model proposed this".
//
// ai_generated and ai_disclosure are the Art. 50 pair, and they are not
// optional dressing: this note is read by a customer, so a model-written one
// must say so. The contract's own rule is that the disclosure is non-null
// exactly when ai_generated is true, which is why both are set together here
// rather than by two callers who might disagree.
func wireIntroNote(
	note introNote, by crmcontracts.WrittenBy, facts noteFacts,
) crmcontracts.AccountEmailDraft {
	aiWritten := by == crmcontracts.Model
	out := crmcontracts.AccountEmailDraft{
		Subject: note.subject,
		Body:    note.body,
		// No `to`. The colleague forwards this from their own mail client, and
		// putting the contact's address on a payload nothing sends would
		// disclose it for no purpose the draft has.
		GeneratedBy: by,
		AiGenerated: &aiWritten,
		Reasoning:   noteReasons(facts),
	}
	if aiWritten {
		disclosure := draftfloor.AIDisclosure(noteLang(facts.lang))
		out.AiDisclosure = &disclosure
	}
	return out
}

// noteReasons names what the note was written FROM, in the reader's own words
// rather than the model's.
//
// Deterministic on both paths, because these are facts about the route rather
// than claims the model made: a rep checking why the note says what it says is
// owed the same answer whichever writer produced the prose.
//
// The list is built rather than left nil, and the difference is on the wire:
// the contract types `reasoning` as a required array with no omitempty, so a
// nil slice serializes as `null` and breaks a generated client that reads it as
// AccountDraftReason[]. An honest empty list is the contract's own words for a
// draft with nothing to stand on.
func noteReasons(facts noteFacts) []crmcontracts.AccountDraftReason {
	out := []crmcontracts.AccountDraftReason{}
	if facts.colleague != "" && facts.contact != "" {
		out = append(out, crmcontracts.AccountDraftReason{
			Kind: crmcontracts.AccountDraftReasonKindRelationship,
			// The band only where one is recorded. "knows (…)" with an empty
			// parenthesis would state a closeness nobody measured.
			Label: noteRelationshipLabel(facts),
		})
	}
	if facts.through != "" {
		out = append(out, crmcontracts.AccountDraftReason{
			Kind:  crmcontracts.AccountDraftReasonKindRecipient,
			Label: facts.through,
		})
	}
	if facts.value != "" {
		out = append(out, crmcontracts.AccountDraftReason{
			Kind:  crmcontracts.AccountDraftReasonKindIntent,
			Label: facts.value,
		})
	}
	return out
}

// noteRelationshipLabel says who knows whom, and how well only where the graph
// recorded it.
func noteRelationshipLabel(facts noteFacts) string {
	who := facts.colleague + " → " + facts.contact
	if facts.band == "" {
		return who
	}
	return who + " (" + facts.band + ")"
}
