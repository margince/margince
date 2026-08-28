// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// noArgs is the document an operation that declares no arguments receives. It
// is `{}` and not empty: the strict decoder refuses an absent document, which
// is what a client sending nothing at all would produce.
const noArgs = `{}`

// ownArgs is what a confirm-first operation's request carries when the caller
// names their own endpoint — the ordinary case every success-path fixture in
// this suite exercises. endpointID is the row every fixture's singleRows
// answer as the caller's own, so this is the one value that survives the
// mismatch check.
const ownArgs = `{"endpoint_id":"` + endpointID + `"}`

func TestOpeningStampsTheOwnerFromTheInvocation(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	// The read answers nothing — this member has no endpoint — and the insert
	// returns the row it wrote.
	rt.tx.noRows = map[int]bool{1: true}
	rt.tx.singleRows = [][]any{endpointRow(endpointID, ownerUserID, "", true)}

	out, err := open(context.Background(), rt, json.RawMessage(noArgs))
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	sql, args := rt.tx.statementMentioning(t, "VALUES ($1::uuid, $2, $3)")
	if len(args) != 3 || args[0] != ownerUserID {
		t.Fatalf("the insert must stamp the caller as the owner; it bound %v in\n%s", args, sql)
	}
	if args[1] != inboundSlug {
		t.Fatalf("the insert must name the unit's declared slug %q; it bound %v", inboundSlug, args[1])
	}
	minted, ok := args[2].(string)
	if !ok || !extension.ValidInboundRef(minted) {
		t.Fatalf("the insert stored %v as the address, which the inbound edge would not route", args[2])
	}
	stored := jsonOf[endpoint](t, out)
	if stored.UserID != ownerUserID {
		t.Fatalf("the answer names %q as the owner, not the caller", stored.UserID)
	}
	if stored.Ref == "" {
		t.Fatal("the answer carries no address, so the member has nothing to point a sender at")
	}
}

// The whole reason the ref exists: one declared edge, one endpoint per person.
// A second member opening is an ordinary open, not a refusal — that refusal was
// the symptom of resolving arrivals by the declared slug.
func TestASecondMemberOpensTheirOwnEndpointOnTheSameEdge(t *testing.T) {
	t.Parallel()
	refs := map[string]bool{}
	for _, member := range []string{ownerUserID, colleagueUserID} {
		rt := newRuntime()
		rt.caller = extension.Caller{Type: extension.CallerHuman, UserID: member}
		rt.tx.noRows = map[int]bool{1: true}
		rt.tx.singleRows = [][]any{endpointRow(endpointID, member, "", true)}

		out, err := open(context.Background(), rt, json.RawMessage(noArgs))
		if err != nil {
			t.Fatalf("opening for %s: %v", member, err)
		}
		if got := jsonOf[endpoint](t, out).UserID; got != member {
			t.Fatalf("the answer names %q as the owner, not %q", got, member)
		}
		_, args := rt.tx.statementMentioning(t, "VALUES ($1::uuid, $2, $3)")
		ref, ok := args[2].(string)
		if !ok {
			t.Fatalf("the minted address is %T, not text", args[2])
		}
		if refs[ref] {
			t.Fatalf("two members were given the same address %q, so one of them would receive the other's requests", ref)
		}
		refs[ref] = true
	}
}

// A member's endpoint is keyed by the member AND the declared edge. A read on
// the member alone would answer an arbitrary one of their endpoints the day this
// unit declares a second edge.
func TestReadingAnEndpointNamesTheDeclaredEdgeAsWellAsTheMember(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.singleRows = [][]any{endpointRow(endpointID, ownerUserID, "", true)}

	out, err := readEndpoint(context.Background(), rt, json.RawMessage(noArgs))
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	_, args := rt.tx.statementMentioning(t, "user_id = $1::uuid AND slug = $2")
	if args[0] != ownerUserID || args[1] != inboundSlug {
		t.Fatalf("the read is keyed on %v, not on the caller and the declared edge", args)
	}
	answer := jsonOf[struct {
		Opened   bool      `json:"opened"`
		Endpoint *endpoint `json:"endpoint"`
	}](t, out)
	if !answer.Opened || answer.Endpoint == nil || answer.Endpoint.Ref != ownerRef {
		t.Fatalf("the read answered %+v", answer)
	}
}

// Not having opened one is the ordinary state of the screen this serves.
func TestReadingBeforeOpeningIsNotAnError(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.noRows = map[int]bool{1: true}

	out, err := readEndpoint(context.Background(), rt, json.RawMessage(noArgs))
	if err != nil {
		t.Fatalf("reading before opening must not be a failure: %v", err)
	}
	if strings.Contains(string(out), `"opened":true`) || strings.Contains(string(out), `"endpoint"`) {
		t.Fatalf("the read reported an endpoint that does not exist:\n%s", out)
	}
}

// The owner is not an argument, so a client that tries to name one is refused
// rather than quietly ignored. That refusal is what stops this unit's own front
// door from forging the consent the anonymous edge reads.
func TestOpeningRefusesAnOwnerNamedInTheBody(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	_, err := open(context.Background(), rt, json.RawMessage(`{"user_id":"`+colleagueUserID+`"}`))
	if err == nil {
		t.Fatal("naming an owner in the body was accepted")
	}
	if len(rt.tx.statements) != 0 {
		t.Fatalf("the refusal must come before any statement; it issued:\n%s", strings.Join(rt.tx.statements, "\n"))
	}
}

// TestOpeningTwiceAnswersTheSameEndpointAndInsertsNothing covers the ordinary
// re-open AND a concurrent open racing this one, because open() cannot tell
// the two apart and must not need to: the insert is ON CONFLICT DO NOTHING, so
// it is issued every time — even against a row that already exists, which is
// what makes a genuinely simultaneous second `open` land on the SAME
// conflict-then-read path rather than surfacing the unique constraint as a
// bare error. What must hold either way is that nothing NEW is created: the
// answer names the row that already existed, and nothing is recorded for a
// write this call did not make.
func TestOpeningTwiceAnswersTheSameEndpointAndInsertsNothing(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	// The insert conflicts (no row) and the fallback read finds the endpoint
	// that already existed.
	rt.tx.noRows[1] = true
	rt.tx.singleRows = [][]any{endpointRow(endpointID, ownerUserID, "", true)}

	out, err := open(context.Background(), rt, json.RawMessage(noArgs))
	if err != nil {
		t.Fatalf("re-opening: %v", err)
	}
	stored := jsonOf[endpoint](t, out)
	if stored.ID != endpointID {
		t.Fatalf("re-opening answered %q, not the endpoint that already existed", stored.ID)
	}
	// The address MUST survive. A new one on every open would break every
	// sender already pointed at the old URL, silently and at the moment a
	// member did the least alarming thing available to them.
	if stored.Ref != ownerRef {
		t.Fatalf("re-opening changed the address to %q, breaking every sender pointed at %q", stored.Ref, ownerRef)
	}
	if len(rt.tx.audited) != 0 {
		t.Fatalf("re-opening recorded %d ledger rows for a write it did not make", len(rt.tx.audited))
	}
}

func TestEveryEndpointOperationRefusesAnInvocationWithNobodyBehindIt(t *testing.T) {
	t.Parallel()
	for name, call := range map[string]func(context.Context, extension.Runtime) error{
		"open": func(ctx context.Context, rt extension.Runtime) error {
			_, err := open(ctx, rt, json.RawMessage(noArgs))
			return err
		},
		"mint": func(ctx context.Context, rt extension.Runtime) error {
			_, err := mintSecret(ctx, rt, json.RawMessage(noArgs))
			return err
		},
		"enable": func(ctx context.Context, rt extension.Runtime) error {
			_, err := setEnabled(ctx, rt, json.RawMessage(`{"enabled":true}`))
			return err
		},
		"register": func(ctx context.Context, rt extension.Runtime) error {
			_, err := registerURL(ctx, rt, json.RawMessage(`{"url":"https://example.com/hook"}`))
			return err
		},
		"list": func(ctx context.Context, rt extension.Runtime) error {
			_, err := listInbound(ctx, rt, json.RawMessage(noArgs))
			return err
		},
		"read": func(ctx context.Context, rt extension.Runtime) error {
			_, err := readEndpoint(ctx, rt, json.RawMessage(noArgs))
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rt := newRuntime().unattended()
			if err := call(context.Background(), rt); !errors.Is(err, extension.ErrForbidden) {
				t.Fatalf("an unattended invocation must be forbidden, got %v", err)
			}
			if len(rt.tx.statements) != 0 {
				t.Fatalf("the refusal must come before any statement; it issued:\n%s", strings.Join(rt.tx.statements, "\n"))
			}
		})
	}
}

// Retrying set_enabled with the state the endpoint already has must write
// NOTHING — matching how `open` is documented to behave when asked twice.
// Bumping the version and appending a ledger entry for a change that never
// happened would drift the version out from under a screen that just read it,
// and would tell an auditor a state change occurred at a moment nothing
// changed.
func TestSetEnabledWithTheStateItAlreadyHasIsANoOp(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.singleRows = [][]any{
		endpointRow(endpointID, ownerUserID, "", true),
	}

	out, err := setEnabled(context.Background(), rt, json.RawMessage(`{"enabled":true}`))
	if err != nil {
		t.Fatalf("asking for the state it already has: %v", err)
	}
	if !jsonOf[endpoint](t, out).Enabled {
		t.Fatal("the answer must still report the endpoint's actual (unchanged) state")
	}
	for _, sql := range rt.tx.statements {
		if strings.HasPrefix(sql, "UPDATE") {
			t.Fatalf("a no-op ask still wrote:\n%s", sql)
		}
	}
	if len(rt.tx.audited) != 0 {
		t.Fatalf("a no-op ask appended %d ledger rows, want 0", len(rt.tx.audited))
	}
	if len(rt.tx.published) != 0 {
		t.Fatalf("a no-op ask published %d events, want 0", len(rt.tx.published))
	}
}

func TestPausingRecordsBothImagesAgainstTheOwnersRow(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.singleRows = [][]any{
		endpointRow(endpointID, ownerUserID, "", true),
		endpointRow(endpointID, ownerUserID, "", false),
	}

	out, err := setEnabled(context.Background(), rt, json.RawMessage(`{"enabled":false}`))
	if err != nil {
		t.Fatalf("pausing: %v", err)
	}
	if jsonOf[endpoint](t, out).Enabled {
		t.Fatal("the answer still reports the endpoint as accepting")
	}
	sql, args := rt.tx.statementMentioning(t, "enabled = $2")
	if !strings.Contains(sql, "version = version + 1") {
		t.Fatalf("a governed write must bump the version:\n%s", sql)
	}
	if args[0] != ownerUserID {
		t.Fatalf("the update must be predicated on the caller; it bound %v", args)
	}
	if len(rt.tx.audited) != 1 {
		t.Fatalf("pausing recorded %d ledger rows, want 1", len(rt.tx.audited))
	}
	change := rt.tx.audited[0]
	if change.Before == nil || change.After == nil {
		t.Fatal("a state change records both images or the trail cannot say what changed")
	}
	if rt.tx.published[0].Verb != eventDisabled {
		t.Fatalf("pausing published %q", rt.tx.published[0].Verb)
	}
}

// TestTheBeforeImageIsReadLocked guards against two overlapping governed
// writes (setEnabled racing registerURL, say) both reading the SAME
// before-image and both updating: without the lock, the second writer's
// ledger row would name a "before" state its own UPDATE never actually
// replaced, because the first writer's change already landed and vanished
// from the trail between the two audit rows.
func TestTheBeforeImageIsReadLocked(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.singleRows = [][]any{
		endpointRow(endpointID, ownerUserID, "", true),
		endpointRow(endpointID, ownerUserID, "", false),
	}
	if _, err := setEnabled(context.Background(), rt, json.RawMessage(`{"enabled":false}`)); err != nil {
		t.Fatalf("pausing: %v", err)
	}
	sql, _ := rt.tx.statementMentioning(t, "user_id = $1::uuid AND slug = $2")
	if !strings.Contains(sql, "FOR UPDATE") {
		t.Fatalf("the before-image read does not lock the row, so a second overlapping write can record a before-image that was never actually replaced:\n%s", sql)
	}
}

func TestRegisteringAnAddressStoresTheCheckedForm(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	const address = "https://example.com/hooks/crm"
	rt.tx.singleRows = [][]any{
		endpointRow(endpointID, ownerUserID, "", true),
		endpointRow(endpointID, ownerUserID, address, true),
	}

	if _, err := registerURL(context.Background(), rt,
		json.RawMessage(`{"endpoint_id":"`+endpointID+`","url":"  `+address+`  "}`)); err != nil {
		t.Fatalf("registering: %v", err)
	}
	_, args := rt.tx.statementMentioning(t, "url = $2")
	if args[1] != address {
		t.Fatalf("the stored address is %q, not the checked form %q", args[1], address)
	}
	if rt.tx.published[0].Verb != eventURLRegistered {
		t.Fatalf("registering published %q", rt.tx.published[0].Verb)
	}
}

// The IDOR guard on the other confirm-first operation: naming a colleague's
// endpoint id must re-point nothing and answer not-found, never a permission
// error — the same rule as mintSecret, and load-bearing here because this
// operation is the one that silently changes where every LATER send posts a
// member's message bodies.
func TestRegisteringAnAddressForAnotherMembersEndpointIsRefusedAsNotFound(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.singleRows = [][]any{endpointRow(endpointID, ownerUserID, "", true)}

	_, err := registerURL(context.Background(), rt,
		json.RawMessage(`{"endpoint_id":"`+colleagueEndpointID+`","url":"https://example.com/hook"}`))
	if !errors.Is(err, extension.ErrNotFound) {
		t.Fatalf("naming another endpoint must answer not-found, got %v", err)
	}
	if errors.Is(err, extension.ErrForbidden) {
		t.Fatal("a permission error confirms the id belongs to somebody — existence must stay hidden")
	}
	// HasPrefix, not Contains: the ownership read now locks with FOR UPDATE
	// (see lockedEndpointOf), and that substring must not be mistaken for the
	// write statement this test asserts never runs.
	for _, sql := range rt.tx.statements {
		if strings.HasPrefix(sql, "UPDATE") {
			t.Fatalf("the mismatch was not caught before a write:\n%s", sql)
		}
	}
}

func TestChangingAnEndpointNobodyOpenedIsNotFound(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.noRows = map[int]bool{1: true}

	_, err := setEnabled(context.Background(), rt, json.RawMessage(`{"enabled":true}`))
	if !errors.Is(err, extension.ErrNotFound) {
		t.Fatalf("changing an endpoint that was never opened must be not-found, got %v", err)
	}
}
