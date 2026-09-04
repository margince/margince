// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

// The writer both grounded drafting surfaces run: build the request, ask the
// lane, parse what came back, correct it once, apply the voice floor, and
// degrade to the surface's own deterministic floor when any of that fails.
//
// This used to be two copies. `Write`, `writeChecked`, `writeWithModel`,
// `buildRequest` and `ParseDraft` existed in full in both accountdraft and
// persondraft, and they differed in error-message wording and one word of one
// comment — nothing else. The `Draft` and `Reason` types were byte-identical.
// Two implementations of one capability are two answers to one question, and
// these two were already deciding independently what the fence looks like, how
// a fenced answer is unwrapped, what a starved MAX_TOKENS reply does, and which
// drafts carry the Art. 50 disclosure.
//
// What stays PER SURFACE is what a draft is grounded in, and that is real: the
// prompt, the response schema, the deterministic floor, and which uncited
// reason kinds are honest. Those arrive through Surface below. A package that
// owned them would be a third surface pretending to be a library — the same
// line this package's doc comment already draws for the retry loop.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Completer is the model seam: the draft lane, or nil.
type Completer interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// Draft is what a drafting surface serves.
//
// One type rather than one per surface. The two were declared separately and
// were identical field for field, so they were already one type written twice —
// and a field added to one would silently have been missing from the other's
// contract mapping.
type Draft struct {
	Subject   string
	Body      string
	To        []string
	Reasoning []Reason
}

// Reason is one input the draft actually used, as the composer's "Based on"
// chip renders it.
type Reason struct {
	Kind  crmcontracts.AccountDraftReasonKind
	Label string
	// EntityType and EntityID are both set or both empty: a citation is a pair,
	// and half of one points at nothing.
	EntityType string
	EntityID   string
}

// Input is what a surface's folded record must be able to answer.
//
// An interface rather than a struct, because the two surfaces fold genuinely
// different records — an Organization360 and a Person360 — and a shared struct
// would be one shape with half its fields empty on either side. What the writer
// needs is only this: something to serialize, the envelope it is written into,
// the names a greeting repair recognises, and where it goes.
type Input interface {
	// Fenced is the payload the model sees. The surface returns itself with the
	// caller's intent removed: the intent rides the user turn OUTSIDE the fence,
	// because it is the one input the caller typed rather than untrusted text.
	//craft:ignore naked-any the writer marshals it to JSON and reads nothing off it; each surface returns its own folded record, so a concrete type would name one surface's shape and exclude the other's.
	Fenced() any
	// WrittenInto is the language, the conversation band, the time and the
	// sender. Named for what it answers rather than for the field it usually
	// comes from: both surfaces carry that field as `Envelope`, and a method of
	// the same name cannot sit beside it.
	WrittenInto() draftfloor.Envelope
	// Steering is the caller's own words, empty when they said nothing.
	Steering() string
	// GreetingNames are the first and last name a greeting-line repair looks
	// for. Both, because the register decides which one opens the message.
	GreetingNames() (first, last string)
	// Addresses is where the draft is sent, empty when the record holds none.
	Addresses() []string
	// Threaded says whether this draft continues an existing subject line.
	Threaded() bool
}

// Surface is what one drafting site decides for itself.
type Surface struct {
	// Name is what an error calls this site: "person draft", "account draft".
	// Only ever in an error, so it is not a routing key.
	Name string
	// System is the prompt, assembled for THIS call — the fence's rule and the
	// voice block are both call-dependent, so it is a function rather than text.
	System func(fence promptfence.Fence, voiced bool) string
	// Schema is the response schema, as raw JSON.
	Schema string
	// Kind admits a reason kind this surface can serve, or refuses one the
	// model invented. The two allowlists genuinely differ: only an account
	// draft has a dossier to cite.
	Kind func(raw string) (crmcontracts.AccountDraftReasonKind, bool)
	// KeepUncited decides whether a reason that cited NO record is still
	// honest. A person draft admits only the caller's own intent; an account
	// draft also admits a dossier line, which is a fact about the company with
	// no record of ours behind it.
	KeepUncited func(kind crmcontracts.AccountDraftReasonKind, label string) bool
	// Cites answers what kind of record this id is, as the caller can see it —
	// so a citation can be checked against the type the model claimed. Empty
	// means "no such record here", which drops the chip.
	Cites func(entityID string) string
}

// modelDraft is the answer's wire shape.
type modelDraft struct {
	Subject   string        `json:"subject"`
	Body      string        `json:"body"`
	Reasoning []modelReason `json:"reasoning"`
}

type modelReason struct {
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
}

// Write produces one draft, degrading to the surface's floor rather than
// failing.
//
// `floor` is the surface's deterministic answer, computed by the caller: a
// model that is down, over budget or answering nonsense must not cost the rep
// their draft, and `generated_by` tells the reader which writer produced it.
func Write(
	ctx context.Context,
	lane Completer,
	surface Surface,
	in Input,
	voice draftvoice.Context,
	floor Draft,
) (Draft, crmcontracts.WrittenBy) {
	if lane == nil {
		return floor, crmcontracts.Deterministic
	}
	written, err := writeChecked(ctx, lane, surface, in, voice)
	if err != nil {
		// The error is deliberately swallowed rather than returned — it is a
		// fact about the lane, not about the record, and there is nothing the
		// caller could do with it. The floor is a real message they can edit.
		return floor, crmcontracts.Deterministic
	}
	return written, crmcontracts.Model
}

func writeChecked(
	ctx context.Context, lane Completer, surface Surface, in Input, voice draftvoice.Context,
) (Draft, error) {
	envelope := in.WrittenInto()
	draft, err := CorrectOnce(ctx, envelope.Lang(), envelope.Band(),
		func(ctx context.Context, correction string) (Draft, error) {
			return writeWithModel(ctx, lane, surface, in, voice, correction)
		},
		DraftText, DraftSubject(in),
		// No observer: these packages hold no logger, and a retry that does not
		// help still returns a real draft. The reply surface, which has one,
		// reports it.
		nil,
	)
	if err != nil {
		return Draft{}, err
	}
	return applyVoiceFloor(ctx, lane, surface, in, voice, draft)
}

func writeWithModel(
	ctx context.Context, lane Completer, surface Surface,
	in Input, voice draftvoice.Context, correction string,
) (Draft, error) {
	req, err := BuildRequest(surface, in, voice)
	if err != nil {
		return Draft{}, err
	}
	if correction != "" {
		// The correction rides the user turn, beside the fenced input, so a
		// retry changes what the model is told about its LAST attempt and
		// nothing about the request's shape.
		req.Messages[len(req.Messages)-1].Content += correction
	}
	// ai.Ask re-asks through the SAME parse the answer path runs, so a reply
	// this site would refuse goes back to the model with the reason rather than
	// degrading silently to the floor.
	res, err := ai.Ask(ctx, lane, req, func(text string) error {
		_, parseErr := ParseDraft(surface, text, in)
		return parseErr
	})
	if err != nil {
		return Draft{}, err
	}
	return ParseDraft(surface, res.Text, in)
}

// BuildRequest assembles the model call: the fenced record, the voice block,
// the caller's steering, and this surface's prompt and schema.
func BuildRequest(surface Surface, in Input, voice draftvoice.Context) (model.Request, error) {
	fence := promptfence.New()
	payload, err := json.Marshal(in.Fenced())
	if err != nil {
		return model.Request{}, fmt.Errorf("marshal %s input: %w", surface.Name, err)
	}
	content := fence.Wrap(string(payload))
	if block := voice.Block(fence); block != "" {
		content += "\n\n" + block
	}
	if steering := in.Steering(); steering != "" {
		content += "\n\nThe salesperson asks for: " + steering
	}
	system := surface.System(fence, voice.OK)
	// The shared rules, checked HERE rather than trusted.
	//
	// Each surface composes its own prompt — that part is per-surface by design
	// — and until this writer existed, a gate could read the file that built the
	// request and see draftrules.Shared in it. It cannot any more: the request
	// is built here and the prompt arrives as a closure whose text no scan can
	// follow. So the seam asserts what the scan used to: a system turn reaching
	// a model through this writer carries the language rule and the voice rule,
	// or it does not go.
	//
	// A refusal rather than a silent repair: composing the missing rule here
	// would leave the surface's prompt permanently wrong and this writer quietly
	// patching it, which is the drift the whole change exists to end.
	//
	// The LANGUAGE rule only. The voice block is conditional on purpose — a
	// system turn telling the model to obey a profile that never arrives in the
	// user turn is an instruction pointing at nothing — so an unvoiced call
	// legitimately carries no VOICE heading, and asserting one here refused
	// every draft by a rep who has not built a profile.
	if !strings.Contains(system, promptlang.Heading) {
		return model.Request{}, fmt.Errorf(
			"%s prompt carries no %s rule: its output language would be whatever the input happened to be in",
			surface.Name, strings.TrimSpace(promptlang.Heading))
	}
	//promptvoice:exempt this is an email the salesperson sends under their OWN name, so it carries THEIR voice (draftvoice) rather than Margince's personality — Margince's voice inside a customer-facing draft would be Margince signing somebody else's mail.
	return model.Request{
		System:   system,
		Messages: []model.Message{{Role: "user", Content: content}},
		// Thinking headroom. A reasoning model spends output tokens on internal
		// thinking BEFORE its answer, and that thinking counts against the cap —
		// so a request with no cap takes the provider's default, and on a
		// premium rung the answer is starved into a MAX_TOKENS stop with zero
		// visible text. The reply site has always set this; these two never did,
		// which is why raising the tier failed here and not there.
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: json.RawMessage(surface.Schema),
		SecretStripper: ai.NewSecretStripper(),
	}, nil
}

// ParseDraft reads what the model answered, or refuses it.
func ParseDraft(surface Surface, raw string, in Input) (Draft, error) {
	var out modelDraft
	// ai.Unfence, not the raw text: a model that wraps its JSON in a ```json
	// fence answers correctly and would fail this parse. The reply surface
	// already strips the fence, so without this the SAME model succeeds when it
	// answers a reply and fails when it writes a draft — and ai.Unfence's own
	// doc says callers must not each invent their own trim.
	if err := json.Unmarshal([]byte(ai.Unfence(raw)), &out); err != nil {
		return Draft{}, fmt.Errorf("%s response: %w", surface.Name, err)
	}
	subject := strings.TrimSpace(out.Subject)
	// Plain text, as the contract says a body is. A model asked for prose
	// answers with `<br>` between paragraphs often enough that it is the shape
	// of the answer; the reply surface reads it the same way, so the same model
	// cannot format correctly on one surface and not the other.
	//
	// The greeting break is restored here rather than trusted to the prompt: the
	// same request returns the run-on and the two-line form about equally often,
	// and the composer renders exactly the breaks it is handed. Both names,
	// because the register decides which one opens the message.
	first, last := in.GreetingNames()
	body := strings.TrimSpace(draftfloor.SplitGreetingLine(ai.PlainText(out.Body), first, last))
	if subject == "" || body == "" {
		return Draft{}, fmt.Errorf("%s response: empty subject or body", surface.Name)
	}
	return Draft{
		Subject:   subject,
		Body:      body,
		To:        in.Addresses(),
		Reasoning: keepGroundedReasons(surface, out.Reasoning),
	}, nil
}

// keepGroundedReasons drops every reason that cannot be checked.
//
// A chip the reader can press has to open the record it names. A citation the
// caller cannot see, or one labelled as the wrong kind, opens the WRONG record's
// page rather than nothing at all — which is the worse of the two failures,
// because it looks like it worked.
func keepGroundedReasons(surface Surface, reasons []modelReason) []Reason {
	out := make([]Reason, 0, len(reasons))
	for _, reason := range reasons {
		kind, ok := surface.Kind(reason.Kind)
		label := strings.TrimSpace(reason.Label)
		if !ok || label == "" {
			continue
		}
		keep := Reason{Kind: kind, Label: label}
		if reason.EntityID != "" {
			// The PAIR, not the id alone: an id checked without its type lets a
			// deal id come back labelled as a person, and the chip then opens
			// the wrong record's page rather than nothing at all — the worse of
			// the two failures, because it looks like it worked.
			if surface.Cites(reason.EntityID) != reason.EntityType {
				continue
			}
			keep.EntityType = reason.EntityType
			keep.EntityID = reason.EntityID
		} else if !surface.KeepUncited(kind, label) {
			// A reason with no citation is only honest where nothing was there
			// to cite. An uncited "deal" is a claim about a record with no
			// record behind it — exactly what this filter exists to drop.
			continue
		}
		out = append(out, keep)
	}
	return out
}

// DraftText is the prose a phrasing check judges: the body, and the reason
// labels beside it.
func DraftText(d Draft) (string, []string) { return d.Body, ReasonLabels(d.Reasoning) }

// DraftSubject is the subject a phrasing check judges, and whether it continues
// an existing thread — a threaded subject is not held to the same rules as one
// opening a conversation.
func DraftSubject(in Input) func(Draft) (string, bool) {
	return func(d Draft) (string, bool) { return d.Subject, in.Threaded() }
}

// ReasonLabels is the reasoning as plain strings, for the phrasing checks.
func ReasonLabels(reasons []Reason) []string {
	out := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		out = append(out, reason.Label)
	}
	return out
}
