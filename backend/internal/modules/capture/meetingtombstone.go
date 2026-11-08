// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A provider tombstone can name nothing but the mailbox's own event id. When a
// colleague's calendar filed the meeting first, no row carries that id. The
// tombstone holds no iCal UID to find the shared meeting by either. The seat's
// own stored original states both, so the removal is resolved through it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// StoredEventIdentity reads the cross-door identity one stored calendar
// original states. Each calendar connector decodes its own JSON, and capture
// imports none of them, so compose injects the reader.
type StoredEventIdentity func(source string, raw []byte) (connector.CrossDoorIdentity, bool, error)

// WithStoredEventIdentity returns a copy that resolves a tombstone naming only
// this mailbox's event id through the original that mailbox delivered.
func (s *Sink) WithStoredEventIdentity(read StoredEventIdentity) *Sink {
	out := *s
	out.storedEventIdentity = read
	return &out
}

// meetingOfStoredOriginal finds the meeting a tombstone's own event id was
// captured onto. It answers a meeting only when this seat imported it, so an
// event id that names somebody else's original reaches nothing.
func (s *Sink) meetingOfStoredOriginal(ctx context.Context, tx pgx.Tx, key connector.NaturalKey) (ids.ActivityID, bool, error) {
	if s.storedEventIdentity == nil {
		return ids.ActivityID{}, false, nil
	}
	if may, err := s.identityMayCancel(ctx, tx, key); err != nil || !may {
		return ids.ActivityID{}, false, err
	}
	var payload []byte
	err := tx.QueryRow(ctx, `SELECT payload FROM raw_capture WHERE source_system=$1 AND source_id=$2`,
		key.SourceSystem, key.SourceID).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ActivityID{}, false, nil
	}
	if err != nil {
		return ids.ActivityID{}, false, fmt.Errorf("capture: reading the original a removed event was captured from: %w", err)
	}
	identity, stated := storedIdentityOf(s.storedEventIdentity, key.SourceSystem, payload)
	if !stated {
		return ids.ActivityID{}, false, nil
	}
	matched, found, err := s.activityHoldingIdentity(ctx, tx, connector.NormalizedRecord{NaturalKey: key, CrossDoorIdentity: identity})
	if err != nil || !found {
		return ids.ActivityID{}, false, err
	}
	var own ids.ActivityID
	err = tx.QueryRow(ctx, `SELECT a.id FROM activity a WHERE a.id=$1 AND a.kind='meeting'
 AND a.archived_at IS NULL AND a.restricted_at IS NULL
 AND EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id=a.id AND ci.user_id=$2) FOR UPDATE OF a`,
		matched, actorUserID(ctx)).Scan(&own)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ActivityID{}, false, nil
	}
	if err != nil {
		return ids.ActivityID{}, false, fmt.Errorf("capture: locking the meeting a removed event was captured onto: %w", err)
	}
	return own, true, nil
}

// storedIdentityOf reads the identity a stored original states. An erased or
// unreadable original states none, and the removal then writes nothing.
func storedIdentityOf(read StoredEventIdentity, source string, payload []byte) (connector.CrossDoorIdentity, bool) {
	if len(payload) == 0 {
		return connector.CrossDoorIdentity{}, false
	}
	raw, err := DecodeStoredOriginal(payload)
	if err != nil {
		return connector.CrossDoorIdentity{}, false
	}
	identity, _, err := read(source, raw)
	if err != nil {
		return connector.CrossDoorIdentity{}, false
	}
	return identity, identity.Stated()
}
