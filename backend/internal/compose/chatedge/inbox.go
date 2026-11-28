// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatedge

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// TxInserter queues a job in the caller's transaction.
type TxInserter interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error
}

// Message is one verified message of a connected account, ready to be stored.
type Message struct {
	// Connection is the live connection the message was delivered to.
	Connection ids.UUID
	// Customers are the identities the message names for the customer. An erasure
	// of any of them drops the message.
	Customers []connector.ChannelIdentity
	// SourceSystem and SourceID are the raw row's redelivery key.
	SourceSystem, SourceID string
	// Payload is the verbatim provider payload.
	Payload []byte
}

// Inbox stores messages and queues the job that files each.
type Inbox struct {
	DB       *database.DB
	Inserter TxInserter
	// Job builds the ingest job of one stored message.
	Job         func(workspace, connection, rawCapture ids.UUID) river.JobArgs
	MaxAttempts int
}

// Store writes every message's raw row and its ingest job in one transaction. A
// 200 means all of them are durable, and a failure leaves none for the
// provider's retry to double. A redelivered message refreshes its row and finds
// its job queued.
//
// Each connection is checked live inside the transaction, because the read that
// verified the delivery came before it, and a disconnected one stores nothing. An
// erased customer is dropped. The identities are locked first, in the eraser's
// order, so no erasure commits between the check and the insert.
func (in *Inbox) Store(ctx context.Context, messages []Message) (stored, dropped int, err error) {
	if len(messages) == 0 {
		return 0, 0, nil
	}
	company, err := in.DB.Workspace(ctx)
	if err != nil {
		return 0, 0, err
	}
	ws := company.UUID
	err = in.DB.Tx(ctx, func(tx pgx.Tx) error {
		stored, dropped = 0, 0
		live, err := lockLive(ctx, tx, messages)
		if err != nil {
			return err
		}
		if err := storekit.LockChannelIdentities(ctx, tx, lockKeys(messages)); err != nil {
			return err
		}
		for _, m := range messages {
			erased := false
			if live[m.Connection] {
				if erased, err = suppressed(ctx, tx, m); err != nil {
					return err
				}
			}
			if !live[m.Connection] || erased {
				dropped++
				continue
			}
			if err := in.insert(ctx, tx, ws, m); err != nil {
				return err
			}
			stored++
		}
		return nil
	})
	return stored, dropped, err
}

func (in *Inbox) insert(ctx context.Context, tx pgx.Tx, ws ids.UUID, m Message) error {
	raw, err := capture.InsertRawCaptureTx(ctx, tx, capture.RawRecord{
		SourceSystem: m.SourceSystem, SourceID: m.SourceID, Payload: m.Payload, Subjects: subjectKeys(m),
	})
	if err != nil {
		return err
	}
	return in.Inserter.EnqueueTx(ctx, tx, in.Job(ws, m.Connection, raw), &river.InsertOpts{
		MaxAttempts: in.MaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	})
}

// lockLive locks each message's connection and reports which are still live.
func lockLive(ctx context.Context, tx pgx.Tx, messages []Message) (map[ids.UUID]bool, error) {
	live := map[ids.UUID]bool{}
	for _, m := range messages {
		if _, seen := live[m.Connection]; seen {
			continue
		}
		_, isLive, err := capture.LockLiveChatLinkTx(ctx, tx, m.Connection)
		if err != nil {
			return nil, err
		}
		live[m.Connection] = isLive
	}
	return live, nil
}

// suppressionKeys names the identities of a message as the suppression list keys them.
func suppressionKeys(m Message) []storekit.ChannelIdentityKey {
	keys := make([]storekit.ChannelIdentityKey, 0, len(m.Customers))
	for _, id := range m.Customers {
		keys = append(keys, storekit.ChannelIdentityKey{
			Provider: id.Provider, ChannelUserID: storekit.ChannelSuppressionAccount(id.AccountScope, id.ChannelUserID),
		})
	}
	return keys
}

func lockKeys(messages []Message) []storekit.ChannelIdentityKey {
	var keys []storekit.ChannelIdentityKey
	for _, m := range messages {
		keys = append(keys, suppressionKeys(m)...)
	}
	return keys
}

// subjectKeys is what the raw row records about whose message it is. An erasure
// or access request finds the row by these identities and never reads its payload.
func subjectKeys(m Message) []string {
	out := make([]string, 0, len(m.Customers))
	for _, id := range m.Customers {
		out = append(out, storekit.ChannelRawSubjectKey(id.Provider, id.AccountScope, id.ChannelUserID))
	}
	return out
}

func suppressed(ctx context.Context, tx pgx.Tx, m Message) (bool, error) {
	for _, key := range suppressionKeys(m) {
		erased, err := storekit.ChannelIdentitySuppressed(ctx, tx, key.Provider, key.ChannelUserID)
		if err != nil || erased {
			return erased, err
		}
	}
	return false, nil
}
