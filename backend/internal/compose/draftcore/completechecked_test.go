// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// plainLane is a lane with no structured-output capability, which is what a
// surface's own test fakes usually are.
type plainLane struct {
	calls int
	text  string
}

func (l *plainLane) Complete(context.Context, model.Request) (model.Response, error) {
	l.calls++
	return model.Response{Text: l.text}, nil
}

// validatedLane is a lane that CAN re-ask. It records the validator it was
// handed rather than acting on it: the retry policy itself belongs to
// ai.Router.CompleteStructured and is tested there, so what a surface owes is
// handing its reading down to it.
type validatedLane struct {
	plainLane
	validatedCalls int
	got            ai.Validator
}

func (l *validatedLane) CompleteValidated(
	_ context.Context, _ model.Request, validate ai.Validator,
) (model.Response, error) {
	l.validatedCalls++
	l.got = validate
	return model.Response{Text: l.text}, nil
}

// The whole point: a lane that can re-ask is asked through the path that does,
// and the reading it is handed is the surface's own. A surface that skips this
// refuses a reply the model was never told was wrong.
func TestALaneThatCanReAskIsAskedThroughIt(t *testing.T) {
	refused := errors.New("empty subject or body")
	read := func(string) error { return refused }
	lane := &validatedLane{plainLane: plainLane{text: "{}"}}

	if _, err := draftcore.CompleteChecked(context.Background(), lane, model.Request{}, read); err != nil {
		t.Fatalf("CompleteChecked errored: %v", err)
	}
	if lane.validatedCalls != 1 {
		t.Errorf("the validated path should have been taken once, got %d", lane.validatedCalls)
	}
	if lane.calls != 0 {
		t.Errorf("the plain path should not have been taken, got %d calls", lane.calls)
	}
	if lane.got == nil {
		t.Fatal("no reading was handed to the lane, so a refused reply buys no re-ask")
	}
	if !errors.Is(lane.got("anything"), refused) {
		t.Error("the lane was handed a reading that is not the surface's own")
	}
}

// A lane without the capability still drafts. Most surface test fakes are this
// shape, and a surface that only worked against the richer lane would be
// untestable without one.
func TestALaneThatCannotReAskStillDrafts(t *testing.T) {
	lane := &plainLane{text: `{"subject":"Angebot","body":"Hallo Marek,"}`}

	resp, err := draftcore.CompleteChecked(context.Background(), lane, model.Request{},
		func(string) error { return nil })
	if err != nil {
		t.Fatalf("CompleteChecked errored on a plain lane: %v", err)
	}
	if lane.calls != 1 {
		t.Errorf("a plain lane should be called once, got %d", lane.calls)
	}
	if resp.Text != lane.text {
		t.Errorf("the reply should be returned unchanged, got %q", resp.Text)
	}
}
