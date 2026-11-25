// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// askingFraming is a modern request from a client that can show a form.
var askingFraming = framing{modern: true, elicit: true}

const testStateKey = "0123456789abcdef0123456789abcdef"

// countingChanger counts the bulk changes that ran.
type countingChanger struct {
	recordingChanger
	executed int
}

func (c *countingChanger) ExecuteBulkChange(ctx context.Context, cmd BulkChangeCommand) (json.RawMessage, error) {
	c.executed++
	return c.recordingChanger.ExecuteBulkChange(ctx, cmd)
}

func bulkDispatcher(t *testing.T, changer BulkChanger) *Dispatcher {
	t.Helper()
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	RegisterBulkTool(registry, changer)
	d := NewDispatcher(registry, bindAuthenticated, "margince-crm", "test").WithLogger(discardLog())
	WithInputStateKey(testStateKey)(d)
	WithReplayGuard(&memGuard{})(d)
	return d
}

func bulkCall(extra string) json.RawMessage {
	return json.RawMessage(`{"name":"bulk_update_records","arguments":{"mode":"execute",` + changeArgs + `}` + extra + `}`)
}

// askedFor reads the question an input_required answer puts, and its state.
//
//craft:ignore naked-any it reads what Dispatcher.call answers, which is any result shape
func askedFor(t *testing.T, got any) (map[string]any, string) {
	t.Helper()
	asked, ok := got.(inputRequiredResult)
	if !ok {
		t.Fatalf("answered %v, want input_required", got)
	}
	requests, ok := asked["inputRequests"].(map[string]any)
	if !ok {
		t.Fatalf("input_required carries no inputRequests: %v", asked)
	}
	state, ok := asked["requestState"].(string)
	if !ok || state == "" {
		t.Fatalf("input_required carries no requestState: %v", asked)
	}
	return requests, state
}

func answering(state, response string) string {
	return `,"requestState":"` + state + `","inputResponses":{"` + bulkConfirmKey + `":` + response + `}`
}

func writer() context.Context { return scopedAgentCtx(principal.ScopeRead, principal.ScopeWrite) }

func TestABulkChangeWaitsForTheUsersYesInTheChat(t *testing.T) {
	changer := &countingChanger{}
	d := bulkDispatcher(t, changer)

	requests, state := askedFor(t, d.call(writer(), bulkCall(""), askingFraming))
	if changer.executed != 0 {
		t.Fatal("the change ran before the user answered")
	}
	form, ok := requests[bulkConfirmKey].(map[string]any)
	if !ok || form["method"] != methodElicitationCreate {
		t.Fatalf("the question is %v, want an elicitation form", requests)
	}
	params, ok := form["params"].(map[string]any)
	message, isText := params["message"].(string)
	if !ok || !isText || !strings.HasPrefix(message, "Archive 1 contact?") {
		t.Errorf("the form says %v", form["params"])
	}

	d.call(writer(), bulkCall(answering(state, `{"action":"accept","content":{"confirm":true}}`)), askingFraming)
	if changer.executed != 1 {
		t.Errorf("a yes ran the change %d times, want once", changer.executed)
	}
}

func TestANoInTheChatChangesNothing(t *testing.T) {
	for name, response := range map[string]string{
		"declined":          `{"action":"decline"}`,
		"accepted unticked": `{"action":"accept","content":{"confirm":false}}`,
	} {
		changer := &countingChanger{}
		d := bulkDispatcher(t, changer)
		_, state := askedFor(t, d.call(writer(), bulkCall(""), askingFraming))
		got := d.call(writer(), bulkCall(answering(state, response)), askingFraming)
		refusal, ok := got.(map[string]any)
		if changer.executed != 0 || !ok || refusal["isError"] != true {
			t.Errorf("%s: answered %v and ran %d changes, want a refusal and none", name, got, changer.executed)
		}
	}
}

func TestAnAnswerCountsOnlyForTheCallItWasAskedFor(t *testing.T) {
	changer := &countingChanger{}
	d := bulkDispatcher(t, changer)
	_, state := askedFor(t, d.call(writer(), bulkCall(""), askingFraming))
	yes := `{"action":"accept","content":{"confirm":true}}`

	other := json.RawMessage(strings.Replace(string(bulkCall(answering(state, yes))), `"archive"`, `"remove_tag"`, 1))
	askedFor(t, d.call(writer(), other, askingFraming))
	askedFor(t, d.call(writer(), bulkCall(answering("forged."+state, yes)), askingFraming))
	if changer.executed != 0 {
		t.Errorf("an answer for another call ran %d changes", changer.executed)
	}
}

func TestAClientThatCannotShowAFormIsNotAsked(t *testing.T) {
	for name, fr := range map[string]framing{
		"handshake era":   legacyFraming,
		"modern, no form": {modern: true},
	} {
		changer := &countingChanger{}
		d := bulkDispatcher(t, changer)
		if got := d.call(writer(), bulkCall(""), fr); changer.executed != 1 {
			t.Errorf("%s: answered %v, want the change run as before", name, got)
		}
	}
	unarmed := &countingChanger{}
	d := bulkDispatcher(t, unarmed)
	d.inputState = inputStateSigner{}
	if d.call(writer(), bulkCall(""), askingFraming); unarmed.executed != 1 {
		t.Error("a server with no signing key asked a question it could not verify the answer to")
	}
}

func TestOnlyAFormCapableElicitationDeclarationCounts(t *testing.T) {
	for capabilities, want := range map[string]bool{
		`{"elicitation":{}}`:                   true,
		`{"elicitation":{"form":{}}}`:          true,
		`{"elicitation":{"form":{},"url":{}}}`: true,
		`{"elicitation":{"url":{}}}`:           false,
		`{"elicitation":null}`:                 false,
		`{}`:                                   false,
	} {
		if got := declaresFormElicitation(json.RawMessage(capabilities)); got != want {
			t.Errorf("%s → %v, want %v", capabilities, got, want)
		}
	}
}

const yesInChat = `{"action":"accept","content":{"confirm":true}}`

// One yes is one change: the same state sent again is asked again and runs
// nothing more.
func TestAnAcceptedAnswerIsGoodForOneRetry(t *testing.T) {
	changer := &countingChanger{}
	d := bulkDispatcher(t, changer)
	_, state := askedFor(t, d.call(writer(), bulkCall(""), askingFraming))
	d.call(writer(), bulkCall(answering(state, yesInChat)), askingFraming)
	askedFor(t, d.call(writer(), bulkCall(answering(state, yesInChat)), askingFraming))
	if changer.executed != 1 {
		t.Errorf("one yes ran the change %d times, want once", changer.executed)
	}
}

// resigned is state with its claims changed and sealed again by the same key,
// as only the server could.
func resigned(t *testing.T, d *Dispatcher, state string, change func(*wireInputState)) string {
	t.Helper()
	payload, ok := d.inputState.open(state)
	if !ok {
		t.Fatal("the server cannot open its own state")
	}
	var claims wireInputState
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	change(&claims)
	again, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return d.inputState.seal(again)
}

// A state names the question it was asked for. Under a state for another
// question of the same call, the answer does not count.
func TestAStateForAnotherQuestionDoesNotAnswerThisOne(t *testing.T) {
	changer := &countingChanger{}
	d := bulkDispatcher(t, changer)
	_, state := askedFor(t, d.call(writer(), bulkCall(""), askingFraming))
	other := resigned(t, d, state, func(claims *wireInputState) { claims.Question = "pick_the_record" })
	askedFor(t, d.call(writer(), bulkCall(answering(other, yesInChat)), askingFraming))
	if changer.executed != 0 {
		t.Errorf("an answer under another question's state ran %d changes", changer.executed)
	}
}

// A state past its window, or one with no id to spend, is asked again.
func TestAnExpiredOrUnspendableStateIsAskedAgain(t *testing.T) {
	signedAt := bulkDispatcher(t, &countingChanger{}).inputState.now()
	for name, change := range map[string]func(*wireInputState){
		"expired":     func(claims *wireInputState) { claims.Exp = signedAt.Add(-time.Minute).Unix() },
		"with no id":  func(claims *wireInputState) { claims.Once = "" },
		"no question": func(claims *wireInputState) { claims.Question = "" },
	} {
		changer := &countingChanger{}
		d := bulkDispatcher(t, changer)
		_, state := askedFor(t, d.call(writer(), bulkCall(""), askingFraming))
		askedFor(t, d.call(writer(), bulkCall(answering(resigned(t, d, state, change), yesInChat)), askingFraming))
		if changer.executed != 0 {
			t.Errorf("%s: the state ran %d changes, want it asked again", name, changer.executed)
		}
	}
}

// A replay, once refused, leaves the user free to answer a fresh question.
func TestAFreshQuestionAfterARefusedReplayStillWorks(t *testing.T) {
	changer := &countingChanger{}
	d := bulkDispatcher(t, changer)
	_, first := askedFor(t, d.call(writer(), bulkCall(""), askingFraming))
	d.call(writer(), bulkCall(answering(first, yesInChat)), askingFraming)
	_, fresh := askedFor(t, d.call(writer(), bulkCall(answering(first, yesInChat)), askingFraming))
	d.call(writer(), bulkCall(answering(fresh, yesInChat)), askingFraming)
	if changer.executed != 2 {
		t.Errorf("two yeses to two questions ran %d changes, want two", changer.executed)
	}
}

// With nothing to spend an answer once, nothing is asked: the call does what it
// did before the question existed. A guard that cannot answer asks again.
func TestWithoutAWorkingGuardNoAnswerCounts(t *testing.T) {
	d := bulkDispatcher(t, &countingChanger{})
	d.replayGuard = nil
	if canAskInChat(d.withChatInput(writer(), askingFraming, "bulk_update_records", json.RawMessage(`{}`), nil)) {
		t.Error("a dispatcher with no guard offered to ask in the chat")
	}

	changer := &countingChanger{}
	broken := bulkDispatcher(t, changer)
	_, state := askedFor(t, broken.call(writer(), bulkCall(""), askingFraming))
	broken.replayGuard = &memGuard{err: errors.New("the replay store is down")}
	askedFor(t, broken.call(writer(), bulkCall(answering(state, yesInChat)), askingFraming))
	if changer.executed != 0 {
		t.Errorf("an answer the guard could not spend ran %d changes", changer.executed)
	}
}
