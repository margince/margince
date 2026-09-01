// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Importing the cards attached to one captured message.
//
// The authority question is the whole of this file. A card handed over by mail
// creates and updates PEOPLE, and the obvious way to write them from a
// background job — the system principal — is the wrong one: PrincipalSystem
// bypasses object RBAC and row scope both, so it would happily update a contact
// nobody may see and create people with no owner. What runs here instead is the
// MAILBOX'S GRANTING USER: the human who connected the mailbox, with their live
// permissions, teams and seat. The import can then reach exactly what that
// person could reach by dragging the same file into the browser, which is the
// honest claim, because mail arriving in their mailbox is theirs to act on.
//
// When that human can no longer write — archived, suspended, seat downgraded —
// the import stops rather than falling back to something stronger. A grant dies
// with the person who gave it.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// vcardIngestMaxCards bounds what ONE MESSAGE may import.
//
// Deliberately far below the manual upload's ceiling, and the difference is who
// is holding it. An upload is a human choosing a file and watching the report; a
// mailed card arrives unattended from whoever wrote to us, so the count is an
// attacker's parameter rather than a user's choice. A handover carries one card,
// occasionally a few — a message bearing more than this is an address-book dump,
// which is not a thing to act on without a human looking at it.
//
// The cap is across the WHOLE MESSAGE, not per attachment: one message may carry
// several .vcf files, and a per-file cap would let ten attachments of five cards
// past a limit of five.
const vcardIngestMaxCards = 5

// vcardIngestMaxAttempts is how often one message's import is retried before the
// job is cancelled. A card that cannot be parsed will not parse on the fourth
// try; what this leaves room for is a database blip or an object store timeout.
const vcardIngestMaxAttempts = 3

// VCardIngestArgs is one captured message's cards.
type VCardIngestArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
	Activity  ids.UUID `json:"activity_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (VCardIngestArgs) Kind() string { return "vcard_ingest" }

// WorkspaceID binds this import to its tenant (jobs.WorkspaceScoped).
func (a VCardIngestArgs) WorkspaceID() ids.UUID { return a.Workspace }

// vcardIngestWorker imports the cards attached to one captured message.
type vcardIngestWorker struct {
	river.WorkerDefaults[VCardIngestArgs]
	pool  *pgxpool.Pool
	blob  blobstore.Store
	users *identity.Service
	log   *slog.Logger
}

func newVCardIngestWorker(pool *pgxpool.Pool, blob blobstore.Store, log *slog.Logger) *vcardIngestWorker {
	return &vcardIngestWorker{pool: pool, blob: blob, users: identity.NewService(pool), log: log}
}

// Work imports one message's cards.
func (w *vcardIngestWorker) Work(ctx context.Context, job *river.Job[VCardIngestArgs]) error {
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	err = w.importCards(wsCtx, job.Args)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, apperrors.ErrPermissionDenied), errors.Is(err, apperrors.ErrNotFound):
		// Not a fault. The mailbox's granting human can no longer write here,
		// or the message is gone — both are answers, and retrying either would
		// have the job beat against the same refusal until it exhausts.
		w.log.InfoContext(ctx, "a mailed card was not imported: the mailbox's grant no longer writes",
			"activity", job.Args.Activity, "workspace", job.Args.Workspace)
		return nil
	case job.Attempt >= vcardIngestMaxAttempts:
		return river.JobCancel(err)
	default:
		return jobs.FaultContext(ctx, err)
	}
}

// importCards is one attempt: establish authority, read the cards, write them.
func (w *vcardIngestWorker) importCards(ctx context.Context, args VCardIngestArgs) error {
	actorCtx, err := w.asMailboxGrantor(ctx, args)
	if err != nil {
		return err
	}
	entries, err := w.readCards(actorCtx, args.Activity)
	if err != nil || len(entries) == 0 {
		return err
	}
	store := people.NewStore(InstallationDB(w.pool))
	results, err := store.ImportVCards(actorCtx, entries)
	if err != nil {
		return err
	}
	// The outcome per card, at info: this path writes people unattended, so
	// "which contact did that come from" must be answerable from the log alone
	// when somebody asks months later. The audit rows carry the same trace;
	// this is what makes it findable without one.
	for _, r := range results {
		w.log.InfoContext(ctx, "a card attached to captured mail was imported",
			"activity", args.Activity, "card", r.Index+1,
			"outcome", string(r.Outcome), "reason", r.Reason)
	}
	return nil
}

// asMailboxGrantor binds the human who connected the mailbox this message
// arrived in.
//
// The connector identity rather than the human's own, because this is not the
// human acting — it is their mailbox's connector acting under their authority,
// which is exactly what PrincipalConnector with OnBehalfOf records. Capture's
// own sync builds the same principal for the same reason; that one runs inside
// the capture module and cannot be called from here, so the shape is stated
// again rather than the authority being weakened to something reachable.
//
// EffectiveAuthority reads grants and seat in ONE snapshot. Composed from two
// reads they can describe an authority nobody held — permissions from before a
// role change with a seat from after.
func (w *vcardIngestWorker) asMailboxGrantor(ctx context.Context, args VCardIngestArgs) (context.Context, error) {
	var grantedBy ids.UUID
	var provider string
	// The mailbox that CAPTURED this message, matched through the provenance
	// string capture stamps (connector:<provider>:<user id>). There is no
	// foreign key, so this is the only join available — and a message whose
	// captured_by names no live connection has no grantor to run as, which
	// stops the import rather than choosing one.
	err := w.pool.QueryRow(ctx, `
		SELECT cc.user_id, cc.provider
		  FROM activity a
		  JOIN capture_connection cc
		    ON ('connector:' || cc.provider || ':' || cc.user_id::text) = a.captured_by
		 WHERE a.id = $1 AND a.archived_at IS NULL AND cc.archived_at IS NULL`,
		args.Activity).Scan(&grantedBy, &provider)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no live mailbox grants this message's import: %w", apperrors.ErrNotFound)
		}
		return nil, fmt.Errorf("compose: resolving the mailbox behind a captured card: %w", err)
	}
	rbac, seat, err := w.users.EffectiveAuthority(ctx, args.Workspace, grantedBy)
	if err != nil {
		return nil, fmt.Errorf("compose: the mailbox's granting human no longer resolves: %w", err)
	}
	actor := principal.Principal{
		Type:        principal.PrincipalConnector,
		ID:          "connector:" + provider,
		UserID:      grantedBy,
		OnBehalfOf:  grantedBy,
		TeamIDs:     rbac.TeamIDs,
		SeatType:    seat,
		Permissions: rbac.Permissions,
	}
	actorCtx := principal.WithActor(ctx, actor)
	return principal.WithCorrelationID(actorCtx, ids.NewV7()), nil
}

// readCards parses every card attached to the message, under the source's own
// liveness check.
//
// The check is the same one the signature pass makes and it is here for the same
// reason: the trigger fired when the message was captured, and a human or a
// verdict can narrow, restrict or archive that message before this job runs.
// What this writes is a name, a number and a postal address onto people every
// seat can read — so copying a narrowed message's contents into person records
// republishes it in a form the narrowing does not reach.
//
// FOR SHARE, so the answer cannot go stale between this read and the writes: at
// read-committed a narrowing could commit in that gap and the cards would land
// anyway.
func (w *vcardIngestWorker) readCards(ctx context.Context, activity ids.UUID) ([]people.VCardEntry, error) {
	keys, err := w.liveCardKeys(ctx, activity)
	if err != nil || len(keys) == 0 {
		return nil, err
	}
	var entries []people.VCardEntry
	for _, key := range keys {
		parsed, err := w.parseCard(ctx, key)
		if err != nil {
			return nil, err
		}
		entries = append(entries, parsed...)
		// Checked as the attachments accumulate rather than at the end: the cap
		// exists to bound what an unattended path will act on, and a message
		// carrying ten files of a thousand cards has already been parsed by the
		// time a final count could refuse it.
		if len(entries) > vcardIngestMaxCards {
			w.log.WarnContext(ctx, "a mailed message carried more cards than one message may import",
				"activity", activity, "cards", len(entries), "limit", vcardIngestMaxCards)
			return nil, nil
		}
	}
	return entries, nil
}

// liveCardKeys reads the message's card attachments while holding the message
// against a concurrent narrowing.
func (w *vcardIngestWorker) liveCardKeys(ctx context.Context, activity ids.UUID) ([]string, error) {
	var keys []string
	err := InstallationDB(w.pool).Tx(ctx, func(tx pgx.Tx) error {
		var limited bool
		if err := tx.QueryRow(ctx, `
			SELECT audience <> 'workspace'
			       OR restricted_at IS NOT NULL
			       OR archived_at IS NOT NULL
			  FROM activity WHERE id = $1 FOR SHARE`, activity).Scan(&limited); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// The message is gone. Nothing to import, and not a fault: it
				// was erased or never existed, and both are answers.
				return nil
			}
			return fmt.Errorf("compose: reading the card's source message: %w", err)
		}
		if limited {
			return nil
		}
		rows, err := tx.Query(ctx, `
			SELECT storage_key FROM attachment
			 WHERE activity_id = $1
			   AND archived_at IS NULL
			   AND (lower(content_type) IN ('text/vcard', 'text/x-vcard', 'text/directory')
			        OR lower(filename) LIKE '%.vcf')
			 ORDER BY created_at`, activity)
		if err != nil {
			return fmt.Errorf("compose: listing a message's card attachments: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return fmt.Errorf("compose: reading a card attachment's key: %w", err)
			}
			keys = append(keys, key)
		}
		return rows.Err()
	})
	return keys, err
}

// parseCard reads one stored attachment and parses the cards in it.
//
// A file that does not parse is NOT a fault. The probe matches a filename as
// well as a content type, so anything named `.vcf` reaches here — including a
// file that is not a card at all. Refusing would retry the same unparseable
// bytes and then cancel the whole message's import, losing the cards in the
// attachment beside it.
func (w *vcardIngestWorker) parseCard(ctx context.Context, key string) ([]people.VCardEntry, error) {
	body, _, err := w.blob.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("compose: opening a stored card: %w", err)
	}
	defer func() {
		if cerr := body.Close(); cerr != nil {
			w.log.WarnContext(ctx, "closing a stored card", "err", cerr)
		}
	}()
	entries, err := people.ParseVCards(body)
	if err != nil {
		w.log.InfoContext(ctx, "an attachment that looked like a card did not parse as one", "err", err)
		return nil, nil
	}
	return entries, nil
}
