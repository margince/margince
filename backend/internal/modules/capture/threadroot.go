// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func (s *Sink) threadTopicTx(ctx context.Context, tx pgx.Tx, subject string) (string, error) {
	keys, err := s.threadProjectKeysTx(ctx, tx, subject)
	return threadBoundaryTopic(subject, keys), err
}

func (s *Sink) threadProjectKeysTx(ctx context.Context, tx pgx.Tx, subject string) ([]string, error) {
	keys := []string{}
	if s.projectKeys != nil {
		for _, token := range projectKeyCandidates(subject) {
			project, err := s.projectKeys.MatchProjectKey(ctx, tx, []string{token})
			if errors.Is(err, apperrors.ErrPermissionDenied) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if !project.IsZero() {
				keys = append(keys, token)
			}
		}
	}
	return keys, nil
}

func (s *Sink) rootThreadTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, rec connector.NormalizedRecord, fields ActivityFields) (connector.NormalizedRecord, error) {
	if !s.threadJoin.complete() || rec.NaturalKey.SourceSystem != connector.EmailSourceSystem || rec.NaturalKey.SourceID == "" {
		return rec, nil
	}
	topic, err := s.threadTopicTx(ctx, tx, fields.Subject)
	if err != nil {
		return rec, err
	}
	rec.ThreadKey = rec.NaturalKey.SourceID
	if len(rec.ReplyTo) == 0 {
		return rec, nil
	}
	parent, key, found, err := s.heldThreadParentTx(ctx, tx, seat, rec.ReplyTo[0])
	if err != nil || !found {
		return rec, err
	}
	parentTopic, available, err := parentThreadTopicTx(ctx, tx, parent)
	if err != nil {
		return rec, err
	}
	if available && (topic == parentTopic || topic == "") {
		rec.ThreadKey = key
	}
	if rec.ThreadKey != key {
		if err := s.threadJoin.InheritSeat(ctx, tx, key, rec.ThreadKey, seat); err != nil {
			return rec, err
		}
		if err := s.inheritThreadBirthBasesTx(ctx, tx, key, rec.ThreadKey); err != nil {
			return rec, err
		}
	}
	return rec, nil
}

func parentThreadTopicTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (string, bool, error) {
	var topic *string
	var subject string
	var available bool
	err := tx.QueryRow(ctx, `SELECT CASE WHEN t.subject_hash=md5(coalesce(a.subject,'')) THEN t.effective_topic END,coalesce(a.subject,''),activity_thread_available(a.thread_key) FROM activity a LEFT JOIN capture_thread_topic t ON t.activity_id=a.id WHERE a.id=@id`, pgx.NamedArgs{"id": id}).Scan(&topic, &subject, &available)
	if err != nil {
		return "", false, err
	}
	if topic != nil {
		return *topic, available, nil
	}
	keys, err := threadHistoricalProjectKeysTx(ctx, tx, id)
	if err != nil {
		return "", false, err
	}
	return threadBoundaryTopic(subject, keys), available, nil
}

func (s *Sink) inheritThreadBirthBasesTx(ctx context.Context, tx pgx.Tx, from, to string) error {
	var after ids.UUID
	for range 64 {
		var basis ids.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM communication_basis WHERE thread_key=@source AND id>@after AND revoked_at IS NULL ORDER BY id LIMIT 1`, pgx.NamedArgs{threadArgSource: from, threadArgAfter: after}).Scan(&basis)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.threadJoin.InheritBasis(ctx, tx, basis, from, to); err != nil {
			return err
		}
		after = basis
	}
	// Remaining evidence is inherited by the same bounded repair before publication.
	if err := QueueThreadRepairTx(ctx, tx, from); err != nil {
		return err
	}
	return QueueThreadRepairTx(ctx, tx, to)
}

func (s *Sink) heldThreadParentTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, reference string) (ids.ActivityID, string, bool, error) {
	parent, key, found, err := namedThreadActivityTx(ctx, tx, reference)
	if err != nil || !found {
		return parent, key, false, err
	}
	held, err := seatHoldsTx(ctx, tx, seat, parent)
	if err != nil {
		return parent, key, false, err
	}
	if held || s.threadJoin.SentBySeat == nil {
		return parent, key, held, nil
	}
	sent, err := s.threadJoin.SentBySeat(ctx, tx, seat, []ids.ActivityID{parent})
	return parent, key, len(sent) == 1, err
}
