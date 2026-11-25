// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Confirming a waiting call in the conversation that asked for it.
//
// Some calls wait for their own user (ToolSpec.WaitsEvenForItsUser). When one
// arrives at the MCP door with that user present, it skips the approval inbox.
// Round 1 changes nothing and answers a preview and a one-time confirm_code.
// The same call with the code runs once. Tool calls and text carry both rounds.
//
// The code is a signed claim, not a stored row. It names the caller, the tool,
// the argument digest and the previewed version, and it expires. The replay
// guard makes it single-use on every replica. Only the Dispatcher arms this
// path, so the REST door and unattended runs still stage into the inbox.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

const (
	// confirmCodeTyp separates this token from every other one the deployment
	// key signs.
	confirmCodeTyp = "mcp-confirm"
	// confirmCodeTTL is how long a user has between seeing a preview and saying
	// yes. It matches the bulk change's window (compose/bulkconfirm.go).
	confirmCodeTTL = 15 * time.Minute
	// confirmGuardMargin covers a replica whose clock runs behind the minting one.
	// The spent id is then still held while any replica accepts the code.
	confirmGuardMargin = 5 * time.Minute
	// confirmOnceBytes is the entropy of the id the replay guard spends.
	confirmOnceBytes = 16
)

// errConfirmCodeRefused answers a code that is used, expired, foreign or
// mismatched, without saying which: the remedy is the same.
var errConfirmCodeRefused = fmt.Errorf("this confirm_code does not confirm this call: it is used, expired, "+
	"or was answered for a different call or user. Call again without it for a new preview: %w",
	apperrors.ErrApprovalTokenInvalid)

// errConfirmCodeNotHere answers a code presented where no conversation can
// confirm: the REST door, a scheduled run, or a call that does not wait.
var errConfirmCodeNotHere = fmt.Errorf("confirm_code confirms a call its user previewed in this MCP "+
	"conversation; this call cannot be confirmed here: %w", apperrors.ErrApprovalTokenInvalid)

// errConfirmCodeNoLonger answers a code for a call its caller can no longer
// release itself, such as a filing under a project held since the preview.
var errConfirmCodeNoLonger = fmt.Errorf("this call can no longer be confirmed in the conversation. "+
	"Call again without confirm_code, and it waits in the approval inbox for a colleague to decide: %w",
	apperrors.ErrApprovalTokenInvalid)

// ConfirmGuard spends one-use ids. platform/replayguard is the production one.
type ConfirmGuard interface {
	FirstUse(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

// conversationConfirm is what the Dispatcher arms a call with: the key that
// signs codes and the guard that spends them.
type conversationConfirm struct {
	signer inputStateSigner
	guard  ConfirmGuard
	now    func() time.Time
}

type conversationConfirmKey struct{}

// withConversationConfirm arms ctx so a waiting call may be confirmed in the
// conversation. Set only by the Dispatcher, and only with both halves present.
func withConversationConfirm(ctx context.Context, c conversationConfirm) context.Context {
	if !c.signer.armed() || c.guard == nil {
		return ctx
	}
	return context.WithValue(ctx, conversationConfirmKey{}, c)
}

func conversationConfirmFrom(ctx context.Context) (conversationConfirm, bool) {
	c, ok := ctx.Value(conversationConfirmKey{}).(conversationConfirm)
	return c, ok
}

// confirmsInConversation reports whether its own user can confirm this refused
// call here. The MCP door armed it, the decision is that user's, and someone is
// present to ask.
func confirmsInConversation(ctx context.Context, waitsEvenForItsUser bool, refusal error) bool {
	if !waitsEvenForItsUser || !errors.Is(refusal, apperrors.ErrRequiresApproval) {
		return false
	}
	if _, armed := conversationConfirmFrom(ctx); !armed {
		return false
	}
	p, ok := principal.Actor(ctx)
	return ok && !principal.ConversationUser(ctx, p).IsZero()
}

// confirmedCall is the call a code confirms. It names the tool, its argument
// digest, the record it acts on and the pins a staging of it would take.
type confirmedCall struct {
	Tool, Args string
	TargetType string
	TargetID   ids.UUID
	Pins       auth.ReleasedRecordPins
	// Once is the code's one-use id, which the audit rows of its run name.
	Once string
}

// wireConfirmCode is what a code carries. The model copies it back by hand, so
// the keys are short. The caller and the call travel as one digest under the
// signature, and the spend recomputes it.
type wireConfirmCode struct {
	Typ        string   `json:"t"`
	Bind       string   `json:"b"`
	TargetType string   `json:"tt,omitempty"`
	TargetID   ids.UUID `json:"ti,omitzero"`
	Target     *wirePin `json:"p,omitempty"`
	CoTarget   *wirePin `json:"c,omitempty"`
	Once       string   `json:"n"`
	Exp        int64    `json:"e"`
}

// wirePin is one pin. The target's own type and id are the code's, so its pin
// carries only the version and context.
type wirePin struct {
	Type    string   `json:"t,omitempty"`
	ID      ids.UUID `json:"i,omitzero"`
	Version int64    `json:"v"`
	Context string   `json:"x,omitempty"`
}

func pinOnWire(pin *auth.ReleasedRecordPin, own bool) *wirePin {
	if pin == nil {
		return nil
	}
	out := &wirePin{Version: pin.Version, Context: pin.ContextDigest}
	if !own {
		out.Type, out.ID = pin.EntityType, pin.EntityID
	}
	return out
}

func (w *wirePin) released(entityType string, id ids.UUID) *auth.ReleasedRecordPin {
	if w == nil {
		return nil
	}
	if w.Type != "" {
		entityType, id = w.Type, w.ID
	}
	return &auth.ReleasedRecordPin{EntityType: entityType, EntityID: id, Version: w.Version, ContextDigest: w.Context}
}

// call is the confirmed call a verified code names.
func (w wireConfirmCode) call(tool, args string) confirmedCall {
	return confirmedCall{Tool: tool, Args: args, TargetType: w.TargetType, TargetID: w.TargetID, Pins: auth.ReleasedRecordPins{
		Target: w.Target.released(w.TargetType, w.TargetID), CoTarget: w.CoTarget.released("", ids.UUID{}),
	}}
}

// codeBinding digests who a code is for and the call it confirms: every part a
// code may not travel between. It answers false with no caller.
func codeBinding(ctx context.Context, tool, args string) (string, bool) {
	p, ok := principal.Actor(ctx)
	if !ok || p.ID == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		confirmCodeTyp, p.ID, p.PassportID.String(), p.ConnectionID.String(), tool, args,
	}, "\x00")))
	return base64.RawURLEncoding.EncodeToString(sum[:confirmOnceBytes]), true
}

// mint signs a code for one previewed call, and answers when it stops working.
func (c conversationConfirm) mint(ctx context.Context, call confirmedCall) (string, time.Time, error) {
	bind, ok := codeBinding(ctx, call.Tool, call.Args)
	if !ok {
		return "", time.Time{}, errConfirmCodeRefused
	}
	once := make([]byte, confirmOnceBytes)
	if _, err := rand.Read(once); err != nil {
		return "", time.Time{}, fmt.Errorf("draw a confirm code: %w", err)
	}
	expires := c.now().Add(confirmCodeTTL)
	ownTarget := call.Pins.Target != nil && call.Pins.Target.EntityType == call.TargetType && call.Pins.Target.EntityID == call.TargetID
	claim := wireConfirmCode{
		Typ: confirmCodeTyp, Bind: bind, TargetType: call.TargetType, TargetID: call.TargetID,
		Target: pinOnWire(call.Pins.Target, ownTarget), CoTarget: pinOnWire(call.Pins.CoTarget, false),
		Once: base64.RawURLEncoding.EncodeToString(once), Exp: expires.Unix(),
	}
	payload, err := json.Marshal(claim)
	if err != nil {
		return "", time.Time{}, err
	}
	return c.signer.seal(payload), expires, nil
}

// spend checks a presented code against this very call and spends it. It answers
// ctx marked released at the pinned versions, and the call the code confirmed.
// Any mismatch is the same refusal. The guard is asked last, so a code that does
// not match spends nothing.
func (c conversationConfirm) spend(ctx context.Context, tool, diffHash, code string) (context.Context, confirmedCall, error) {
	payload, ok := c.signer.open(code)
	if !ok {
		return ctx, confirmedCall{}, errConfirmCodeRefused
	}
	var got wireConfirmCode
	if err := json.Unmarshal(payload, &got); err != nil {
		return ctx, confirmedCall{}, errConfirmCodeRefused
	}
	bind, ok := codeBinding(ctx, tool, diffHash)
	left := time.Unix(got.Exp, 0).Sub(c.now())
	if !ok || got.Typ != confirmCodeTyp || got.Bind != bind || left <= 0 || got.Once == "" {
		return ctx, confirmedCall{}, errConfirmCodeRefused
	}
	first, err := c.guard.FirstUse(ctx, confirmCodeTyp+":"+got.Once, confirmCodeTTL+confirmGuardMargin)
	if err != nil {
		// Fail closed: a guard that cannot answer cannot prove this is the
		// first use. Its own words stay in the log, never in the answer.
		slog.Default().ErrorContext(ctx, "mcp: the confirm code's one-use check could not be made", "err", err)
		return ctx, confirmedCall{}, errConfirmCodeRefused
	}
	if !first {
		return ctx, confirmedCall{}, errConfirmCodeRefused
	}
	confirmed := got.call(tool, diffHash)
	confirmed.Once = got.Once
	marked, _, _ := markReleased(ctx, confirmed.Pins)
	return marked, confirmed, nil
}

// ConfirmInConversationError is round 1's answer: nothing changed, and the
// user's yes is needed in the conversation before the same call runs.
type ConfirmInConversationError struct {
	Tool      string
	Summary   string
	Code      string
	ExpiresAt time.Time
}

func (e *ConfirmInConversationError) Error() string {
	return "this call needs its user's yes in the conversation: " + apperrors.ErrRequiresApproval.Error()
}

func (e *ConfirmInConversationError) Unwrap() error { return apperrors.ErrRequiresApproval }

// confirmInConversationLead opens round 1's text. The e2e lane's checker reads
// it as a call that worked (e2e/llm/check.py), so the two are tested together.
const confirmInConversationLead = "Needs the user's yes in this conversation."

// confirmMeta names round 1's structured half in a result's _meta, beside the
// text that says the same, for a client or view that reads fields.
const confirmMeta = "margince/confirm"

// confirmFieldNextSteps names the calls that follow round 1, in order.
const confirmFieldNextSteps = "next_steps"

// confirmMetaOf is round 1 as fields: what would change, the code, and the next
// calls in order.
func confirmMetaOf(ask *ConfirmInConversationError) map[string]any {
	return map[string]any{
		fieldStatus:         "needs_confirmation",
		"tool":              ask.Tool,
		"what_would_change": ask.Summary,
		confirmCodeArg:      ask.Code,
		"expires_at":        ask.ExpiresAt.UTC().Format(time.RFC3339),
		confirmFieldNextSteps: []string{
			"Tell the user what_would_change in your own words and ask them.",
			"Only after they say yes: repeat this exact call with " + confirmCodeArg + ".",
			"If they say no: do not call it again; nothing changed.",
		},
	}
}

// confirmInConversationExplanation is round 1 in words every client reads: what
// would change, that the user decides here, and the exact next call. The code is
// in the text because the retry needs it and text is the one channel every
// client carries.
func confirmInConversationExplanation(ask *ConfirmInConversationError) string {
	what := "Nothing was changed yet."
	if ask.Summary != "" {
		what = "Nothing was changed yet; it would " + echoSafe(ask.Summary, workflow.MaxStagedSummary) + "."
	}
	return confirmInConversationLead + " " + what +
		" Tell the user in your own words what it would do and ask them. Only after they say yes, repeat " +
		"this exact call, adding \"" + confirmCodeArg + "\": \"" + ask.Code + "\" (good once, until " +
		ask.ExpiresAt.UTC().Format(time.RFC3339) + "). If they say no, do not call it again: nothing changed. " +
		"Nothing waits in the approval inbox. Blocks THIS call only: do the rest of what you were asked " +
		"that does not depend on it."
}

// refusalMetaOf is the structured half of a refusal, where it has one. A retry
// key may be in flight, or a call may need its user's yes in the conversation.
func refusalMetaOf(err error) map[string]any {
	if _, inFlight := errors.AsType[*claimInFlightError](err); inFlight {
		return map[string]any{refusalCodeMeta: httperr.IdempotencyInProgressCode}
	}
	if ask, asks := errors.AsType[*ConfirmInConversationError](err); asks {
		return map[string]any{confirmMeta: confirmMetaOf(ask)}
	}
	return nil
}
