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

	"github.com/margince/margince/backend/internal/modules/capture"
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

// vcardIngestInsertOpts is the trigger's insert, spelled here beside the worker
// whose queue and attempt cap it names.
//
// Built directly rather than through oneOffChildOpts: that helper reads the
// fan-out declaration, and this kind is inserted by a CONSUMER naming one
// message rather than by a dispatcher fanning out over a tenant list — which is
// what opts_owner: caller says in the contract. Asking the fan-out helper for a
// caller-owned kind panics, and it panics inside the subscriber goroutine, so
// the first mailed card would take the worker process down.
//
// The uniqueness is per MESSAGE: the args name one activity, so two deliveries
// of one capture event collapse onto a single import while two messages each get
// their own.
func vcardIngestInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		Queue:      aiCaptureQueue,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	}
}

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
	default:
		// River owns the attempt cap, from the contract's own max_attempts.
		// A second ceiling counted here would be a copy of that number, and the
		// two would drift the first time either moved.
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
	// The message goes WITH the cards, and the import re-reads it inside each
	// card's own write transaction.
	//
	// That is the only place the check can hold. liveCardKeys locks the message
	// while it lists the attachments and releases it when that transaction
	// commits; the object-store fetches, and then a transaction per card, all
	// happen after. A check here — before the writes, however close to them —
	// would leave exactly the gap it looks like it closes.
	store := people.NewStore(InstallationDB(w.pool))
	results, err := store.ImportVCardsFromMessage(actorCtx, entries, args.Activity)
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
	return w.stageReviews(actorCtx, entries, results)
}

// stageReviews turns every near-match into a proposal a human can decide.
//
// ImportVCards reports a card that RESEMBLES somebody rather than merging it,
// and that verdict is only worth having if the question outlives the import. The
// browser upload stages each one; without this the mailed path would log the
// same verdict and drop it, so a contact who posted their card is never created
// and nothing ever reaches a queue — the exact silence this feature exists to
// end, reintroduced one branch deeper.
//
// The stager is self-only and takes the ACTING principal as the proposal's
// subject, which here is the mailbox's granting human. That is the right
// reviewer: the card arrived in their mailbox.
func (w *vcardIngestWorker) stageReviews(ctx context.Context, entries []people.VCardEntry, results []people.VCardResult) error {
	stage := vcardCreateStager(w.pool)
	for _, r := range results {
		// The index is ImportVCards' own position in the slice it was handed, so
		// the bound is a belt on a contract that already holds — but a panic in
		// an unattended writer is worth one comparison.
		if r.Outcome != people.VCardNeedsReview || r.Index < 0 || r.Index >= len(entries) {
			continue
		}
		if err := stage(ctx, entries[r.Index], r.PersonID); err != nil {
			return fmt.Errorf("compose: staging a mailed card for review: %w", err)
		}
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
	// The declared default, from the entry that declares it rather than repeated
	// here: a second copy of the number would be the drift the settings registry
	// exists to prevent, and it would drift silently — a workspace that has never
	// touched the switch would follow one answer for signatures and another for
	// cards.
	defaultRaw, err := capture.SignatureEnrich.DefaultJSON()
	if err != nil {
		return nil, fmt.Errorf("compose: reading the declared mail-reading default: %w", err)
	}
	var grantedBy ids.UUID
	var provider string
	// The mailbox that CAPTURED this message, matched through the provenance
	// string capture stamps (connector:<provider>:<user id>). There is no
	// foreign key, so this is the only join available — and a message whose
	// captured_by names no live connection has no grantor to run as, which
	// stops the import rather than choosing one.
	//
	// The format is capture's, spelled again here because a module never imports
	// a sibling; capture.connectorProvenance owns it, and people's own signature
	// candidates carry a third copy for the same reason. Nothing holds the three
	// to each other, and a change to the Go owner makes every SQL copy match zero
	// rows — which stops this import with no failing assertion anywhere.
	//
	// status = 'connected' with the archive test, because a mailbox the human
	// DISCONNECTED without archiving is a grant they took back. The signature
	// pass reads the same connection for its own switch, and this path writes
	// people rather than filling a field, so it may not be the looser of the two.
	err = w.pool.QueryRow(ctx, `
		SELECT cc.user_id, cc.provider
		  FROM activity a
		  JOIN capture_connection cc
		    ON ('connector:' || cc.provider || ':' || cc.user_id::text) = a.captured_by
		 WHERE a.id = $1 AND a.archived_at IS NULL
		   -- INBOUND only, the same gate the signature candidates apply. A card a
		   -- rep SENT is our own attachment coming back: forwarding a colleague's
		   -- .vcf out of the mailbox would otherwise file it as a contact, which
		   -- is not "mail arriving in their mailbox" by any reading.
		   AND a.direction = 'inbound'
		   AND cc.archived_at IS NULL AND cc.status = 'connected'
		   -- The per-mailbox switch, and the workspace default when the mailbox
		   -- has not chosen. It is the SAME switch the signature pass obeys:
		   -- both answer "read my mail for contact details", and a mailbox owner
		   -- who turned that off has not agreed to the card half of it either.
		   --
		   -- Read from the setting row rather than through the settings store,
		   -- because that store gates on auth.Require and this read is what
		   -- DECIDES whether to build a principal — it cannot use the one it
		   -- gates. capture.SignatureEnrich is declared MachineryApplied for
		   -- exactly this: candidate selection reads it per mailbox, in SQL.
		   AND COALESCE(cc.signature_enrich_enabled, (
		       SELECT coalesce((SELECT value FROM setting WHERE key = $2),
		                       $3::jsonb)::boolean))`,
		args.Activity, capture.SignatureEnrich.Key(), string(defaultRaw)).Scan(&grantedBy, &provider)
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

// readCards parses every card attached to the message, refusing a source the
// workspace may no longer read.
//
// The check exists because the trigger fired when the message was captured, and
// a human or a verdict can narrow, restrict or archive that message before this
// job runs. What this writes is a name, a number and a postal address onto
// people every seat can read — so copying a narrowed message's contents into
// person records republishes it in a form the narrowing does not reach.
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
		// The cap refuses the MESSAGE, not the surplus, and it is checked as the
		// attachments accumulate so a second file cannot carry the count past it.
		//
		// It bounds what is WRITTEN and not what is read: ParseVCards reads a
		// whole file before returning, so one attachment of ten thousand cards is
		// already parsed and resident by the time this sees it. Its own 4 MiB and
		// 5000-card ceilings are what bound that read; this one decides whether
		// an unattended path may act on what came back.
		if len(entries)+len(parsed) > vcardIngestMaxCards {
			w.log.WarnContext(ctx, "a mailed message carried more cards than one message may import",
				"activity", activity, "cards", len(entries)+len(parsed), "limit", vcardIngestMaxCards)
			return nil, nil
		}
		entries = append(entries, parsed...)
	}
	return entries, nil
}

// liveCardKeys reads the message's card attachments while holding the message
// against a narrowing that commits during the read.
//
// FOR SHARE holds only for as long as this transaction, which ends when the keys
// are returned — the blob reads and the writes both happen after it. What the
// lock buys is that the attachment list cannot be assembled from a message that
// is being narrowed as it is read; importCards re-checks before writing, and the
// comment there says what remains open.
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
			w.log.InfoContext(ctx, "a mailed card was not imported: its message is not workspace-readable",
				"activity", activity)
			return nil
		}
		rows, err := tx.Query(ctx, `
			SELECT storage_key FROM attachment
			 WHERE entity_type = 'activity' AND entity_id = $1
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
