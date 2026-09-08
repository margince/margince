// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package testmailbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// echoedRecord is what Normalize re-parses from Raw — the inverse of what
// Sync writes below, exactly like offlinedemo's message/record() pair.
type echoedRecord struct {
	MessageID string    `json:"message_id"`
	To        []string  `json:"to"`
	Subject   string    `json:"subject"`
	SentAt    time.Time `json:"sent_at"`
}

func (e echoedRecord) record() connector.NormalizedRecord {
	raw, _ := json.Marshal(e) //nolint:errchkjson // a struct of strings and a time cannot fail to marshal

	// This is a real send test_mailbox itself performed, filed back exactly
	// like a provider files a copy into Sent — both halves of
	// WithOwnerAttestation are genuinely true here (Direction independently
	// names the owner as author, and this Sync IS the provider filing it),
	// unlike offlinedemo's synthetic correspondence, which nobody actually
	// sent and so may never attest ownership.
	counterparty := connector.Counterparty{Direction: connector.DirectionOutbound}
	if len(e.To) > 0 {
		counterparty.Email = e.To[0]
	}
	counterparty = counterparty.WithOwnerAttestation(true)

	return connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{
			SourceSystem: connector.EmailSourceSystem,
			SourceID:     e.MessageID,
		},
		Fields: capture.ActivityFields{
			Kind:       "email",
			Subject:    e.Subject,
			OccurredAt: e.SentAt,
			Direction:  connector.DirectionOutbound,
		},
		Source:       Name + ":" + e.MessageID,
		CapturedBy:   "connector:" + Name,
		Raw:          raw,
		ThreadKey:    e.MessageID,
		Addresses:    e.To,
		Counterparty: counterparty,
	}
}

// Sync files back every send this seat has made that its own ledger has not
// yet echoed. There is no external provider to poll — the ledger IS the
// "provider" this connector's own SendEmail already wrote to — so this is
// not incremental in the cursor sense; it always asks for everything
// unechoed and lets the ledger's echoed_at column be the watermark.
func (c *Connector) Sync(ctx context.Context, auth connector.Auth, cursor connector.Cursor, sink connector.Sink) (connector.Cursor, error) {
	userID, err := readUserID(auth)
	if err != nil {
		return cursor, err
	}
	pending, err := c.ledger.Unechoed(ctx, userID)
	if err != nil {
		return cursor, fmt.Errorf("test_mailbox: reading unechoed sends: %w", err)
	}
	for _, msg := range pending { // msg is a capture.SentMessage
		rec := echoedRecord{MessageID: msg.MessageID, To: msg.To, Subject: msg.Subject, SentAt: msg.SentAt}.record()
		if _, err := sink.Upsert(ctx, rec); err != nil {
			return cursor, fmt.Errorf("test_mailbox: echoing %s: %w", msg.MessageID, err)
		}
		if err := c.ledger.MarkEchoed(ctx, msg.ID); err != nil {
			return cursor, fmt.Errorf("test_mailbox: marking %s echoed: %w", msg.MessageID, err)
		}
	}
	return cursor, nil
}

// Normalize re-parses one stored echo. The sync writes its own JSON into Raw,
// so this is the inverse of that and nothing more, exactly like offlinedemo.
func (c *Connector) Normalize(_ context.Context, raw connector.RawRecord) ([]connector.NormalizedRecord, error) {
	var e echoedRecord
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, fmt.Errorf("test_mailbox: re-reading an echoed message: %w", err)
	}
	return []connector.NormalizedRecord{e.record()}, nil
}

var _ connector.Connector = (*Connector)(nil)
