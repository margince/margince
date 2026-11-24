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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
	// confirmOnceBytes is the entropy of the id the replay guard spends.
	confirmOnceBytes = 16
)

// errConfirmCodeRefused is every refused code's answer. It does not say which of
// used, expired, foreign or mismatched it was: the remedy is the same.
var errConfirmCodeRefused = fmt.Errorf("this confirm_code does not confirm this call: it is used, expired, "+
	"or was answered for a different call or person. Call again without it for a new preview: %w",
	apperrors.ErrApprovalTokenInvalid)

// errConfirmCodeNotHere answers a code presented where no conversation can
// confirm: the REST door, a scheduled run, or a call that does not wait.
var errConfirmCodeNotHere = fmt.Errorf("confirm_code confirms a call its user previewed in this MCP "+
	"conversation; this call cannot be confirmed here: %w", apperrors.ErrApprovalTokenInvalid)

// ConfirmGuard spends one-use ids. platform/replayguard is the production one.
type ConfirmGuard interface {
	FirstUse(ctx context.Context, key string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, key string) error
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
// call here. The MCP door armed it, the decision is that user's, and a person is
// present to ask.
func confirmsInConversation(ctx context.Context, waitsEvenForItsUser bool, refusal error) bool {
	if !waitsEvenForItsUser || !errors.Is(refusal, apperrors.ErrRequiresApproval) || principal.Unattended(ctx) {
		return false
	}
	if _, armed := conversationConfirmFrom(ctx); !armed {
		return false
	}
	p, ok := principal.Actor(ctx)
	return ok && p.ActsAsItsUser()
}

// wireConfirmCode is what a code claims.
type wireConfirmCode struct {
	Typ        string `json:"t"`
	Sub        string `json:"sub"`
	Passport   string `json:"pp"`
	Connection string `json:"conn"`
	Tool       string `json:"tool"`
	Args       string `json:"args"`
	// The record the preview read, and its version when it has one.
	TargetType string   `json:"tt,omitempty"`
	TargetID   ids.UUID `json:"tid,omitzero"`
	Version    *int64   `json:"v,omitempty"`
	Once       string   `json:"n"`
	Exp        int64    `json:"exp"`
}

// previewedTarget is the record a preview read, pinned at the version it saw.
type previewedTarget struct {
	Type    string
	ID      ids.UUID
	Version *int64
}

// codeIdentity is who a code is for: every part of the caller a code may not
// travel between.
func codeIdentity(ctx context.Context) wireConfirmCode {
	p, _ := principal.Actor(ctx)
	return wireConfirmCode{
		Typ: confirmCodeTyp, Sub: p.ID,
		Passport: p.PassportID.String(), Connection: p.ConnectionID.String(),
	}
}

// mint signs a code for one previewed call, and answers when it stops working.
func (c conversationConfirm) mint(ctx context.Context, tool, diffHash string, target previewedTarget) (string, time.Time, error) {
	once := make([]byte, confirmOnceBytes)
	if _, err := rand.Read(once); err != nil {
		return "", time.Time{}, fmt.Errorf("draw a confirm code: %w", err)
	}
	expires := c.now().Add(confirmCodeTTL)
	claim := codeIdentity(ctx)
	claim.Tool, claim.Args = tool, diffHash
	claim.TargetType, claim.TargetID, claim.Version = target.Type, target.ID, target.Version
	claim.Once, claim.Exp = base64.RawURLEncoding.EncodeToString(once), expires.Unix()
	payload, err := json.Marshal(claim)
	if err != nil {
		return "", time.Time{}, err
	}
	return c.signer.seal(payload), expires, nil
}

// spend checks a presented code against this very call and spends it, answering
// ctx marked released at the pinned version. Any mismatch is the same refusal.
// The guard is asked last, so a code that does not match spends nothing.
func (c conversationConfirm) spend(ctx context.Context, tool, diffHash, code string) (context.Context, string, error) {
	payload, ok := c.signer.open(code)
	if !ok {
		return ctx, "", errConfirmCodeRefused
	}
	var got wireConfirmCode
	if err := json.Unmarshal(payload, &got); err != nil {
		return ctx, "", errConfirmCodeRefused
	}
	want := codeIdentity(ctx)
	left := time.Unix(got.Exp, 0).Sub(c.now())
	if got.Typ != want.Typ || got.Sub != want.Sub || got.Passport != want.Passport ||
		got.Connection != want.Connection || got.Tool != tool || got.Args != diffHash || left <= 0 || got.Once == "" {
		return ctx, "", errConfirmCodeRefused
	}
	first, err := c.guard.FirstUse(ctx, confirmCodeTyp+":"+got.Once, left)
	if err != nil {
		// Fail closed: a guard that cannot answer cannot prove this is the
		// first use, and a second use is what it exists to stop.
		return ctx, "", fmt.Errorf("%w (the one-use check could not be made: %w)", errConfirmCodeRefused, err)
	}
	if !first {
		return ctx, "", errConfirmCodeRefused
	}
	return markConfirmed(ctx, got), got.Once, nil
}

// markConfirmed marks ctx released as a redeemed approval would be, so every
// writer honours it. The previewed version travels into the write, where the
// writer re-checks it under its own row lock.
func markConfirmed(ctx context.Context, claim wireConfirmCode) context.Context {
	if claim.Version != nil && claim.TargetType != "" && !claim.TargetID.IsZero() {
		ctx = auth.WithReleasedRecordPins(ctx, auth.ReleasedRecordPins{Target: &auth.ReleasedRecordPin{
			EntityType: claim.TargetType, EntityID: claim.TargetID, Version: *claim.Version,
		}})
		return withConfirmedInConversation(withApprovalRedeemed(ctx, *claim.Version, true))
	}
	return withConfirmedInConversation(withApprovalRedeemed(ctx, 0, false))
}

type confirmedInConversationKey struct{}

func withConfirmedInConversation(ctx context.Context) context.Context {
	return context.WithValue(ctx, confirmedInConversationKey{}, true)
}

// ConfirmedInConversation reports whether this call's release was its user's
// yes in the conversation rather than a decision in the inbox.
func ConfirmedInConversation(ctx context.Context) bool {
	confirmed, ok := ctx.Value(confirmedInConversationKey{}).(bool)
	return ok && confirmed
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

// confirmMetaOf is round 1 as fields: what would change, the code, and the next
// calls in order.
func confirmMetaOf(ask *ConfirmInConversationError) map[string]any {
	return map[string]any{
		"status":            "needs_confirmation",
		"tool":              ask.Tool,
		"what_would_change": ask.Summary,
		confirmCodeArg:      ask.Code,
		"expires_at":        ask.ExpiresAt.UTC().Format(time.RFC3339),
		"next_steps": []string{
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
