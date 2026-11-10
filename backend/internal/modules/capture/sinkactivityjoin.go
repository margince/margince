// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// joinedCapture names the two answers a joining capture carries, so neither
// reads as a bare true at the call.
type joinedCapture struct {
	memberBound  bool
	alreadyFiled bool
}

// finishJoinedActivity completes a capture that landed on an activity already
// filed. Another mailbox, this seat's own earlier sync and a replay of the same
// message all reach it.
//
// The counterpart of finishNewActivity. With both, captureActivity reads as the
// three answers a capture can have: the row was already here, the row is new,
// or the capture failed.
func (s *Sink) finishJoinedActivity(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID,
	rec connector.NormalizedRecord, fields ActivityFields, birth birthDecision,
	joined joinedCapture,
) error {
	// Before this seat's own import and participant rows. The claim asks who
	// the stored row names as sender, and this capture is about to add itself.
	if err := s.claimOwnSentMailTx(ctx, tx, id, rec); err != nil {
		return err
	}
	if err := s.recordThisImport(ctx, tx, id, rec, fields, birth, joined.memberBound); err != nil {
		return err
	}
	if err := s.joinThread(ctx, tx, id, rec, false); err != nil {
		return err
	}
	if err := s.settleMeetingReplay(ctx, tx, id, rec, joined.alreadyFiled); err != nil {
		return err
	}
	return s.adoptWithheldParts(ctx, tx, id, rec, fields)
}
