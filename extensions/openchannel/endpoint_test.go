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

func TestOpeningTwiceAnswersTheSameEndpointAndInsertsNothing(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.singleRows = [][]any{endpointRow(endpointID, ownerUserID, "", true)}

	out, err := open(context.Background(), rt, json.RawMessage(noArgs))
	if err != nil {
		t.Fatalf("re-opening: %v", err)
	}
	for _, sql := range rt.tx.statements {
		if strings.Contains(sql, "VALUES ($1::uuid, $2, $3)") {
			t.Fatalf("re-opening inserted a second endpoint:\n%s", sql)
		}
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
	for _, sql := range rt.tx.statements {
		if strings.Contains(sql, "UPDATE") {
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
