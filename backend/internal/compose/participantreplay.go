// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Recovering the CCs and meeting attendees of activities captured before
// participants were recorded structurally (ADR-0078 / ACT-DDL-3).
//
// The two-end backfill (activities.BackfillParticipantsBatch) recovers who the
// message was BETWEEN. It deliberately stops there, because naming everyone
// else means re-reading the stored original — a different kind of work, with
// its own parser per provider and its own failure mode. This is that pass.
//
// It lives in compose because it spans two modules that may not import each
// other: the parsers belong to capture (mail and calendar each read their own
// format), and nothing in one module may reach into a sibling. The write goes
// back through capture.StampFurtherParticipants rather than a second copy of
// the resolution SQL, so a recovered row and a captured one are identical to
// the interaction graph that reads them.
//
// The pass is bounded by a durable marker per activity rather than by a
// cursor. Most messages have no CCs at all, so "did this one have any" cannot
// be asked of the result — without the marker the loop would re-parse the same
// thousands of originals forever. participantreplay's own migration says why
// the two-end backfill needs no such thing.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The verdicts the marker table records. Only `participants` wrote rows; the
// other three say why an activity produced none, which is what keeps a
// re-parse from being attempted forever.
const (
	replayWroteParticipants = "participants"
	replayFoundNone         = "none"
	replayUnreadable        = "unreadable"
	replayNoOwner           = "no_owner"
)

// The connectors whose stored originals this pass can re-read. A connector
// absent from here is marked unreadable rather than skipped: skipping would
// re-select it on every pass, and the honest record is that this parser has no
// reading of that format.
//
// These are CONNECTOR names, read from the row's captured_by provenance, not
// natural-key systems: mail from every adapter now shares one identity
// (connector.EmailSourceSystem), so the identity no longer says which bytes are
// on file. The offline demo stores JSON under that same mail identity, and
// handing its payload to the RFC822 parser is exactly the confusion this
// distinction prevents.
const (
	sourceGmail    = "gmail"
	sourceIMAP     = "imap"
	sourceGraph    = "graph"
	sourceGCal     = "gcal"
	sourceGraphCal = "graphcal"
)

// replayCandidate is one activity whose original is still on file.
type replayCandidate struct {
	activityID ids.ActivityID
	kind       string
	// source is the CONNECTOR that captured this row, read from captured_by —
	// which format its stored payload is in. Not the natural-key system: mail
	// from every adapter shares one identity now, so that column no longer
	// distinguishes an RFC822 original from the demo generator's JSON.
	source  string
	payload []byte
	// owner is the mailbox address the connection reads, taken from the
	// connection's own account label rather than the granting user's login
	// address — those differ, and it is the MAILBOX the headers name.
	owner string
	// ourHeaderIsTrusted is the persisted owner attestation: the provider
	// vouched that our own mailbox owner SENT this message, so its recipient
	// list is what our user typed. Without it the list is the sender's text
	// and no colleague may be bound from it — capture.StampFurtherParticipants
	// says what a forged Cc line would otherwise buy.
	ourHeaderIsTrusted bool
}

// partyListIsAttested is this candidate's answer to the question
// capture.ParticipantListAttested asks of a live record: did the PROVIDER state
// this party list?
//
// Both halves are read from what the row persisted. The mail half is the stored
// owner attestation. The calendar half is the connector that captured the row,
// because a calendar's attendee list is the provider's own record of who was
// invited rather than a header somebody typed — and `source` comes from
// captured_by, which capture wrote, not from anything the record claimed about
// itself.
//
// A replayed row and a live one must name the same people, so this answers the
// same question live capture asks; a drift here is an attendee who reads a
// meeting on one path and not the other.
func (c replayCandidate) partyListIsAttested() bool {
	return c.ourHeaderIsTrusted || c.source == sourceGCal || c.source == sourceGraphCal
}

// replayParticipantsBatch re-reads up to limit stored originals and returns how
// many activities it settled — written, empty or refused alike, because every
// one of them is progress the next pass will not repeat.
func replayParticipantsBatch(ctx context.Context, pool *pgxpool.Pool, limit int, log *slog.Logger) (int, error) {
	return drainStoredOriginals(ctx, pool, limit, log, storedOriginalPass{
		name:   "participant replay",
		unit:   "activities",
		offer:  selectReplayCandidates,
		settle: replayOne,
		mark:   markReplayed,
	})
}

// storedOriginalPass is one sweep over stored provider originals: which rows to
// offer, what to do with each, and where the outcome is recorded.
//
// Two passes share this shape — the participant replay and the meeting attendee
// repair — and they share the DRAIN rather than each spelling it. The parts they
// have in common are the parts that are easy to get subtly wrong: one bounded
// transaction, one correlation id for the whole batch (an audited write is
// refused without one, which would fail the batch and re-select the same rows
// forever), and a marker written for every row the pass touched so a settled row
// is never offered twice.
type storedOriginalPass struct {
	// name and unit are what the debug line says: which pass ran, and what its
	// count is counting.
	name string
	unit string
	// offer answers which rows this pass still owes work on.
	offer  func(ctx context.Context, tx pgx.Tx, limit int) ([]replayCandidate, error)
	settle func(ctx context.Context, tx pgx.Tx, c replayCandidate) (string, error)
	mark   func(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, outcome string) error
}

// drainStoredOriginals runs one bounded batch of a stored-original pass and
// answers how many rows it settled — written, empty or refused alike, because
// every one of them is progress the next pass will not repeat.
func drainStoredOriginals(
	ctx context.Context, pool *pgxpool.Pool, limit int, log *slog.Logger, pass storedOriginalPass,
) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("compose: the %s needs a positive batch limit, got %d", pass.name, limit)
	}
	// One correlation id per batch. Naming an attendee is an audited write, and
	// storekit refuses to emit its event without one — a refusal that would
	// take the whole batch down with it and re-select the same rows forever.
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	var settled int
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		candidates, err := pass.offer(ctx, tx, limit)
		if err != nil {
			return err
		}
		for _, c := range candidates {
			outcome, err := pass.settle(ctx, tx, c)
			if err != nil {
				return err
			}
			if err := pass.mark(ctx, tx, c.activityID, outcome); err != nil {
				return err
			}
			settled++
		}
		if settled > 0 {
			log.DebugContext(ctx, "compose: settled a batch of stored originals",
				"pass", pass.name, pass.unit, settled)
		}
		return nil
	})
	return settled, err
}

// selectReplayCandidates finds interaction activities whose original is still
// stored and which have not been re-read yet.
//
// Both the mailbox owner and the format are resolved from captured_by, which
// the sink stamps from the AUTHENTICATED principal as `connector:<name>:<user>`
// — never from anything a record claimed. That is a stricter answer than the
// one it replaces: the old query matched capture_connection.provider against
// the activity's source_system and accepted it only when the provider had
// exactly ONE connection, so a workspace with two Gmail mailboxes replayed
// nothing at all. Naming the user directly resolves those, and
// capture_connection is unique on (user_id, provider), so the pair identifies
// one mailbox or none.
//
// A row whose provenance does not parse — a human-logged activity, an older
// stamp carrying no user — yields no owner and is recorded as such rather than
// guessed at. Parsing against the wrong address would file the mailbox owner as
// a participant of their own conversation.
func selectReplayCandidates(ctx context.Context, tx pgx.Tx, limit int) ([]replayCandidate, error) {
	// split_part on a `connector:<name>:<user>` stamp: field 2 is the connector,
	// field 3 the seat.
	//
	// The seat is missing on rows captured before provenance carried one, whose
	// stamp is the bare `connector:gmail`. Those fall back to the rule this
	// query used to apply to every row -- the provider's connection, when it has
	// exactly one -- so an old row still replays, and still declines rather than
	// guessing when two mailboxes share a provider. A stamp naming a seat never
	// reaches the fallback, so a workspace with two Gmail mailboxes resolves both
	// of its NEW rows, which the single-connection rule alone could not do.
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.kind, split_part(a.captured_by, ':', 2), rc.payload,
		       coalesce(a.counterparty_outbound_attested, false),
		       coalesce((
		         SELECT c.account_label
		           FROM capture_connection c
		          WHERE c.provider = split_part(a.captured_by, ':', 2)
		            AND (
		              c.user_id::text = split_part(a.captured_by, ':', 3)
		              OR (split_part(a.captured_by, ':', 3) = '' AND NOT EXISTS (
		                  SELECT 1 FROM capture_connection other
		                   WHERE other.provider = c.provider AND other.id <> c.id)))
		          LIMIT 1), '')
		  FROM activity a
		  JOIN raw_capture rc
		    ON rc.source_system = a.source_system AND rc.source_id = a.source_id
		 WHERE a.archived_at IS NULL
		   AND a.source_system <> ''
		   AND a.captured_by LIKE 'connector:%'
		   AND NOT EXISTS (
		       SELECT 1 FROM activity_participant_replay r WHERE r.activity_id = a.id)
		 ORDER BY a.id
		 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("compose: selecting activities whose participants can be replayed: %w", err)
	}
	defer rows.Close()

	var out []replayCandidate
	for rows.Next() {
		var c replayCandidate
		if err := rows.Scan(&c.activityID, &c.kind, &c.source, &c.payload,
			&c.ourHeaderIsTrusted, &c.owner); err != nil {
			return nil, fmt.Errorf("compose: reading a replay candidate: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compose: reading the replay candidates: %w", err)
	}
	return out, nil
}

// replayOne parses one stored original and writes whatever further
// participants it names, returning the verdict to record.
//
// A parse failure is a verdict, not an error: the payload is years-old
// provider output, and one message this parser cannot decompose must not stop
// the pass from reaching the rest.
func replayOne(ctx context.Context, tx pgx.Tx, c replayCandidate) (string, error) {
	if c.owner == "" {
		return replayNoOwner, nil
	}
	// A kind that is not an interaction writes no participants however many
	// the header names, so parsing it and recording `participants` would file
	// the one verdict nobody ever revisits against an activity that produced
	// nothing.
	if !relstrength.IsParticipantKind(c.kind) {
		return replayFoundNone, nil
	}
	// A payload this parser cannot decompose is a VERDICT the pass records and
	// moves past, which is why neither failure below is returned. These are
	// years-old provider originals; failing the batch on one of them would
	// stop the pass reaching every message after it, and the marker is what
	// makes the attempt not repeat forever.
	raw, decodeErr := decodeStoredOriginal(c.payload)
	if decodeErr != nil {
		return replayUnreadable, nil //nolint:nilerr // unreadable is the recorded outcome, not a fault
	}
	var participants []connector.MessageParticipant
	var parseErr error
	switch c.source {
	case sourceGmail, sourceIMAP, sourceGraph:
		// All three store the message as its RFC822 original, so one reader
		// serves them: Graph hands over the MIME itself (/$value), which is why
		// the Outlook mailbox needs no parser of its own. Anything else that
		// shares the mail IDENTITY but not the format — the offline demo, which
		// stores JSON — falls to the default arm rather than being fed to this
		// parser, which is why the switch reads the connector and not the key.
		participants, parseErr = mailmap.ParticipantsOf(raw, c.owner)
	case sourceGCal:
		participants, parseErr = gcal.ParticipantsOf(raw, c.owner)
	case sourceGraphCal:
		participants, parseErr = graphcal.ParticipantsOf(raw, c.owner)
	default:
		return replayUnreadable, nil
	}
	if parseErr != nil {
		return replayUnreadable, nil //nolint:nilerr // unreadable is the recorded outcome, not a fault
	}
	if len(participants) == 0 {
		return replayFoundNone, nil
	}
	// No transport: this pass re-reads stored MAIL and CALENDAR originals, whose
	// parties are addresses. A party named by a channel account arrives from a
	// live record and has no stored original to replay.
	if err := capture.StampFurtherParticipants(ctx, tx, c.activityID, c.kind, "",
		c.partyListIsAttested(), participants); err != nil {
		return "", err
	}
	// The rows just written carry whatever name the original gave, so the
	// people they resolved to are named here rather than left to the recovery
	// pass beside this one. That pass selects on display_name IS NULL, which
	// the stamp above has just filled in, and this pass is settled per activity
	// and will not offer the meeting again — so a meeting replayed before the
	// recovery ever ran would otherwise fall permanently between the two.
	if err := people.FillParticipantNamesTx(ctx, tx, c.activityID); err != nil {
		return "", err
	}
	return replayWroteParticipants, nil
}

// decodeStoredOriginal unwraps what the sink put in raw_capture.payload.
//
// Delegated rather than spelled here: the three spellings a payload can carry
// are capture/sinkraw.go's own invention, and a reader holding its own copy of
// that list is the copy that falls behind when a fourth arrives.
func decodeStoredOriginal(payload []byte) ([]byte, error) {
	return capture.DecodeStoredOriginal(payload)
}

// markReplayed records that this activity has been re-read, so no later pass
// selects it again.
func markReplayed(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, outcome string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_participant_replay (activity_id, outcome)
		VALUES ($1, $2)
		ON CONFLICT (activity_id) DO NOTHING`,
		activityID, outcome); err != nil {
		return fmt.Errorf("compose: recording that an activity's participants were replayed: %w", err)
	}
	return nil
}
