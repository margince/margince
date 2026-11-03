// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type partySourceCandidate struct {
	replayCandidate
	user *ids.UUID
}

func recoverPartySourcesBatch(ctx context.Context, pool *pgxpool.Pool, limit int, log *slog.Logger) (int, error) {
	return drainStoredOriginals(ctx, pool, limit, log, storedOriginalPass[partySourceCandidate]{
		name: "captured party authority", unit: partyReplayUnit, offer: selectPartySourceCandidates, settle: restorePartySource, mark: capture.MarkPartySourceReplayedTx,
	})
}

func selectPartySourceCandidates(ctx context.Context, tx pgx.Tx, limit int) ([]partySourceCandidate, error) {
	rows, err := tx.Query(ctx, `SELECT a.id,a.kind,split_part(a.captured_by,':',2),rc.id,
 a.counterparty_outbound_attested,`+stampedMailboxLabelSQL+`,ci.user_id
 FROM activity a JOIN raw_capture rc ON `+storedMailOriginal+`
 LEFT JOIN capture_import ci ON ci.activity_id=a.id AND ci.user_id::text=`+auth.ConnectorSeatSQL("a")+`
 WHERE a.archived_at IS NULL AND (`+storedMailProvider+` OR split_part(a.captured_by,':',2)=ANY(@calendars::text[]))
 AND NOT EXISTS(SELECT 1 FROM capture_party_source_replay r WHERE r.activity_id=a.id)
 AND NOT EXISTS(SELECT 1 FROM capture_party_source ps WHERE ps.activity_id=a.id AND ps.user_id=ci.user_id AND ps.provider=split_part(a.captured_by,':',2))
 ORDER BY a.id LIMIT @party_source_limit`, pgx.NamedArgs{"party_source_limit": limit, "calendars": []string{sourceGCal, sourceGraphCal}})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []partySourceCandidate{}
	for rows.Next() {
		var c partySourceCandidate
		if err := rows.Scan(&c.activityID, &c.kind, &c.source, &c.rawCaptureID, &c.ourHeaderIsTrusted, &c.owner, &c.user); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func restorePartySource(ctx context.Context, tx pgx.Tx, c partySourceCandidate, payload []byte) (string, error) {
	if c.user == nil || c.owner == "" {
		return replayNoOwner, nil
	}
	rec := connector.NormalizedRecord{Fields: capture.ActivityFields{Kind: c.kind}}
	if c.kind == fieldEmail {
		raw, err := capture.DecodeStoredOriginal(payload)
		if err != nil {
			//nolint:nilerr // The durable unreadable outcome advances past this original.
			return replayUnreadable, nil
		}
		parsed, err := mailmap.Parse(raw, c.owner)
		if err != nil {
			//nolint:nilerr // The durable unreadable outcome advances past this original.
			return replayUnreadable, nil
		}
		rec = parsed.AttestSentByOwner(c.ourHeaderIsTrusted).ToRecord(c.source, nil)
	}
	if c.kind == activityKindMeeting {
		var parseErr error
		switch c.source {
		case sourceGCal:
			rec, parseErr = gcal.RecordOf(payload, c.owner)
		case sourceGraphCal:
			rec, parseErr = graphcal.RecordOf(payload, c.owner)
		default:
			rec = connector.NormalizedRecord{Fields: capture.ActivityFields{Kind: c.kind, ProviderPrivate: true}}
		}
		if parseErr != nil {
			rec = connector.NormalizedRecord{Fields: capture.ActivityFields{Kind: c.kind, ProviderPrivate: true}}
		}
	}
	err := capture.RestorePartySourceTx(ctx, tx, capture.PartySourceOriginal{Activity: c.activityID, Original: c.rawCaptureID, User: *c.user, Provider: c.source, Record: rec})
	return "restored", err
}

const partyReplayUnit = "activities"
