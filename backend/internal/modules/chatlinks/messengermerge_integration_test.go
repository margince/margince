// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatlinks

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (c *receiptClient) OpenPeople(ctx context.Context, _ []string) (messaging.Conversation, error) {
	return c.Conversation(ctx, "ROOM")
}

func TestMessengerMergeKeepsTheSurvivorsRoomAndUncertainReceipt(t *testing.T) {
	for _, kind := range []string{RecordCompany, RecordDeal} {
		t.Run(kind, func(t *testing.T) {
			client := &receiptClient{sendErr: messaging.ErrDeliveryUnknown}
			e, in := setupMessengerSend(t, client)
			ctx := e.seatWith(e.rep, principal.RowScopeOwn, map[string]principal.ObjectGrant{RecordCompany: {Read: true, Update: true}, RecordDeal: {Read: true, Update: true}, RecordProject: {Read: true, Update: true}})
			from, to := e.deal, e.second
			if kind == RecordCompany {
				from, to = e.company, ids.NewV7()
				e.exec(t, `INSERT INTO company(id,display_name,owner_id,source,captured_by) VALUES($1,'Survivor',$2,'manual','system')`, to, e.rep)
			}
			for _, record := range []ids.UUID{from, to} {
				if _, err := e.store.MessengerOpenMembers(ctx, kind, record, "slack", []string{"COLLEAGUE"}); err != nil {
					t.Fatal(err)
				}
			}
			original, err := e.store.SendMessenger(ctx, kind, from, "slack", in)
			if err != nil || original.State != "unknown" {
				t.Fatalf("initial send: %+v %v", original, err)
			}
			err = e.store.db.Tx(ctx, func(tx pgx.Tx) error {
				if kind == RecordCompany {
					return MoveCompanyRefsTx(ctx, tx, from, to)
				}
				_, err := MoveDealRefsTx(ctx, tx, from, to)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := e.count(t, `SELECT count(*) FROM chat_member_room WHERE record_type=$1 AND record_id=$2`, kind, to); got != 1 {
				t.Fatalf("survivor rooms=%d", got)
			}
			moved, err := e.store.MessengerAttempt(ctx, kind, to, "slack", in.RequestID)
			if err != nil || moved.ID != original.ID || moved.contentHash != original.contentHash || moved.State != "unknown" {
				t.Fatalf("moved receipt: %+v %v", moved, err)
			}
			if _, err := e.store.SendMessenger(ctx, kind, to, "slack", in); !errors.Is(err, apperrors.ErrConflict) {
				t.Fatalf("relocated request was resent: %v", err)
			}
			room, err := e.store.MessengerConversation(ctx, kind, to, "slack", "ROOM")
			if err != nil || room.Unresolved == nil || room.Unresolved.RequestID != in.RequestID {
				t.Fatalf("unknown send hidden after merge: %+v %v", room, err)
			}
			in.RequestID = ids.NewV7()
			if _, err := e.store.SendMessenger(ctx, kind, to, "slack", in); !errors.Is(err, ErrMessengerDeliveryNeedsReview) || client.sends.Load() != 1 {
				t.Fatalf("merge bypassed delivery review: %v sends=%d", err, client.sends.Load())
			}
		})
	}
}

func TestDeletedRecordKeepsDeliveryEvidenceButCannotExposeItsRoomOrReceipt(t *testing.T) {
	client := &receiptClient{sendErr: messaging.ErrDeliveryUnknown}
	e, in := setupMessengerSend(t, client)
	ctx := e.seat(e.rep)
	if _, err := e.store.MessengerOpenMembers(ctx, RecordDeal, e.second, "slack", []string{"COLLEAGUE"}); err != nil {
		t.Fatal(err)
	}
	receipt, err := e.store.SendMessenger(ctx, RecordDeal, e.second, "slack", in)
	if err != nil || receipt.State != "unknown" {
		t.Fatalf("send=%+v %v", receipt, err)
	}
	e.exec(t, `DELETE FROM deal WHERE id=$1`, e.second)
	if got := e.count(t, `SELECT count(*) FROM chat_send_attempt WHERE id=$1`, receipt.ID); got != 1 {
		t.Fatal("record deletion erased delivery uncertainty")
	}
	if got := e.count(t, `SELECT count(*) FROM chat_member_room WHERE record_id=$1`, e.second); got != 1 {
		t.Fatal("record deletion erased the grant-owned room")
	}
	if _, err := e.store.MessengerAttempt(ctx, RecordDeal, e.second, "slack", in.RequestID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("deleted record receipt: %v", err)
	}
	if _, err := e.store.MessengerOpenMembers(ctx, RecordDeal, e.second, "slack", []string{"COLLEAGUE"}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("deleted record room: %v", err)
	}
}
