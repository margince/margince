// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Parent identity and subject boundaries define a conversation independently of arrival order.

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ThreadJoiner connects capture to the activity owner and the cross-module thread merge.
type ThreadJoiner struct {
	Repair       func(context.Context, pgx.Tx, int, ids.UUID, string) (int, int, bool, error)
	InheritSeat  func(context.Context, pgx.Tx, string, string, ids.UUID) error
	InheritBasis func(context.Context, pgx.Tx, ids.UUID, string, string) error
	// SentBySeat proves which neighbours this seat sent before a mailbox echo exists.
	SentBySeat func(ctx context.Context, tx pgx.Tx, seat ids.UUID, candidates []ids.ActivityID) ([]ids.ActivityID, error)
	// RecordReferences stores the ids one email says it answers.
	RecordReferences func(ctx context.Context, tx pgx.Tx, id ids.ActivityID, referenced []string) error
}

func (j ThreadJoiner) complete() bool {
	return j.RecordReferences != nil && j.Repair != nil && j.InheritSeat != nil && j.InheritBasis != nil
}

// WithThreadJoin returns a copy that joins the threads each captured email
// links. A sink without it threads on the References root alone, as before.
func (s *Sink) WithThreadJoin(j ThreadJoiner) *Sink {
	c := *s
	c.threadJoin = j
	return &c
}

// joinThread uses the same persisted partition engine as historical correction.
func (s *Sink) joinThread(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, created bool,
) error {
	if !s.threadJoin.complete() || rec.NaturalKey.SourceSystem != connector.EmailSourceSystem {
		return nil
	}
	live, err := threadSourceLiveTx(ctx, tx, id)
	if err != nil || !live {
		return err
	}
	if err := s.threadJoin.RecordReferences(ctx, tx, id, rec.ReplyTo); err != nil {
		return err
	}
	seat := actorUserID(ctx)
	if seat == ids.Nil {
		return nil
	}
	held, err := seatHoldsTx(ctx, tx, seat, id)
	if err != nil || !held {
		return err
	}
	if err := s.rememberThreadTopicTx(ctx, tx, id); err != nil {
		return err
	}
	var key string
	if err := tx.QueryRow(ctx, `SELECT coalesce(thread_key,'') FROM activity WHERE id=@id`, pgx.NamedArgs{"id": id}).Scan(&key); err != nil {
		return err
	}
	changed, err := rememberThreadInputTx(ctx, tx, id, seat)
	if err != nil {
		return err
	}
	if created || changed {
		if err := QueueThreadRepairTx(ctx, tx, key); err != nil {
			return err
		}
	}

	// Last, because it asks what thread this message is in and only the merge
	// above knows the answer.
	return s.holdFromJoinedThreadTx(ctx, tx, id)
}

// Capture takes this workspace lock before any activity row; repair uses the same order.
const mergeLockStatement = `SELECT pg_advisory_xact_lock(hashtextextended('thread_merge', 0))`

func seatHoldsTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, id ids.ActivityID) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capture_import WHERE activity_id=@id AND user_id=@seat)`, pgx.NamedArgs{"id": id, threadArgSeat: seat}).Scan(&held)
	return held, err
}

func (s *Sink) rememberThreadTopicTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	live, err := threadSourceLiveTx(ctx, tx, id)
	if err != nil || !live {
		return err
	}
	var subject string
	if err := tx.QueryRow(ctx, `SELECT coalesce(subject,'') FROM activity WHERE id=@id`, pgx.NamedArgs{"id": id}).Scan(&subject); err != nil {
		return err
	}
	keys, err := s.threadProjectKeysTx(ctx, tx, subject)
	if err != nil {
		return err
	}
	prior, err := threadHistoricalProjectKeysTx(ctx, tx, id)
	if err != nil {
		return err
	}
	keys = append(keys, prior...)
	slices.Sort(keys)
	keys = slices.Compact(keys)
	topic := threadBoundaryTopic(subject, keys)
	_, err = tx.Exec(ctx, `INSERT INTO capture_thread_topic(activity_id,own_topic,recognized_keys,subject_hash,effective_topic,root_key) SELECT @id,@topic,@keys,md5(coalesce(subject,'')),@topic,thread_key FROM activity WHERE id=@id ON CONFLICT(activity_id) DO UPDATE SET own_topic=EXCLUDED.own_topic,recognized_keys=EXCLUDED.recognized_keys,subject_hash=EXCLUDED.subject_hash`, pgx.NamedArgs{"id": id, threadArgTopic: topic, "keys": keys})
	return err
}

func (s *Sink) finishCapturedThreadTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord) error {
	if !s.threadJoin.complete() || rec.NaturalKey.SourceSystem != connector.EmailSourceSystem {
		return nil
	}
	live, err := threadSourceLiveTx(ctx, tx, id)
	if err != nil || !live {
		return err
	}

	run, traced := principal.CorrelationID(ctx)
	if !traced {
		run = ids.NewV7()
	}
	var key string
	if err := tx.QueryRow(ctx, `SELECT coalesce(thread_key,'') FROM activity WHERE id=@id`, pgx.NamedArgs{"id": id}).Scan(&key); err != nil {
		return err
	}
	if _, _, _, err := s.threadJoin.Repair(ctx, tx, 256, run, key); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_pending p SET blocked=true FROM activity a WHERE a.id=@id AND p.source_key=a.thread_key`, pgx.NamedArgs{"id": id})
	return err
}
