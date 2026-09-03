// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Recovering the names calendar invitations gave, for the attendee rows written
// before those names were carried.
//
// Its own pass rather than an arm of the participant replay, because the replay
// is settled per activity: a meeting whose participants it already recovered
// carries a marker saying so, and re-reading it would re-answer a question that
// is closed. This asks a different question of the same stored originals — not
// "who was in this meeting", which is answered, but "what did the invitation
// call them" — so it selects on the column that records that and drains as it
// fills.
//
// It ends permanently. Live stamping now writes a name or '' for every attendee
// it records, so the NULL this selects on can only appear on rows written before
// the column existed, and every pass leaves fewer.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// nameRecoveryCandidate is one calendar activity whose stored original may name
// attendees the participant rows do not.
type nameRecoveryCandidate struct {
	activityID ids.ActivityID
	source     string
	payload    []byte
}

// recoverAttendeeNamesBatch fills in the names on up to limit calendar
// activities' participant rows, and names the people those rows resolved to.
func recoverAttendeeNamesBatch(ctx context.Context, pool *pgxpool.Pool, limit int, log *slog.Logger) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("compose: the attendee name recovery needs a positive batch limit, got %d", limit)
	}
	// One correlation id per batch, minted here because this pass has no
	// request behind it. The fill it drives is an audited write and storekit
	// refuses to emit without one — and the refusal would be silent in the
	// worst way: the transaction rolls back, the rows it would have named stay
	// NULL, the same batch is selected on the next tick, and the pass retries
	// forever while reporting nothing.
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	var settled int
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		candidates, err := selectNameRecoveryCandidates(ctx, tx, limit)
		if err != nil {
			return err
		}
		for _, c := range candidates {
			if err := recoverOneMeetingsNames(ctx, tx, c); err != nil {
				return err
			}
			settled++
		}
		if settled > 0 {
			log.DebugContext(ctx, "attendee name recovery: read a batch of stored invitations",
				"activities", settled)
		}
		return nil
	})
	return settled, err
}

// selectNameRecoveryCandidates finds calendar activities whose original is
// still stored and whose participant rows predate the name column.
func selectNameRecoveryCandidates(ctx context.Context, tx pgx.Tx, limit int) ([]nameRecoveryCandidate, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.source_system, rc.payload
		  FROM activity a
		  JOIN raw_capture rc
		    ON rc.source_system = a.source_system AND rc.source_id = a.source_id
		 WHERE a.archived_at IS NULL
		   AND a.source_system IN ($1, $2)
		   AND EXISTS (
		       SELECT 1 FROM activity_participant ap
		        WHERE ap.activity_id = a.id
		          AND ap.display_name IS NULL
		          AND ap.address IS NOT NULL)
		 ORDER BY a.occurred_at DESC, a.id
		 LIMIT $3`, sourceGCal, sourceGraphCal, limit)
	if err != nil {
		return nil, fmt.Errorf("compose: selecting the invitations to re-read for names: %w", err)
	}
	defer rows.Close()
	var out []nameRecoveryCandidate
	for rows.Next() {
		var c nameRecoveryCandidate
		if err := rows.Scan(&c.activityID, &c.source, &c.payload); err != nil {
			return nil, fmt.Errorf("compose: selecting the invitations to re-read for names: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compose: selecting the invitations to re-read for names: %w", err)
	}
	return out, nil
}

// recoverOneMeetingsNames reads one stored invitation and settles every
// name-less participant row on it, through the module that owns that write.
//
// An unreadable original still settles its rows, with nobody named: the read
// has been attempted and will not improve, and a row left NULL would be offered
// again on every later tick.
func recoverOneMeetingsNames(ctx context.Context, tx pgx.Tx, c nameRecoveryCandidate) error {
	if err := capture.RecordAttendeeNames(ctx, tx, c.activityID, namedPartiesOf(c)); err != nil {
		return err
	}
	// The rows now carry what the invitation said, so the people they resolved
	// to can be named from it — the same call the live capture path makes.
	return people.FillParticipantNamesTx(ctx, tx, c.activityID)
}

// namedPartiesOf reads the parties one stored invitation names, lowercased to
// match how the participant row stores an address.
//
// An original that cannot be read yields nobody rather than a fault: the
// meeting itself was captured successfully, and a payload this cannot parse is
// not going to become parseable later.
func namedPartiesOf(c nameRecoveryCandidate) []connector.MessageParticipant {
	raw, err := decodeStoredOriginal(c.payload)
	if err != nil {
		return nil
	}
	var parties []connector.MessageParticipant
	switch c.source {
	case sourceGCal:
		parties, err = gcal.ParticipantsOf(raw, "")
	case sourceGraphCal:
		parties, err = graphcal.ParticipantsOf(raw, "")
	default:
		return nil
	}
	if err != nil {
		return nil
	}
	named := make([]connector.MessageParticipant, 0, len(parties))
	for _, party := range parties {
		if party.DisplayName == "" || party.Email == "" {
			continue
		}
		named = append(named, party)
	}
	return named
}
