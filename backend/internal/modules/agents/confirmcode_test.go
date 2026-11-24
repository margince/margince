// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// memGuard is the replay guard's contract in memory: one first use per key.
type memGuard struct {
	mu   sync.Mutex
	used map[string]bool
	err  error
}

func (g *memGuard) FirstUse(_ context.Context, key string, _ time.Duration) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.err != nil {
		return false, g.err
	}
	if g.used[key] {
		return false, nil
	}
	if g.used == nil {
		g.used = map[string]bool{}
	}
	g.used[key] = true
	return true, nil
}

// pinningApprovals answers the pins a staging would take from a fixed table of
// row versions, as the approvals engine reads them.
type pinningApprovals struct {
	*recordingApprovals
	versions map[ids.UUID]int64
}

func (p pinningApprovals) PinRecords(_ context.Context, in StageRequest) (auth.ReleasedRecordPins, error) {
	pin := func(entityType string, id ids.UUID) *auth.ReleasedRecordPin {
		version, ok := p.versions[id]
		if !ok {
			return nil
		}
		return &auth.ReleasedRecordPin{EntityType: entityType, EntityID: id, Version: version}
	}
	return auth.ReleasedRecordPins{Target: pin(in.TargetType, in.TargetID), CoTarget: pin(in.CoTargetType, in.CoTargetID)}, nil
}

// waitingTool waits for its own user, previews a company and the partner it
// pairs with, and records what each run was released by.
type waitingTool struct {
	name     string
	target   ids.UUID
	coTarget ids.UUID
	fail     bool
	runs     []context.Context
}

func (w *waitingTool) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: w.name, Title: "Waits", Version: "1", Description: "waits for its user",
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierConfirmationRequired, WaitsEvenForItsUser: true,
		InputSchema: json.RawMessage(`{"type":"object","properties":{"to":{"type":"string"}}}`),
	}
}

func (w *waitingTool) StageInfo(context.Context, json.RawMessage) (StageInfo, error) {
	// The tool's own read is not the pin, as it is not for a staging.
	misread := int64(99)
	return StageInfo{
		TargetType: "company", TargetID: w.target, TargetVersion: &misread,
		CoTargetType: "company", CoTargetID: w.coTarget, Summary: "set up a partner program",
	}, nil
}

func (w *waitingTool) Handle(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
	w.runs = append(w.runs, ctx)
	if w.fail {
		return nil, errors.New("the write failed before anything committed")
	}
	return json.RawMessage(`{}`), nil
}

type confirmFixture struct {
	reg       *Registry
	approvals *recordingApprovals
	tool      *waitingTool
	guard     *memGuard
	now       time.Time
	user      context.Context
}

func newConfirmFixture(t *testing.T) *confirmFixture {
	t.Helper()
	f := &confirmFixture{
		approvals: &recordingApprovals{releasable: true},
		tool:      &waitingTool{name: "set_up_partner", target: ids.NewV7(), coTarget: ids.NewV7()},
		guard:     &memGuard{},
		now:       time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC),
	}
	pinning := pinningApprovals{f.approvals, map[ids.UUID]int64{f.tool.target: 4, f.tool.coTarget: 7}}
	f.reg = NewRegistry(pinning, auth.NewGate(fullSeatAuthority{}))
	f.reg.Register(f.tool)
	p, _ := principal.Actor(passportCtx())
	p.ConnectionID = ids.NewV7()
	f.user = principal.WithActor(passportCtx(), p)
	return f
}

// armed is ctx as the MCP Dispatcher arms it for one call.
func (f *confirmFixture) armed(ctx context.Context) context.Context {
	return withConversationConfirm(ctx, conversationConfirm{
		signer: newInputStateSigner([]byte(strings.Repeat("k", 32))), guard: f.guard,
		now: func() time.Time { return f.now },
	})
}

// preview runs round 1 and answers the code it handed out.
func (f *confirmFixture) preview(ctx context.Context, t *testing.T, args string) string {
	t.Helper()
	_, err := f.reg.Invoke(ctx, f.tool.name, json.RawMessage(args))
	ask, asks := errors.AsType[*ConfirmInConversationError](err)
	if !asks {
		t.Fatalf("round 1 answered %v, want the user's yes asked for in the conversation", err)
	}
	return ask.Code
}

func withCode(args, code string) json.RawMessage {
	return json.RawMessage(strings.TrimSuffix(args, "}") + `,"confirm_code":"` + code + `"}`)
}

// Its own user is in the conversation, so the call asks there. Round 1 changes
// nothing and stages nothing. The same call with the code runs once, released as
// an approval would be, pinned to both rows as a staging would have pinned them.
func TestAWaitingCallIsConfirmedInTheConversationAndStagesNothing(t *testing.T) {
	f := newConfirmFixture(t)
	ctx := f.armed(f.user)
	args := `{"to":"resellers"}`
	code := f.preview(ctx, t, args)
	if len(f.tool.runs) != 0 || len(f.approvals.staged) != 0 {
		t.Fatalf("round 1 ran the tool %d time(s) and staged %d approval(s), want neither", len(f.tool.runs), len(f.approvals.staged))
	}
	if _, err := f.reg.Invoke(ctx, f.tool.name, withCode(args, code)); err != nil {
		t.Fatalf("round 2 with the code: %v", err)
	}
	if len(f.tool.runs) != 1 {
		t.Fatalf("round 2 ran the tool %d time(s), want once", len(f.tool.runs))
	}
	run := f.tool.runs[0]
	if !ApprovalRedeemed(run) {
		t.Error("the run must be released as a redeemed approval is")
	}
	for id, want := range map[ids.UUID]int64{f.tool.target: 4, f.tool.coTarget: 7} {
		if v, pinned := auth.ReleasedRecordVersion(run, "company", id); !pinned || v != want {
			t.Errorf("the write is pinned to %d (pinned=%v) on %v, want the previewed version %d", v, pinned, id, want)
		}
	}
	if len(f.approvals.staged) != 0 {
		t.Error("a call confirmed in the conversation must leave nothing in the approval inbox")
	}
}

// One yes is one run.
func TestAConfirmCodeIsGoodOnce(t *testing.T) {
	f := newConfirmFixture(t)
	ctx := f.armed(f.user)
	args := `{"to":"resellers"}`
	code := f.preview(ctx, t, args)
	if _, err := f.reg.Invoke(ctx, f.tool.name, withCode(args, code)); err != nil {
		t.Fatalf("first use: %v", err)
	}
	if _, err := f.reg.Invoke(ctx, f.tool.name, withCode(args, code)); !errors.Is(err, apperrors.ErrApprovalTokenInvalid) {
		t.Fatalf("second use answered %v, want it refused", err)
	}
	if len(f.tool.runs) != 1 {
		t.Errorf("the tool ran %d times on one yes, want once", len(f.tool.runs))
	}
}

// A code confirms one call by one user: no other arguments, tool, user,
// passport or connection, and not after it expires or once altered.
func TestAConfirmCodeConfirmsOnlyItsOwnCall(t *testing.T) {
	args := `{"to":"resellers"}`
	cases := []struct {
		name string
		use  func(f *confirmFixture, code string) error
	}{
		{"other arguments", func(f *confirmFixture, code string) error {
			_, err := f.reg.Invoke(f.armed(f.user), f.tool.name, withCode(`{"to":"everyone"}`, code))
			return err
		}},
		{"another user", func(f *confirmFixture, code string) error {
			_, err := f.reg.Invoke(f.armed(passportCtx()), f.tool.name, withCode(args, code))
			return err
		}},
		{"another connection of the same user", func(f *confirmFixture, code string) error {
			p, _ := principal.Actor(f.user)
			p.ConnectionID = ids.NewV7()
			_, err := f.reg.Invoke(f.armed(principal.WithActor(f.user, p)), f.tool.name, withCode(args, code))
			return err
		}},
		{"another passport of the same user", func(f *confirmFixture, code string) error {
			p, _ := principal.Actor(f.user)
			p.PassportID = ids.NewV7()
			_, err := f.reg.Invoke(f.armed(principal.WithActor(f.user, p)), f.tool.name, withCode(args, code))
			return err
		}},
		{"after it expires", func(f *confirmFixture, code string) error {
			f.now = f.now.Add(confirmCodeTTL + time.Second)
			_, err := f.reg.Invoke(f.armed(f.user), f.tool.name, withCode(args, code))
			return err
		}},
		{"altered", func(f *confirmFixture, code string) error {
			_, err := f.reg.Invoke(f.armed(f.user), f.tool.name, withCode(args, code+"x"))
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newConfirmFixture(t)
			code := f.preview(f.armed(f.user), t, args)
			if err := tc.use(f, code); !errors.Is(err, apperrors.ErrApprovalTokenInvalid) {
				t.Fatalf("answered %v, want the code refused", err)
			}
			if len(f.tool.runs) != 0 {
				t.Error("a refused code must run nothing")
			}
		})
	}
}

// Where no conversation can ask its user, the call stages into the inbox as
// before. The MCP door did not arm it, or nobody is watching the run.
func TestWithoutAConversationTheCallStagesIntoTheInboxAsBefore(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  func(f *confirmFixture) context.Context
	}{
		{"not armed by the MCP door", func(f *confirmFixture) context.Context { return f.user }},
		{"a run nobody watches", func(f *confirmFixture) context.Context { return f.armed(principal.WithUnattended(f.user)) }},
		{"a passport minted without a connection", func(f *confirmFixture) context.Context {
			p, _ := principal.Actor(f.user)
			p.ConnectionID = ids.UUID{}
			return f.armed(principal.WithActor(f.user, p))
		}},
		{"a guard missing", func(f *confirmFixture) context.Context {
			return withConversationConfirm(f.user, conversationConfirm{signer: newInputStateSigner([]byte(strings.Repeat("k", 32)))})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newConfirmFixture(t)
			_, err := f.reg.Invoke(tc.ctx(f), f.tool.name, json.RawMessage(`{"to":"resellers"}`))
			if !errors.Is(err, apperrors.ErrRequiresApproval) || len(f.approvals.staged) != 1 {
				t.Fatalf("answered %v with %d staged, want it staged into the inbox", err, len(f.approvals.staged))
			}
			if _, asks := errors.AsType[*ConfirmInConversationError](err); asks {
				t.Error("nothing here can ask the user, so no code may be handed out")
			}
		})
	}
}

// A code presented where no conversation can confirm releases nothing.
func TestAConfirmCodeWhereNoConversationCanConfirmIsRefused(t *testing.T) {
	f := newConfirmFixture(t)
	args := `{"to":"resellers"}`
	code := f.preview(f.armed(f.user), t, args)
	if _, err := f.reg.Invoke(f.user, f.tool.name, withCode(args, code)); !errors.Is(err, apperrors.ErrApprovalTokenInvalid) {
		t.Fatalf("answered %v, want the code refused off the MCP conversation", err)
	}
	if len(f.tool.runs) != 0 || len(f.approvals.staged) != 0 {
		t.Error("a refused code must run nothing and stage nothing")
	}
}

// A yes covers one run, and a run that failed spent it. What it did outside the
// database before failing may not be undone, so the retry needs a new yes.
func TestAFailedRunSpendsTheCode(t *testing.T) {
	f := newConfirmFixture(t)
	ctx := f.armed(f.user)
	args := `{"to":"resellers"}`
	code := f.preview(ctx, t, args)
	f.tool.fail = true
	if _, err := f.reg.Invoke(ctx, f.tool.name, withCode(args, code)); err == nil {
		t.Fatal("the failing run answered success")
	}
	f.tool.fail = false
	if _, err := f.reg.Invoke(ctx, f.tool.name, withCode(args, code)); !errors.Is(err, apperrors.ErrApprovalTokenInvalid) {
		t.Fatalf("the retry on a spent code → %v, want it refused", err)
	}
}

// The code releases no more than the caller could release from the inbox. A call
// it could not release, one that cannot be undone, waits there for a colleague.
func TestACallTheCallerCouldNotReleaseWaitsInTheInbox(t *testing.T) {
	f := newConfirmFixture(t)
	f.approvals.releasable = false
	_, err := f.reg.Invoke(f.armed(f.user), f.tool.name, json.RawMessage(`{"to":"resellers"}`))
	if _, asks := errors.AsType[*ConfirmInConversationError](err); asks || len(f.approvals.staged) != 1 {
		t.Fatalf("answered %v with %d staged, want it staged and no code", err, len(f.approvals.staged))
	}
}

// Whether the caller may release it is asked again at the spend. A project held
// since the preview makes the filing one it can no longer release.
func TestACodeForACallNoLongerReleasableIsRefused(t *testing.T) {
	f := newConfirmFixture(t)
	ctx := f.armed(f.user)
	args := `{"to":"resellers"}`
	code := f.preview(ctx, t, args)
	f.approvals.releasable = false
	if _, err := f.reg.Invoke(ctx, f.tool.name, withCode(args, code)); !errors.Is(err, errConfirmCodeNoLonger) {
		t.Fatalf("answered %v, want the code refused for a call no longer releasable", err)
	}
	if len(f.tool.runs) != 0 {
		t.Error("a call the caller can no longer release must not run")
	}
}

// A guard that cannot answer cannot prove a first use, so the code is refused.
func TestAGuardThatCannotAnswerRefusesTheCode(t *testing.T) {
	f := newConfirmFixture(t)
	ctx := f.armed(f.user)
	args := `{"to":"resellers"}`
	code := f.preview(ctx, t, args)
	f.guard.err = errors.New("NOAUTH the replay store wants a password")
	_, err := f.reg.Invoke(ctx, f.tool.name, withCode(args, code))
	if !errors.Is(err, apperrors.ErrApprovalTokenInvalid) {
		t.Fatalf("answered %v, want the code refused", err)
	}
	if strings.Contains(err.Error(), "NOAUTH") {
		t.Errorf("the refusal shows the guard's own words: %v", err)
	}
	if len(f.tool.runs) != 0 {
		t.Error("an unproven first use must run nothing")
	}
}

// Each of the two releases is enough on its own, so one call carries one.
func TestAnApprovalAndAConfirmCodeTogetherAreRefused(t *testing.T) {
	_, err := splitReserved(json.RawMessage(`{"approval_id":"` + ids.NewV7().String() + `","confirm_code":"c"}`))
	if badArgs, ok := errors.AsType[*BadArgsError](err); !ok || !errors.Is(badArgs.Cause, errTwoReleases) {
		t.Fatalf("answered %v, want the pair refused", err)
	}
}

// Only a tool that can wait for its own user advertises the code.
func TestOnlyAToolThatWaitsForItsUserAdvertisesTheConfirmCode(t *testing.T) {
	f := newConfirmFixture(t)
	f.reg.Register(&servingTool{spec: mcp.ToolSpec{
		Name: "merge_tags", Title: "Merge tags", Version: "1", Description: "merges",
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierConfirmationRequired, InputSchema: json.RawMessage(`{"type":"object"}`),
	}})
	for name, want := range map[string]bool{f.tool.name: true, "merge_tags": false} {
		spec, _ := f.reg.Spec(name)
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		if _, has := schema.Properties[confirmCodeArg]; has != want {
			t.Errorf("%s advertises %s: %v, want %v", name, confirmCodeArg, has, want)
		}
	}
}

// At the MCP door, the Dispatcher arms the call itself. Round 1 says what would
// change, the code and the next call, in words and in _meta. The same call with
// that code then runs through the same door.
func TestTheMCPDoorAsksInTheConversationAndRunsOnTheCode(t *testing.T) {
	f := newConfirmFixture(t)
	d := NewDispatcher(f.reg, func(context.Context) (context.Context, error) { return f.user, nil }, "t", "0")
	d.inputState = newInputStateSigner([]byte(strings.Repeat("k", 32)))
	d.confirmGuard = f.guard
	call := func(args string) map[string]any {
		t.Helper()
		params, _ := json.Marshal(map[string]any{"name": f.tool.name, "arguments": json.RawMessage(args)})
		out, ok := d.call(context.Background(), params, framing{}).(map[string]any)
		if !ok {
			t.Fatalf("tools/call answered a %T, want a result", out)
		}
		return out
	}

	asked := call(`{"to":"resellers"}`)
	meta, _ := asked["_meta"].(map[string]any)
	fields, _ := meta[confirmMeta].(map[string]any)
	code, _ := fields[confirmCodeArg].(string)
	if code == "" || fields["what_would_change"] != "set up a partner program" || fields[confirmFieldNextSteps] == nil {
		t.Fatalf("round 1 _meta %v, want the code, what would change and the next steps", meta)
	}
	text, _ := json.Marshal(asked["content"])
	for _, words := range []string{"yes in this conversation", code, "set up a partner program", "Nothing waits in the approval inbox"} {
		if !strings.Contains(string(text), words) {
			t.Errorf("round 1's text does not say %q: %s", words, text)
		}
	}
	if len(f.approvals.staged) != 0 || len(f.tool.runs) != 0 {
		t.Fatal("round 1 at the door must stage nothing and run nothing")
	}
	if done := call(string(withCode(`{"to":"resellers"}`, code))); done["isError"] == true {
		t.Fatalf("round 2 at the door: %v", done)
	}
	if len(f.tool.runs) != 1 {
		t.Errorf("round 2 at the door ran the tool %d time(s), want once", len(f.tool.runs))
	}
}

// The e2e lane scores round 1 as a call that worked by its opening words. A
// rewording the checker does not follow would score every preview as a failure.
// So the checker's own pattern is read and held to the words.
func TestTheLanesCheckerReadsRoundOneAsACallThatWorked(t *testing.T) {
	source, err := os.ReadFile("../../../../e2e/llm/check.py")
	if err != nil {
		t.Fatalf("reading the lane's checker: %v", err)
	}
	found := regexp.MustCompile(`_STAGED = re\.compile\(\s*r"([^"]*)"`).FindSubmatch(source)
	if found == nil {
		t.Fatal("e2e/llm/check.py has no _STAGED pattern to read")
	}
	staged := regexp.MustCompile("(?i)" + string(found[1]))
	if got := confirmInConversationExplanation(&ConfirmInConversationError{Code: "c"}); !staged.MatchString(got) {
		t.Errorf("e2e/llm/check.py's _STAGED does not match round 1 (%q), so the lane would score it as a failed call", got)
	}
}

// With no approvals engine on the surface, a waiting call has nothing to stand
// in for. Its refusal stands and no code is handed out.
func TestWithNoApprovalsTheWaitingCallIsRefusedPlainly(t *testing.T) {
	f := newConfirmFixture(t)
	f.reg = NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	f.reg.Register(f.tool)
	_, err := f.reg.Invoke(f.armed(f.user), f.tool.name, json.RawMessage(`{"to":"resellers"}`))
	if !errors.Is(err, apperrors.ErrRequiresApproval) {
		t.Fatalf("answered %v, want the plain refusal", err)
	}
	if _, asks := errors.AsType[*ConfirmInConversationError](err); asks {
		t.Error("a surface with no approvals must not hand out a code")
	}
}

// A code names who it is for, so a context with no caller can neither mint one
// nor spend one.
func TestAConfirmCodeNeedsACaller(t *testing.T) {
	f := newConfirmFixture(t)
	c, _ := conversationConfirmFrom(f.armed(f.user))
	call := confirmedCall{Tool: f.tool.name, Args: "hash"}
	if _, _, err := c.mint(context.Background(), call); !errors.Is(err, apperrors.ErrApprovalTokenInvalid) {
		t.Errorf("minting with no caller → %v, want refused", err)
	}
	code, _, err := c.mint(f.user, call)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.spend(context.Background(), f.tool.name, "hash", code); !errors.Is(err, apperrors.ErrApprovalTokenInvalid) {
		t.Errorf("spending with no caller → %v, want refused", err)
	}
}

// The pins survive the trip through the code, context digest included, so the
// writer re-checks what the preview pinned.
func TestThePinsSurviveTheCode(t *testing.T) {
	f := newConfirmFixture(t)
	c, _ := conversationConfirmFrom(f.armed(f.user))
	lead, company := ids.NewV7(), ids.NewV7()
	code, _, err := c.mint(f.user, confirmedCall{
		Tool: f.tool.name, Args: "hash", TargetType: "lead", TargetID: lead,
		Pins: auth.ReleasedRecordPins{
			Target:   &auth.ReleasedRecordPin{EntityType: "lead", EntityID: lead, Version: 3, ContextDigest: "facts"},
			CoTarget: &auth.ReleasedRecordPin{EntityType: "company", EntityID: company, Version: 9},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	marked, call, err := c.spend(f.user, f.tool.name, "hash", code)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := auth.ReleasedRecordVersion(marked, "lead", lead); !ok || v != 3 {
		t.Errorf("the lead is pinned at %d (%v), want 3", v, ok)
	}
	if v, ok := auth.ReleasedRecordVersion(marked, "company", company); !ok || v != 9 {
		t.Errorf("the company is pinned at %d (%v), want 9", v, ok)
	}
	if digest, ok := auth.ReleasedRecordContext(marked, "lead", lead); !ok || digest != "facts" {
		t.Errorf("the lead's context is %q (%v), want the previewed digest", digest, ok)
	}
	if call.TargetType != "lead" || call.TargetID != lead || call.Once == "" {
		t.Errorf("the code names %+v, want the lead and its one-use id", call)
	}
}
