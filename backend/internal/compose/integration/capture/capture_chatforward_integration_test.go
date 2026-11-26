// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A member's forward to a company chat account, filed through
// capture.FileChatMessage over the sink production composes.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pendingforward"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// membersByAccount stands in for a network whose members are verified in a store
// the product owner has not approved yet.
type membersByAccount map[string]ids.UUID

func (m membersByAccount) MemberOf(_ context.Context, _ pgx.Tx, _, account string) (ids.UUID, bool, error) {
	id, ok := m[account]
	return id, ok, nil
}

var forwardBinding = connector.ChatBinding{
	Provider: "telegram", CapturedBy: "connector:telegram", KeyScope: "77", ThreadScope: "77",
	Ownership: connector.ChatOwnedByWorkspace,
}

func forwardedMessage(id string, forward *connector.ChatForward) connector.ChatMessage {
	return connector.ChatMessage{
		ConversationID: "5001", IdempotencyKey: id, Direction: connector.DirectionInbound,
		Counterparty: connector.ChatParty{UserID: "5001", DisplayName: "Lars"},
		Body:         "price for 20 seats?", OccurredAt: chatChangeEpoch, Forward: forward,
	}
}

func TestAMembersForwardWithAnAuthorFilesOnTheAuthorsContact(t *testing.T) {
	e := newCaptureEnv(t).e
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{}).WithMemberIdentities(membersByAccount{"5001": e.Rep1})
	origin := chatChangeEpoch.Add(-3 * time.Hour)
	forward := &connector.ChatForward{Author: &connector.ChatParty{UserID: "6002", DisplayName: "Anna Weber"}, OriginalAt: origin}
	ref, err := capturemod.FileChatMessage(workspaceConnectorCtx(e), sink, forwardBinding, forwardedMessage("1", forward))
	if err != nil {
		t.Fatal(err)
	}
	if n := wsCount(t, e, `SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = '5001'`); n != 0 {
		t.Fatalf("the member got %d channel identities, want none", n)
	}
	if n := wsCount(t, e, `SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = '6002'`); n != 1 {
		t.Fatalf("the author has %d channel identities, want 1", n)
	}
	if n := wsCount(t, e, `SELECT count(*) FROM activity_link WHERE activity_id = $1`, ref.ID); n != 1 {
		t.Fatalf("the message has %d links, want the author's contact", n)
	}
	if got := wsScalar(t, e, `SELECT (occurred_at = $2 AND direction = 'inbound' AND captured_by LIKE '%' || $3::text)::text FROM activity WHERE id = $1`, ref.ID, origin, e.Rep1.String()); got != "true" {
		t.Fatalf("original time, direction and submitter = %s", got)
	}
}

func TestAMembersForwardWithNoAuthorIsHeldForThatMemberAlone(t *testing.T) {
	e := newCaptureEnv(t).e
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{}).WithMemberIdentities(membersByAccount{"5001": e.Rep1})
	ref, err := capturemod.FileChatMessage(workspaceConnectorCtx(e), sink, forwardBinding, forwardedMessage("2", &connector.ChatForward{}))
	if err != nil {
		t.Fatal(err)
	}
	if n := wsCount(t, e, `SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = '5001'`); n != 0 {
		t.Fatalf("the member got %d channel identities, want none", n)
	}
	if got := wsScalar(t, e, `SELECT audience || '/' || audience_reason FROM activity WHERE id = $1`, ref.ID); got != "participants/no_counterparty" {
		t.Fatalf("hold = %s", got)
	}
}

func TestANonMembersForwardIsOrdinaryCorrespondence(t *testing.T) {
	e := newCaptureEnv(t).e
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{}).WithMemberIdentities(membersByAccount{})
	forward := &connector.ChatForward{Author: &connector.ChatParty{UserID: "6002", DisplayName: "Anna Weber"}}
	if _, err := capturemod.FileChatMessage(workspaceConnectorCtx(e), sink, forwardBinding, forwardedMessage("3", forward)); err != nil {
		t.Fatal(err)
	}
	if n := wsCount(t, e, `SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = '5001'`); n != 1 {
		t.Fatalf("the sender has %d channel identities, want 1", n)
	}
	if n := wsCount(t, e, `SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = '6002'`); n != 0 {
		t.Fatalf("the named author got %d channel identities, want none", n)
	}
}

func TestAPendingForwardExpiresWithItsRawRow(t *testing.T) {
	e := newCaptureEnv(t).e
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{}).WithMemberIdentities(membersByAccount{"5001": e.Rep1})
	fresh, err := capturemod.FileChatMessage(workspaceConnectorCtx(e), sink, forwardBinding, forwardedMessage("4", &connector.ChatForward{}))
	if err != nil {
		t.Fatal(err)
	}
	old, err := capturemod.FileChatMessage(workspaceConnectorCtx(e), sink, forwardBinding, forwardedMessage("5", &connector.ChatForward{}))
	if err != nil {
		t.Fatal(err)
	}
	wsExec(t, e, `UPDATE activity SET created_at = now() - $2::interval WHERE id = $1`, old.ID, (pendingforward.Window + time.Hour).String())
	svc := compose.NewRetentionServiceFor(compose.InstallationDB(e.Pool), nil, slog.Default())
	if err := svc.EvaluateInstallation(integration.RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}
	if n := wsCount(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND body IS NOT NULL`, fresh.ID); n != 1 {
		t.Fatalf("the fresh forward lost its text")
	}
	if n := wsCount(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND body IS NOT NULL`, old.ID); n != 0 {
		t.Fatalf("the expired forward kept its text")
	}
	if n := wsCount(t, e, `SELECT count(*) FROM raw_capture r JOIN activity a ON a.raw_capture_id = r.id WHERE a.id = $1`, old.ID); n != 0 {
		t.Fatalf("the expired forward kept its raw row")
	}
}
