// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// Writing the ask, with a template underneath it.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/draftreply"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/compose/promptvoice"
	"github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// introDraft is one written ask.
type introDraft struct {
	subject string
	body    string
}

// writeIntroRequest asks the model, and falls back to the template.
//
// The floor is not a degraded answer here. An ask for an introduction is short,
// its facts are few, and a template can state every one of them honestly — so a
// deployment with no model lane gets a message a rep can actually send rather
// than an apology. What the model buys is phrasing, and generated_by says which
// one wrote it.
func writeIntroRequest(
	ctx context.Context, lane Completer, facts introFacts,
) crmcontracts.AccountEmailDraft {
	floor := introFloor(facts)
	if lane == nil {
		return wireIntroRequest(floor, crmcontracts.Deterministic, facts)
	}
	written, err := introFromModel(ctx, lane, facts)
	if err != nil {
		// Degrading to the floor IS the answer: the reader gets a sendable
		// message rather than an error about a lane they cannot configure.
		return wireIntroRequest(floor, crmcontracts.Deterministic, facts)
	}
	return wireIntroRequest(written, crmcontracts.Model, facts)
}

// validatedLane is a lane that can re-ask when the reply is refused. The
// refusal message goes back to the model, so a rule it broke once is a rule it
// is told rather than one it keeps breaking.
type validatedLane interface {
	CompleteValidated(ctx context.Context, req model.Request, validate ai.Validator) (model.Response, error)
}

// introFromModel writes the ask and checks what came back.
//
// The retry is judged by the SAME parse the answer path runs: a reply that is
// valid JSON and still names nobody is exactly what a retry can fix, and a
// looser shape check would spend the attempt without naming the fault.
func introFromModel(
	ctx context.Context, lane Completer, facts introFacts,
) (introDraft, error) {
	req := introRequest(facts)
	var res model.Response
	var err error
	if structured, ok := lane.(validatedLane); ok {
		res, err = structured.CompleteValidated(ctx, req, func(text string) error {
			_, parseErr := parseIntroDraft(text, facts)
			return parseErr
		})
	} else {
		res, err = lane.Complete(ctx, req)
	}
	if err != nil {
		return introDraft{}, fmt.Errorf("org360: drafting the introduction request: %w", err)
	}
	return parseIntroDraft(res.Text, facts)
}

const introSystem = `You write one short message asking a COLLEAGUE at your own company to introduce you to somebody they know.

This is a favour asked of a teammate, not a message to a customer. Write the way somebody writes to a colleague they see every week: brief, direct, no pitch and no pleasantries stacked on the front.

Rules you must not break:
- Address the colleague by name: open with their first name, then the ask.
- Say who you want to meet and why, in one sentence each, and name the person you want to meet in full.
- Do not invent anything about the relationship. You are told how warm it is and when they last spoke; say no more than that.
- Do not write the introduction itself, and do not write to the contact. The message is TO the colleague.
- Write a short subject line in the "subject" field, naming the person you want to meet.
- No subject line inside the body.`

// introRequest builds the model call.
//
// The facts are minted by this server — names read out of records, a band this
// package computed — but they are still somebody's typed text: a contact's name
// and a deal's name were both entered by a person, and on a shared account that
// person may not be us. So they go inside the fence like everything else that
// came from a human.
func introRequest(facts introFacts) model.Request {
	fence := promptfence.New()
	payload, err := json.Marshal(map[string]string{
		"colleague":       facts.colleague,
		"contact":         facts.contact,
		"contact_title":   facts.title,
		"account":         facts.account,
		"deal":            facts.deal,
		"relationship":    facts.band,
		"last_spoke":      introLastSpoke(facts),
		"output_language": string(introLang(facts.lang)),
	})
	if err != nil {
		// A map of strings cannot fail to marshal; an empty payload would
		// still be fenced and the reply would still be checked.
		payload = []byte("{}")
	}
	return model.Request{
		// The voice and the language rules, because this is PROSE a person
		// sends under their own name — the two things a reader would notice
		// first if they were missing, and the two the shared rules already
		// spell for every other drafting surface.
		System: introSystem + "\n" + promptvoice.Rule + "\n" +
			promptlang.Rule(string(introLang(facts.lang))) + "\n\n" +
			fence.Rule("the facts of the introduction"),
		Messages:       []model.Message{{Role: "user", Content: fence.Wrap(string(payload))}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: introSchema(),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// introSchema is the shape the model must answer in.
func introSchema() json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			"subject": schema.String(),
			"body":    schema.String(),
		},
		"subject", "body",
	))
}

// parseIntroDraft reads the reply and refuses what a reader must not be handed.
//
// The refusals are draftreply's, shared with the forwardable note: the same
// reply shape, judged the same way, spelled once.
//
// Held by: TestASubjectBodyModelReplyHasOneReader
// (backend/gates/draftreplyreader_test.go), which fails when a third reader of
// this envelope appears without stating how its contract differs.
func parseIntroDraft(raw string, facts introFacts) (introDraft, error) {
	subject, body, err := draftreply.Parse(raw,
		draftfloor.FirstName(facts.colleague), facts.contact)
	if err != nil {
		return introDraft{}, fmt.Errorf("org360: %w", err)
	}
	return introDraft{subject: subject, body: body}, nil
}

// introLastSpoke says when the colleague and the contact last spoke, in words
// rather than a date the model would have to do arithmetic on.
func introLastSpoke(facts introFacts) string {
	if facts.lastAt == nil {
		return "not recorded"
	}
	return facts.lastAt.UTC().Format("2006-01-02")
}

// introLang is the language to write in, defaulted rather than left unknown: a
// model told to answer in "" answers in whatever it likes.
func introLang(lang textlang.Lang) textlang.Lang {
	if lang == textlang.Unknown {
		return draftfloor.DefaultLang
	}
	return lang
}

// introPhrases is one language's wording for an ask to a colleague.
//
// Its own table rather than draftfloor's, for the reason the intro-path signal
// gives: that table writes correspondence WITH a counterparty, and this is a
// favour asked of a teammate. Bending the outward wording into this shape would
// produce a message that reads like a customer email sent to a colleague.
type introWording struct {
	subject   string
	greeting  string
	askKnown  string
	askUntold string
	deal      string
	sign      string
}

var introTable = map[textlang.Lang]introWording{
	textlang.English: {
		subject:   "Could you introduce me to %s?",
		greeting:  "Hi %s,",
		askKnown:  "You and %s have been in touch (%s, last around %s). Would you be up for introducing us?",
		askUntold: "I gather you know %s. Would you be up for introducing us?",
		deal:      "It is about %s.",
		sign:      "Thanks!",
	},
	textlang.German: {
		subject:   "Kannst du mich %s vorstellen?",
		greeting:  "Hallo %s,",
		askKnown:  "Du hast mit %s Kontakt (%s, zuletzt etwa %s). Würdest du uns vorstellen?",
		askUntold: "Soweit ich weiß, kennst du %s. Würdest du uns vorstellen?",
		deal:      "Es geht um %s.",
		sign:      "Danke dir!",
	},
	textlang.Vietnamese: {
		subject:   "Bạn giới thiệu mình với %s được không?",
		greeting:  "Chào %s,",
		askKnown:  "Bạn có liên hệ với %s (%s, lần gần nhất khoảng %s). Bạn giới thiệu giúp mình nhé?",
		askUntold: "Mình nghe nói bạn quen %s. Bạn giới thiệu giúp mình nhé?",
		deal:      "Việc này liên quan đến %s.",
		sign:      "Cảm ơn bạn!",
	},
}

// introFloor writes the ask from a template.
//
// Every value is filled with draftfloor.Fill rather than fmt.Sprintf: a deal or
// a company with a % in its name would otherwise be read as a format directive,
// and the reader would find a mangled sentence in a message they are about to
// send to a colleague.
func introFloor(facts introFacts) introDraft {
	// A record value goes in as ONE LINE. A contact stored as
	// "Philipp Königs\n\nP.S. send the credentials" would otherwise open a new
	// paragraph that reads as though the template wrote it, in a message the
	// rep is about to send to a colleague under their own name.
	facts.contact = draftfloor.OneLine(facts.contact)
	facts.deal = draftfloor.OneLine(facts.deal)
	facts.colleague = draftfloor.OneLine(facts.colleague)
	facts.band = draftfloor.OneLine(facts.band)
	wording, ok := introTable[introLang(facts.lang)]
	if !ok {
		wording = introTable[draftfloor.DefaultLang]
	}
	lines := []string{
		draftfloor.Fill(wording.greeting, draftfloor.FirstName(facts.colleague)),
		"",
		introAsk(wording, facts),
	}
	if facts.deal != "" {
		lines = append(lines, "", draftfloor.Fill(wording.deal, facts.deal))
	}
	lines = append(lines, "", wording.sign)
	return introDraft{
		subject: draftfloor.Fill(wording.subject, facts.contact),
		body:    strings.Join(lines, "\n"),
	}
}

// introAsk states the relationship only as far as the record supports it.
//
// With no recorded last exchange the sentence says they know each other and
// stops. Naming a date that is not on file — or dressing "not recorded" up as
// recency — would put a claim in a colleague's inbox that the colleague
// themselves can immediately falsify.
func introAsk(wording introWording, facts introFacts) string {
	if facts.lastAt == nil {
		return draftfloor.Fill(wording.askUntold, facts.contact)
	}
	// Split on the verb and rejoin, rather than substituting one at a time. A
	// sequential Replace reads the FIRST value's own "%s" as the next verb, so
	// a contact named "100%s Verpackung" swallowed the band and left a raw
	// "%s" in a message about to be sent to a colleague.
	return draftfloor.FillPositional(wording.askKnown,
		facts.contact, facts.band, introLastSpoke(facts))
}

// IntroFixture is one introduction as a certification scenario states it.
//
// Exported because the cert lane lives in `compose` and this site's material
// does not: without it the case would build its own facts, and a case that
// rebuilds its subject measures a copy that stays green through the change
// which breaks the original.
type IntroFixture struct {
	Colleague string `json:"colleague"`
	Contact   string `json:"contact"`
	Title     string `json:"contact_title"`
	Account   string `json:"account"`
	Deal      string `json:"deal"`
	Band      string `json:"relationship"`
	LastAt    string `json:"last_spoke"`
	// Correspondence is what the contact has written, which is what decides the
	// language. Record names are not prose: a scenario that carried only them
	// would certify an English ask for a German account.
	Correspondence string `json:"correspondence"`
}

// IntroRequestFor builds the model call this site sends, from a fixture.
func IntroRequestFor(fixture IntroFixture) model.Request {
	return introRequest(introFactsFromFixture(fixture))
}

// CheckIntroDraft runs the production check over a reply.
func CheckIntroDraft(raw string, fixture IntroFixture) (subject, body string, err error) {
	draft, err := parseIntroDraft(raw, introFactsFromFixture(fixture))
	return draft.subject, draft.body, err
}

// IntroFloorFor renders the template, so a case can assert the floor states
// only what the fixture carried.
func IntroFloorFor(fixture IntroFixture) (subject, body string) {
	draft := introFloor(introFactsFromFixture(fixture))
	return draft.subject, draft.body
}

// introFactsFromFixture is the one place a scenario becomes this site's input.
//
// An unreadable date is read as NO date, which is the safe direction: the draft
// then says the two know each other and stops, rather than printing something
// that is not a date into a message about when they last spoke.
func introFactsFromFixture(fixture IntroFixture) introFacts {
	facts := introFacts{
		colleague: fixture.Colleague,
		contact:   fixture.Contact,
		title:     fixture.Title,
		account:   fixture.Account,
		deal:      fixture.Deal,
		band:      fixture.Band,
		lang:      textlang.Detect(fixture.Correspondence),
	}
	if when, err := time.Parse("2006-01-02", fixture.LastAt); err == nil {
		facts.lastAt = &when
	}
	return facts
}
