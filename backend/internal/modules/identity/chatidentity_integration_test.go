// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// confirm starts a code for the member and sends it from account, as a company
// chat account would receive it.
func (e *revocationEnv) confirm(t *testing.T, member Identity, network ChatNetwork, scope, account string) {
	t.Helper()
	code, _, err := e.svc.StartChatVerification(e.wsCtx(member), network)
	if err != nil {
		t.Fatalf("starting a code: %v", err)
	}
	err = e.svc.db.Tx(e.wsCtx(e.admin), func(tx pgx.Tx) error {
		_, redeemed, err := RedeemChatCodeTx(e.wsCtx(e.admin), tx, network, scope, account, code)
		if err == nil && !redeemed {
			t.Fatal("the code was not accepted")
		}
		return err
	})
	if err != nil {
		t.Fatalf("redeeming the code: %v", err)
	}
}

func (e *revocationEnv) chatMember(t *testing.T, network ChatNetwork, account string) (ids.UUID, bool) {
	t.Helper()
	var member ids.UUID
	var found bool
	err := e.svc.db.Tx(e.wsCtx(e.admin), func(tx pgx.Tx) error {
		var err error
		member, found, err = ChatMemberTx(context.Background(), tx, network, "", account)
		return err
	})
	if err != nil {
		t.Fatalf("finding the member of %s: %v", account, err)
	}
	return member, found
}

func TestARetiredMembersChatAccountsAreFreedForASuccessor(t *testing.T) {
	e := setupRevocationEnv(t, "chat-retire")
	e.svc.WithMemberWork(noMemberWork{})
	leaver := e.seat(t, "leaver", "rep")
	if _, err := e.owner.Exec(context.Background(), `UPDATE app_user SET status = 'active' WHERE id = $1`, leaver.UserID); err != nil {
		t.Fatal(err)
	}
	e.confirm(t, leaver, ChatWhatsApp, "", "491701234567")
	if _, _, err := e.svc.StartChatVerification(e.wsCtx(leaver), ChatTelegram); err != nil {
		t.Fatal(err)
	}

	if _, err := e.owner.Exec(context.Background(), `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, leaver.UserID); err != nil {
		t.Fatal(err)
	}
	preview, err := e.svc.PreviewMemberRetirement(e.wsCtx(e.admin), e.admin, leaver.UserID, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blockers) > 0 {
		t.Fatalf("blockers: %+v", preview.Blockers)
	}
	if err := e.svc.RetireMember(e.wsCtx(e.admin), e.admin, leaver.UserID, nil, preview.Token, false); err != nil {
		t.Fatal(err)
	}

	var phone, hash *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT chat_whatsapp_phone, chat_telegram_code_hash FROM app_user WHERE id = $1`, leaver.UserID).Scan(&phone, &hash); err != nil {
		t.Fatal(err)
	}
	if phone != nil || hash != nil {
		t.Fatalf("the retired member kept a chat account (%v) or a code (%v)", phone, hash)
	}
	e.confirm(t, e.member, ChatWhatsApp, "", "491701234567")
	if got, found := e.chatMember(t, ChatWhatsApp, "491701234567"); !found || got != e.member.UserID.UUID {
		t.Fatalf("the account belongs to %v (found %v), want the successor", got, found)
	}
}

func TestAChatCodeIsDatedAndJudgedByTheDatabaseClock(t *testing.T) {
	e := setupRevocationEnv(t, "chat-clock")
	e.svc.now = func() time.Time { return time.Now().Add(-48 * time.Hour) }
	e.confirm(t, e.member, ChatTelegram, "", "7001")
	if _, found := e.chatMember(t, ChatTelegram, "7001"); !found {
		t.Fatal("a code was refused because the service clock ran two days behind the database")
	}
}
