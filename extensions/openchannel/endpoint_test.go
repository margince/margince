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

func TestOpeningStampsTheOwnerFromTheInvocation(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	// Two reads answer nothing — no endpoint for this member, and nobody
	// holding the slug — and the insert returns the row it wrote.
	rt.tx.noRows = map[int]bool{1: true, 2: true}
	rt.tx.singleRows = [][]any{endpointRow(endpointID, ownerUserID, "", true)}

	out, err := open(context.Background(), rt, json.RawMessage(noArgs))
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	sql, args := rt.tx.statementMentioning(t, "VALUES ($1::uuid, $2)")
	if len(args) != 2 || args[0] != ownerUserID {
		t.Fatalf("the insert must stamp the caller as the owner; it bound %v in\n%s", args, sql)
	}
	if args[1] != inboundSlug {
		t.Fatalf("the insert must claim the unit's declared slug %q; it bound %v", inboundSlug, args[1])
	}
	stored := jsonOf[endpoint](t, out)
	if stored.UserID != ownerUserID {
		t.Fatalf("the answer names %q as the owner, not the caller", stored.UserID)
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
		if strings.Contains(sql, "VALUES ($1::uuid, $2)") {
			t.Fatalf("re-opening inserted a second endpoint:\n%s", sql)
		}
	}
	if got := jsonOf[endpoint](t, out).ID; got != endpointID {
		t.Fatalf("re-opening answered %q, not the endpoint that already existed", got)
	}
	if len(rt.tx.audited) != 0 {
		t.Fatalf("re-opening recorded %d ledger rows for a write it did not make", len(rt.tx.audited))
	}
}

func TestOpeningRefusesASlugAnotherMemberHolds(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	// This member has no endpoint; the declared slug is held by a colleague.
	rt.tx.noRows = map[int]bool{1: true}
	rt.tx.singleRows = [][]any{endpointRow(endpointID, colleagueUserID, "", true)}

	_, err := open(context.Background(), rt, json.RawMessage(noArgs))
	if !errors.Is(err, extension.ErrConflict) {
		t.Fatalf("a slug another member holds must be a conflict, got %v", err)
	}
	if strings.Contains(err.Error(), colleagueUserID) {
		t.Fatalf("the refusal names the holder, which tells the caller who they are: %v", err)
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
		json.RawMessage(`{"url":"  `+address+`  "}`)); err != nil {
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

func TestChangingAnEndpointNobodyOpenedIsNotFound(t *testing.T) {
	t.Parallel()
	rt := newRuntime()
	rt.tx.noRows = map[int]bool{1: true}

	_, err := setEnabled(context.Background(), rt, json.RawMessage(`{"enabled":true}`))
	if !errors.Is(err, extension.ErrNotFound) {
		t.Fatalf("changing an endpoint that was never opened must be not-found, got %v", err)
	}
}
