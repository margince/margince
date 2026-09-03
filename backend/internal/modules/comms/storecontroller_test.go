// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// What controller staging refuses before it touches the database.
//
// Each refusal is a property of the lane rather than a precaution: the
// installation's own words are fixed in code, and a message that was supposed
// to carry a one-time link and does not is a dead end for whoever receives it.

import (
	"errors"
	"testing"
)

// knownTemplates admits exactly one pair, so a test can prove both directions.
type knownTemplates struct{}

func (knownTemplates) Registered(key string, version int) bool {
	return key == "record_confirmation" && version == 1
}

func validControllerInput() StageControllerInput {
	return StageControllerInput{
		Recipient:       "subject@example.test",
		TemplateKey:     "record_confirmation",
		TemplateVersion: 1,
		MessageID:       "<ctrl-1@margince.test>",
		Subject:         "Your details",
		Body:            "Open " + LinkPlaceholder + " to check what we hold.",
		PayloadRef:      "mgv.1.ws.tok",
	}
}

func TestControllerStagingRefusesAnUnregisteredTemplate(t *testing.T) {
	in := validControllerInput()
	in.TemplateKey = "something_a_caller_invented"

	_, err := (&Store{}).StageControllerTx(t.Context(), nil, in, knownTemplates{})
	if !errors.Is(err, ErrTemplateNotRegistered) {
		t.Errorf("err = %v, want ErrTemplateNotRegistered — the installation's words are not composed at a call site", err)
	}
}

// A version bump is a different template. Wording that changed without one is
// the case the registry exists to make impossible.
func TestControllerStagingRefusesAnUnregisteredVersion(t *testing.T) {
	in := validControllerInput()
	in.TemplateVersion = 2

	_, err := (&Store{}).StageControllerTx(t.Context(), nil, in, knownTemplates{})
	if !errors.Is(err, ErrTemplateNotRegistered) {
		t.Errorf("err = %v, want ErrTemplateNotRegistered for an unknown version", err)
	}
}

// No registry wired is a deployment defect, and it fails closed: a nil
// registry that admitted everything would let any wording ship.
func TestControllerStagingRefusesWithNoRegistry(t *testing.T) {
	_, err := (&Store{}).StageControllerTx(t.Context(), nil, validControllerInput(), nil)
	if !errors.Is(err, ErrTemplateNotRegistered) {
		t.Errorf("err = %v, want a refusal when no template registry is wired", err)
	}
}

// The body and the material must tell the same story, in both directions.
func TestControllerStagingRefusesAPlaceholderMismatch(t *testing.T) {
	cases := map[string]struct {
		body    string
		payload string
	}{
		"material but no placeholder": {body: "Nothing to click here.", payload: "mgv.1.ws.tok"},
		"material and two placeholders": {
			body:    LinkPlaceholder + " and again " + LinkPlaceholder,
			payload: "mgv.1.ws.tok",
		},
		"placeholder but no material": {body: "Open " + LinkPlaceholder, payload: ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			in := validControllerInput()
			in.Body, in.PayloadRef = tc.body, tc.payload

			_, err := (&Store{}).StageControllerTx(t.Context(), nil, in, knownTemplates{})
			if !errors.Is(err, ErrTemplateShape) {
				t.Errorf("err = %v, want ErrTemplateShape", err)
			}
		})
	}
}

// A message with no recipient is refused before anything else, so the refusal
// names the missing addressee rather than the template.
func TestControllerStagingRefusesNoRecipient(t *testing.T) {
	in := validControllerInput()
	in.Recipient = ""

	_, err := (&Store{}).StageControllerTx(t.Context(), nil, in, knownTemplates{})
	if !errors.Is(err, ErrNoAddressee) {
		t.Errorf("err = %v, want ErrNoAddressee", err)
	}
}

// A link-less template is legitimate — an opt-out acknowledgement carries no
// capability — and must pass the shape check rather than being refused for
// having no placeholder.
func TestALinklessTemplatePassesTheShapeCheck(t *testing.T) {
	in := validControllerInput()
	in.Body, in.PayloadRef = "We have recorded that you asked us to stop.", ""

	if err := checkPlaceholder(in); err != nil {
		t.Errorf("checkPlaceholder = %v, want nil for a template that carries no link", err)
	}
}

// An empty message id is non-NULL, so it satisfies the row's shape CHECK and
// then collides on the uniqueness index the moment a SECOND controller message
// is staged. Named as the caller defect it is, rather than surfacing as a
// duplicate nobody sent.
func TestControllerStagingRefusesAnEmptyMessageID(t *testing.T) {
	in := validControllerInput()
	in.MessageID = ""

	_, err := (&Store{}).StageControllerTx(t.Context(), nil, in, knownTemplates{})
	if !errors.Is(err, ErrTemplateShape) {
		t.Errorf("err = %v, want a refusal naming the missing message id", err)
	}
}
