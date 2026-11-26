// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A code sent from a member's chat account confirms it. A confirmed member's
// forward to a company chat account files the original author's message.
// Every case runs through capture.FileChatMessage over the sink
// production composes.

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/forwardexpiry"
	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pendingforward"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

var forwardBinding = connector.ChatBinding{
	Provider: "telegram", CapturedBy: "connector:telegram", KeyScope: "77", ThreadScope: "77",
	Ownership: connector.ChatOwnedByWorkspace,
}

var whatsappForwardBinding = connector.ChatBinding{
	Provider: "whatsapp", CapturedBy: "connector:whatsapp", KeyScope: "77", ThreadScope: "77",
	Ownership: connector.ChatOwnedByWorkspace,
}

func chatFrom(sender, id, body string, forward *connector.ChatForward) connector.ChatMessage {
	return connector.ChatMessage{
		ConversationID: sender, IdempotencyKey: id, Direction: connector.DirectionInbound,
		Counterparty: connector.ChatParty{UserID: sender, DisplayName: "Lars"},
		Body:         body, OccurredAt: chatChangeEpoch, Forward: forward,
	}
}

func forwardedMessage(id string, forward *connector.ChatForward) connector.ChatMessage {
	return chatFrom("5001", id, "price for 20 seats?", forward)
}

func verifyTelegram(t *testing.T, e *integration.SearchEnv, member ids.UUID, account string) {
	t.Helper()
	wsExec(t, e, `UPDATE app_user SET chat_telegram_user_id = $2, chat_telegram_verified_at = now() WHERE id = $1`, member, account)
}

// connectorCtx is the company account's ingest principal for the binding's provider.
func connectorCtx(e *integration.SearchEnv, binding connector.ChatBinding) context.Context {
	ctx := workspaceConnectorCtx(e)
	actor, _ := principal.Actor(ctx)
	actor.ID = binding.CapturedBy
	return principal.WithActor(ctx, actor)
}

func humanCtx(e *integration.SearchEnv, member ids.UUID) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + member.String(), UserID: member})
}

func TestAVerifiedMembersForwardWithAnAuthorFilesOnTheAuthorsContact(t *testing.T) {
	e := newCaptureEnv(t).e
	verifyTelegram(t, e, e.Rep1, "5001")
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
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

func TestAVerifiedWhatsAppMembersForwardWithNoAuthorIsHeldForThatMemberAlone(t *testing.T) {
	e := newCaptureEnv(t).e
	wsExec(t, e, `UPDATE app_user SET chat_whatsapp_phone = '491701234567', chat_whatsapp_verified_at = now() WHERE id = $1`, e.Rep1)
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
	msg := chatFrom("491701234567", "2", "price for 20 seats?", &connector.ChatForward{})
	ref, err := capturemod.FileChatMessage(connectorCtx(e, whatsappForwardBinding), sink, whatsappForwardBinding, msg)
	if err != nil {
		t.Fatal(err)
	}
	if n := wsCount(t, e, `SELECT count(*) FROM contact_channel_identity WHERE channel_user_id = '491701234567'`); n != 0 {
		t.Fatalf("the member got %d channel identities, want none", n)
	}
	if got := wsScalar(t, e, `SELECT audience || '/' || audience_reason FROM activity WHERE id = $1`, ref.ID); got != "participants/no_counterparty" {
		t.Fatalf("hold = %s", got)
	}
}

func TestAnUnverifiedAccountNeverRecognisesAMember(t *testing.T) {
	e := newCaptureEnv(t).e
	if _, _, err := identity.NewServiceFor(compose.InstallationDB(e.Pool)).StartChatVerification(humanCtx(e, e.Rep1), identity.ChatTelegram); err != nil {
		t.Fatal(err)
	}
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
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
	verifyTelegram(t, e, e.Rep1, "5001")
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
	fresh, err := capturemod.FileChatMessage(workspaceConnectorCtx(e), sink, forwardBinding, forwardedMessage("4", &connector.ChatForward{}))
	if err != nil {
		t.Fatal(err)
	}
	old, err := capturemod.FileChatMessage(workspaceConnectorCtx(e), sink, forwardBinding, forwardedMessage("5", &connector.ChatForward{}))
	if err != nil {
		t.Fatal(err)
	}
	wsExec(t, e, `UPDATE activity SET created_at = now() - $2::interval WHERE id = $1`, old.ID, (pendingforward.Window + time.Hour).String())
	purger := compose.NewCapturePurger(e.Pool, compose.NewRetentionServiceFor(compose.InstallationDB(e.Pool), nil, slog.Default()))
	if err := forwardexpiry.Run(principal.SystemActing(workspaceConnectorCtx(e), "system:test-forward-expiry"), e.Pool, purger); err != nil {
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

// codeCase is one network's way of sending a code: the binding, the sender and
// the column that holds the verified account.
type codeCase struct {
	network  identity.ChatNetwork
	binding  connector.ChatBinding
	sender   string
	verified string
}

var codeCases = []codeCase{
	{identity.ChatTelegram, forwardBinding, "5001", `chat_telegram_user_id`},
	{identity.ChatWhatsApp, whatsappForwardBinding, "491701234567", `chat_whatsapp_phone`},
	{identity.ChatZalo, connector.ChatBinding{
		Provider: "zalo_oa", CapturedBy: "connector:zalo_oa", KeyScope: "88", ThreadScope: "88",
		AccountScope: "88", Ownership: connector.ChatOwnedByWorkspace,
	}, "zu-1", `chat_zalo_user_id`},
}

func TestACodeSentFromAnAccountVerifiesItForTheMemberWhoAskedAndFilesNothing(t *testing.T) {
	for _, c := range codeCases {
		t.Run(string(c.network), func(t *testing.T) {
			e := newCaptureEnv(t).e
			svc := identity.NewServiceFor(compose.InstallationDB(e.Pool))
			code, _, err := svc.StartChatVerification(humanCtx(e, e.Rep1), c.network)
			if err != nil {
				t.Fatal(err)
			}
			sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
			if _, err := capturemod.FileChatMessage(connectorCtx(e, c.binding), sink, c.binding, chatFrom(c.sender, "wrong", "ABCDEFGH", nil)); err != nil {
				t.Fatalf("a message that is no code: %v", err)
			}
			if n := wsCount(t, e, `SELECT count(*) FROM app_user WHERE `+c.verified+` IS NOT NULL`); n != 0 {
				t.Fatal("a wrong code verified an account")
			}
			before := wsCount(t, e, `SELECT count(*) FROM activity WHERE kind = 'message'`)
			_, err = capturemod.FileChatMessage(connectorCtx(e, c.binding), sink, c.binding, chatFrom(c.sender, "code", " "+code+" ", nil))
			if !errors.Is(err, capturemod.ErrChatCodeRedeemed) {
				t.Fatalf("filing the code = %v, want it redeemed", err)
			}
			if n := wsCount(t, e, `SELECT count(*) FROM app_user WHERE id = $1 AND `+c.verified+` IS NOT NULL`, e.Rep1); n != 1 {
				t.Fatal("the right code did not verify the account")
			}
			if after := wsCount(t, e, `SELECT count(*) FROM activity WHERE kind = 'message'`); after != before {
				t.Fatal("the code was filed as a message")
			}
			mine, err := svc.MyChatIdentities(humanCtx(e, e.Rep1))
			if err != nil || len(mine) != 1 || mine[0].Network != c.network {
				t.Fatalf("identities = %+v, %v", mine, err)
			}
			if err := svc.RemoveChatIdentity(humanCtx(e, e.Rep1), c.network); err != nil {
				t.Fatal(err)
			}
			if n := wsCount(t, e, `SELECT count(*) FROM app_user WHERE `+c.verified+` IS NOT NULL`); n != 0 {
				t.Fatal("the identity was not removed")
			}
		})
	}
}

func TestAnExpiredCodeAndATakenAccountVerifyNothing(t *testing.T) {
	e := newCaptureEnv(t).e
	svc := identity.NewServiceFor(compose.InstallationDB(e.Pool))
	sink := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})
	code, _, err := svc.StartChatVerification(humanCtx(e, e.Rep1), identity.ChatTelegram)
	if err != nil {
		t.Fatal(err)
	}
	wsExec(t, e, `UPDATE app_user SET chat_telegram_code_expires_at = now() - interval '1 minute' WHERE id = $1`, e.Rep1)
	if _, err := capturemod.FileChatMessage(workspaceConnectorCtx(e), sink, forwardBinding, chatFrom("5001", "late", code, nil)); err != nil {
		t.Fatalf("an expired code is an ordinary message: %v", err)
	}
	if n := wsCount(t, e, `SELECT count(*) FROM app_user WHERE chat_telegram_user_id IS NOT NULL`); n != 0 {
		t.Fatal("an expired code verified an account")
	}
	verifyTelegram(t, e, e.Rep1, "5001")
	second, _, err := svc.StartChatVerification(humanCtx(e, e.Rep3), identity.ChatTelegram)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := capturemod.FileChatMessage(workspaceConnectorCtx(e), sink, forwardBinding, chatFrom("5001", "taken", second, nil)); !errors.Is(err, capturemod.ErrChatCodeRedeemed) {
		t.Fatalf("the code is consumed, not filed: %v", err)
	}
	if n := wsCount(t, e, `SELECT count(*) FROM app_user WHERE id = $1 AND chat_telegram_user_id IS NOT NULL`, e.Rep3); n != 0 {
		t.Fatal("an account another member holds was verified a second time")
	}
}
