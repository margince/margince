// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// plainLane answers once and cannot re-ask, which is the offline fake and every
// test double.
type plainLane struct {
	calls int
	reply string
}

func (l *plainLane) Complete(context.Context, model.Request) (model.Response, error) {
	l.calls++
	return model.Response{Text: l.reply}, nil
}

// retryingLane is the structured lane: it hands the validator its first answer
// and, when that is refused, answers again — which is what the real pipeline
// does with the refusal message in hand.
type retryingLane struct {
	plainLane
	validated int
	refused   error
	second    string
}

func (l *retryingLane) CompleteValidated(
	_ context.Context, _ model.Request, validate Validator,
) (model.Response, error) {
	l.validated++
	l.refused = validate(l.reply)
	if l.refused == nil {
		return model.Response{Text: l.reply}, nil
	}
	// A refusal is not this lane's failure to report: the real pipeline hands
	// the message back to the model and asks again, and this double answers
	// with what a corrected model would send.
	return model.Response{Text: l.second}, nil
}

const (
	refusable = `{"subject":"Intro?","body":""}`
	sendable  = `{"subject":"Intro?","body":"Could you introduce me to them?"}`
)

// readsIt stands for a calling site's own read of its reply. It is spelled here
// rather than borrowed from a real site because ai may not import compose,
// where every real one lives — and because what this file proves is the
// DISPATCH, which is the same whichever read a site brings.
func readsIt(text string) error {
	var reply struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal([]byte(text), &reply); err != nil {
		return fmt.Errorf("the reply is not the shape this site takes: %w", err)
	}
	if reply.Body == "" {
		return errors.New("the reply carries no body")
	}
	return nil
}

// A lane that can re-ask is asked through the site's OWN read, so the refusal
// the model is shown is the one the answer path would have raised.
func TestALaneThatCanReAskIsGivenTheSitesOwnRefusal(t *testing.T) {
	t.Parallel()
	lane := &retryingLane{plainLane: plainLane{reply: refusable}, second: sendable}
	res, err := Ask(context.Background(), lane, model.Request{}, readsIt)
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if lane.validated != 1 {
		t.Errorf("the validated lane was used %d times, want once", lane.validated)
	}
	if lane.calls != 0 {
		t.Errorf("the plain path was taken %d times on a lane that can re-ask", lane.calls)
	}
	if lane.refused == nil {
		t.Fatal("the validator never refused the first answer, so nothing was re-asked")
	}
	if res.Text != sendable {
		t.Errorf("the second answer did not come back: %q", res.Text)
	}
}

// A lane that cannot re-ask still answers. Its absence is an ordinary
// configuration, not an error: the reader gets the template rather than a
// failure about a lane they cannot configure.
func TestALaneThatCannotReAskStillAnswers(t *testing.T) {
	t.Parallel()
	lane := &plainLane{reply: sendable}
	res, err := Ask(context.Background(), lane, model.Request{}, readsIt)
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if lane.calls != 1 {
		t.Errorf("the plain lane was called %d times, want once", lane.calls)
	}
	if res.Text != sendable {
		t.Errorf("the answer did not come back: %q", res.Text)
	}
}

// failingLane is a lane that cannot answer at all.
type failingLane struct{ err error }

func (l failingLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{}, l.err
}

// The lane's own failure reaches the caller, which is what lets the site
// degrade to its template rather than report a reply it never got.
func TestALaneFailureReachesTheCaller(t *testing.T) {
	t.Parallel()
	want := errors.New("every bound tier failed")
	if _, err := Ask(
		context.Background(), failingLane{err: want}, model.Request{}, readsIt,
	); !errors.Is(err, want) {
		t.Fatalf("the lane's failure did not reach the caller: %v", err)
	}
}
