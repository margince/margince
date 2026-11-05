// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Asking the user a question in the chat, in the middle of a call.
//
// A tool that needs a yes or a pick answers input_required, the 2026-07-28
// multi round-trip pattern, and the host shows the user a form. The answer comes
// back through the client, the same party that made the call. So it can only
// settle what the client could settle by itself: a bulk change's go-ahead, or
// which of several records a name meant. A change a human must approve still
// goes to the approval inbox.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	resultTypeInputRequired = "input_required"
	methodElicitationCreate = "elicitation/create"
	// inputStateTyp separates this token from every other one the same
	// deployment key signs.
	inputStateTyp = "mcp-input"
	// inputStateTTL is how long the user has to answer before the question
	// must be asked again.
	inputStateTTL = 10 * time.Minute
	// minInputStateKeyLen matches the floor the other signers of the
	// deployment key hold.
	minInputStateKeyLen = 32
)

// inputRequiredResult is the answer that asks: the questions and the state the
// retry must carry back.
type inputRequiredResult map[string]any

func (inputRequiredResult) resultType() string { return resultTypeInputRequired }

// chatQuestion is one form a tool puts to the user. Key names the answer in
// the retry's inputResponses.
type chatQuestion struct {
	Key     string
	Message string
	Schema  json.RawMessage
}

// askInChatError is how a tool says it cannot go on without the user's answer.
// The dispatcher turns it into input_required; it never reaches the client as a
// refusal.
type askInChatError struct{ question chatQuestion }

func (e *askInChatError) Error() string {
	return "this call waits for the user's answer: " + e.question.Message
}

// errDeclinedInChat is a tool's refusal when the user said no to its question.
var errDeclinedInChat = errors.New("the user said no in the chat, so nothing was changed")

// chatReply is what the user answered to one question.
type chatReply struct {
	// Available is false when this caller cannot be asked at all. The tool then
	// does what it did before the question existed.
	Available bool
	Accepted  bool
	Content   map[string]json.RawMessage
}

type chatAnswersKey struct{}

// chatAnswers are the answers a verified retry carried, by question key. An
// empty map still means the caller can be asked.
type chatAnswers map[string]json.RawMessage

func withChatAnswers(ctx context.Context, answers chatAnswers) context.Context {
	return context.WithValue(ctx, chatAnswersKey{}, answers)
}

// canAskInChat reports whether this call may put a question to the user.
func canAskInChat(ctx context.Context) bool {
	_, ok := ctx.Value(chatAnswersKey{}).(chatAnswers)
	return ok
}

// replyInChat answers what the user said to q, or asks it. With no answer yet
// it returns an askInChatError the tool hands straight back.
func replyInChat(ctx context.Context, q chatQuestion) (chatReply, error) {
	answers, ok := ctx.Value(chatAnswersKey{}).(chatAnswers)
	if !ok {
		return chatReply{}, nil
	}
	var reply struct {
		Action  string                     `json:"action"`
		Content map[string]json.RawMessage `json:"content"`
	}
	raw, answered := answers[q.Key]
	if !answered || json.Unmarshal(raw, &reply) != nil {
		return chatReply{Available: true}, &askInChatError{question: q}
	}
	switch reply.Action {
	case "accept":
		return chatReply{Available: true, Accepted: true, Content: reply.Content}, nil
	case "decline", "cancel":
		return chatReply{Available: true}, nil
	default:
		return chatReply{Available: true}, &askInChatError{question: q}
	}
}

// declaresFormElicitation reports whether this request's client can show a form.
// An elicitation object with neither mode named is the form mode by the
// specification's default.
func declaresFormElicitation(capabilities json.RawMessage) bool {
	var declared map[string]json.RawMessage
	if err := json.Unmarshal(capabilities, &declared); err != nil {
		return false
	}
	var modes map[string]json.RawMessage
	if err := json.Unmarshal(declared["elicitation"], &modes); err != nil || modes == nil {
		return false
	}
	_, form := modes["form"]
	_, url := modes["url"]
	return form || !url
}

// inputStateSigner binds a question to the caller, the tool and its exact
// arguments, so an answer cannot be carried over to some other call.
type inputStateSigner struct {
	key []byte
	now func() time.Time
}

func newInputStateSigner(key []byte) inputStateSigner {
	if len(key) < minInputStateKeyLen {
		return inputStateSigner{}
	}
	return inputStateSigner{key: key, now: time.Now}
}

func (s inputStateSigner) armed() bool { return s.key != nil }

type wireInputState struct {
	Typ      string `json:"t"`
	Sub      string `json:"sub"`
	Tool     string `json:"tool"`
	Args     string `json:"args"`
	Question string `json:"q"`
	Exp      int64  `json:"exp"`
}

func (s inputStateSigner) sign(state wireInputState) (string, error) {
	state.Typ, state.Exp = inputStateTyp, s.now().Add(inputStateTTL).Unix()
	payload, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return enc + "." + base64.RawURLEncoding.EncodeToString(s.mac(enc)), nil
}

// verify reports whether token was minted for this very call. Any failure
// is the same false: the caller is asked again, which is what the
// specification asks of a retry the server cannot use.
func (s inputStateSigner) verify(token string, want wireInputState) bool {
	enc, macPart, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	gotMAC, err := base64.RawURLEncoding.DecodeString(macPart)
	if err != nil || subtle.ConstantTimeCompare(gotMAC, s.mac(enc)) != 1 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return false
	}
	var got wireInputState
	if err := json.Unmarshal(payload, &got); err != nil {
		return false
	}
	return got.Typ == inputStateTyp && got.Sub == want.Sub && got.Tool == want.Tool &&
		got.Args == want.Args && s.now().Unix() <= got.Exp
}

func (s inputStateSigner) mac(enc string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(enc))
	return m.Sum(nil)
}

// callIdentity is the state a question is bound to before its key is known.
func callIdentity(ctx context.Context, tool string, args json.RawMessage) wireInputState {
	sub := ""
	if actor, ok := principal.Actor(ctx); ok {
		sub = actor.ID + "|" + actor.ConnectionID.String()
	}
	return wireInputState{Sub: sub, Tool: tool, Args: argsDigest(args)}
}

// argsDigest hashes the arguments in one canonical spelling, so a client that
// re-encodes the same call on retry still matches it.
func argsDigest(args json.RawMessage) string {
	decoder := json.NewDecoder(bytes.NewReader(args))
	decoder.UseNumber()
	//craft:ignore naked-any the arguments are any JSON a client sent, decoded only to be spelled again in key order
	var value any
	canonical := []byte(args)
	if decoder.Decode(&value) == nil {
		if again, err := json.Marshal(value); err == nil {
			canonical = again
		}
	}
	sum := sha256.Sum256(canonical)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// chatInputParams are the retry members a tools/call may carry.
type chatInputParams struct {
	//nolint:tagliatelle // both are the protocol's wire members, camelCase by the specification
	InputResponses map[string]json.RawMessage `json:"inputResponses"`
	//nolint:tagliatelle // see InputResponses
	RequestState string `json:"requestState"`
}

// withChatInput readies ctx for questions when this caller can be asked. The
// answers count only under a state this server minted for this very call.
func (s *Dispatcher) withChatInput(ctx context.Context, fr framing, tool string, args, params json.RawMessage) context.Context {
	if !fr.modern || !fr.elicit || !s.inputState.armed() {
		return ctx
	}
	var p chatInputParams
	if err := json.Unmarshal(params, &p); err != nil || p.RequestState == "" {
		return withChatAnswers(ctx, chatAnswers{})
	}
	if !s.inputState.verify(p.RequestState, callIdentity(ctx, tool, args)) {
		return withChatAnswers(ctx, chatAnswers{})
	}
	return withChatAnswers(ctx, p.InputResponses)
}

// askedInChat turns a tool's question into the input_required answer, or
// reports that err asks nothing.
func (s *Dispatcher) askedInChat(ctx context.Context, tool string, args json.RawMessage, err error) (inputRequiredResult, bool) {
	ask, asked := errors.AsType[*askInChatError](err)
	if !asked {
		return nil, false
	}
	state := callIdentity(ctx, tool, args)
	state.Question = ask.question.Key
	token, signErr := s.inputState.sign(state)
	if signErr != nil {
		s.log.Warn("mcp: signing an in-chat question failed", "tool", tool, "err", signErr)
		return nil, false
	}
	return inputRequiredResult{
		"inputRequests": map[string]any{ask.question.Key: map[string]any{
			"method": methodElicitationCreate,
			"params": map[string]any{
				"mode": "form", "message": ask.question.Message, "requestedSchema": ask.question.Schema,
			},
		}},
		"requestState": token,
	}, true
}
