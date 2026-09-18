// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The route that records who wrote imported activities in the system they came
// from, and the one that re-folds the interaction graph afterwards.
//
// WHY COMPOSE OWNS THE LEDGER. `source_attribution_repair` is the only table
// this package writes here, and it is registered to this package because the
// repair spans modules: it writes activities today and the record tables next,
// and no single module can hold the ledger of what it did without reaching into
// its siblings' tables to keep it.
//
// WHOSE ACT THIS IS. The caller's. There is no synthesized principal, no
// on-behalf-of naming a departed colleague, and no forged session: the audit
// row says the administrator repaired the record of who wrote what, which is
// exactly what happened. `captured_by` is never touched — it answers who
// recorded the row HERE, and the columns this writes answer who wrote it THERE.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type attributionHandlers struct {
	db         *database.DB
	activities *activities.Store
	// The record stores, one per module that owns a table this repair reaches.
	// contacts owns three of the five record types (contact, company, lead), so
	// it takes the object name as an argument where the other two do not.
	contacts *contacts.Store
	deals    *deals.Store
	projects *projects.Store
}

// RepairSourceAttribution writes one batch of author attributions.
//
// PER ROW, not one transaction for the batch. Five hundred records walked from
// another system will contain a few this installation does not hold, or has
// archived, or whose author never had a seat — and refusing four hundred and
// ninety-nine good rows because of one is how a repair becomes unrunnable. Each
// row answers for itself and the result names every one that did not move.
func (h attributionHandlers) RepairSourceAttribution(w http.ResponseWriter, r *http.Request) {
	// The contract says `x-agent-access: human-only`, and that annotation is a
	// DECLARATION — the generated wrapper stamps the security scheme into the
	// context and refuses nobody. auth.RequireHuman is its in-handler twin, and
	// without it a passport-bearing agent reaches this route: rewriting who is
	// on record as having written somebody else's correspondence is a human's
	// act on the installation's own history, taken on the caller's authority.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// ADMIN, not merely a seat that may edit an activity. `activity:update` is
	// the right object grant for the column this writes, but it is the grant
	// every rep holds — and rewriting who is on record as having written
	// somebody else's correspondence, in batches, across the installation's
	// whole history, is not an ordinary edit. Without this a rep could restate
	// the authorship of any activity they may edit.
	if err := auth.RequireAdmin(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.SourceAttributionRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if len(req.Rows) == 0 {
		httperr.Write(w, r, httperr.Validation("rows", "empty", "Send at least one record to attribute."))
		return
	}
	// The contract bounds the batch at five hundred and the generated wrapper
	// enforces no maxItems, so this is where that bound binds. The object-type
	// enum is checked a few lines down, in attributionRowsRefused, against the
	// same map the dispatch reads.
	if len(req.Rows) > attributionBatchMax {
		httperr.Write(w, r, httperr.Validation("rows", "too_many",
			fmt.Sprintf("Send at most %d records in one batch.", attributionBatchMax)))
		return
	}
	// A LABEL RATHER THAN A SENTENCE. The contract carries this pattern and the
	// generated wrapper enforces no pattern, so this is where it binds.
	//
	// It is hygiene, NOT a privacy control, and the distinction is worth stating
	// because this check was once written as one: `alice-smith` satisfies the
	// pattern and names a human. What actually protects the column is that the
	// Art. 17 erasure clears it, along with the author's name and its digest,
	// on any record whose content it destroys.
	if !attributionBatchRef.MatchString(req.BatchRef) {
		httperr.Write(w, r, httperr.Validation("batch_ref", "invalid",
			"Name the run with letters, digits, dot, underscore, colon or hyphen "+
				"(\"hubspot-mirror-2026-09-17\")."))
		return
	}
	if bad := attributionRowsRefused(req.Rows); bad != nil {
		httperr.Write(w, r, bad)
		return
	}
	out := crmcontracts.SourceAttributionResult{
		Rows: make([]crmcontracts.SourceAttributionRowResult, 0, len(req.Rows)),
	}
	for _, row := range req.Rows {
		outcome, reason, err := h.attributeOne(r.Context(), req.BatchRef, row)
		if err != nil {
			httperr.Write(w, r, err)
			return
		}
		result := crmcontracts.SourceAttributionRowResult{
			ObjectType: string(row.ObjectType),
			ObjectId:   row.ObjectId,
			Outcome:    crmcontracts.SourceAttributionRowResultOutcome(outcome),
		}
		if reason != "" {
			r := reason
			result.Reason = &r
		}
		out.Rows = append(out.Rows, result)
		switch outcome {
		case string(storekit.SourceAuthorApplied):
			out.Applied++
		case outcomeUnchanged:
			out.Unchanged++
		default:
			out.Skipped++
		}
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// attributionRowsRefused validates the batch BEFORE any row is written, and
// answers the refusal when one cannot stand. Nil when every row may proceed.
//
// Whole-batch, deliberately, and this is the one validation that is not per-row:
// a row the CONTRACT forbids is the caller's own mistake, visible before any
// work starts, and half-applying a batch whose shape was wrong leaves them
// guessing which half. That is a different thing from a row this installation
// cannot attribute — an archived record, an author with no seat — which is
// answered per row with a reason, because a batch of five hundred walked from
// another system will always contain a few.
//
// Both checks exist because the generated wrapper enforces neither. It stamps
// the security scheme and decodes the body; it validates no enum, no maxItems,
// no pattern and no maxLength.
func attributionRowsRefused(rows []crmcontracts.SourceAttributionRow) error {
	for i, row := range rows {
		// Unchecked, the type is worse than cosmetic: it selects which TABLE the
		// row is written to, and the ledger is keyed on whatever the caller
		// sent — so a type nothing dispatches on would record a revision under a
		// key no write ever reaches, and the gate that is supposed to refuse a
		// stale answer would stop seeing it.
		//
		// Checked against the dispatch itself rather than against a list beside
		// it: a type accepted here and unhandled there is exactly the divergence
		// a second list produces.
		if _, ok := attributableObjects[row.ObjectType]; !ok {
			return httperr.Validation(
				fmt.Sprintf("rows[%d].object_type", i), "unsupported",
				"A source author can be recorded on an activity, contact, company, deal, lead or project.")
		}
		// Unchecked, an over-long name reaches the column as a database error
		// mid-batch: the rows before it are committed, the rows after it never
		// run, and the caller gets a 500 naming nothing they can act on.
		//
		// CHARACTERS, not bytes. `len` on a string counts UTF-8 bytes, and the
		// contract's maxLength counts characters — so a byte comparison refuses
		// names the contract permits, and refuses them by accent: "Renée" costs
		// six bytes for five characters, and a name at the limit with one such
		// letter would 422 the whole batch. On an import of German and Arabic
		// correspondence that is most of the interesting names.
		if row.SourceAuthorName != nil && utf8.RuneCountInString(*row.SourceAuthorName) > attributionNameMax {
			return httperr.Validation(
				fmt.Sprintf("rows[%d].source_author_name", i), "too_long",
				fmt.Sprintf("An author's name is at most %d characters.", attributionNameMax))
		}
	}
	return nil
}

// outcomeUnchanged is the wire's word for a row this batch did not move, and it
// is reached two ways.
//
// The LEDGER reaches it when a concurrent batch has already recorded a newer
// revision, which is this layer's own question and answered by the upsert below.
// The STORE reaches it when the activity already carries exactly this answer,
// which is the activity's question and answered under the activity's lock —
// activities.SourceAuthorUnchanged, whose own comment says why it could not
// stay here.
const outcomeUnchanged = "unchanged"

// attributionBatchMax is the contract's own bound, enforced here because the
// generated wrapper enforces no maxItems.
const attributionBatchMax = 500

// attributionBatchRef is the contract's own pattern for `batch_ref`, enforced
// here because the generated wrapper enforces no pattern. See the check in
// RepairSourceAttribution for why the shape is load-bearing rather than tidy.
var attributionBatchRef = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,200}$`)

// attributionNameMax is the contract's own bound on `source_author_name`,
// enforced here for the same reason: the generated wrapper enforces no
// maxLength.
const attributionNameMax = 200

// errRevisionSuperseded unwinds one record's transaction when a concurrent
// batch has already recorded a newer answer. It never reaches the caller: the
// row is reported `unchanged`, which is what it is.
var errRevisionSuperseded = errors.New("compose: a newer revision is already recorded")

// attributeOne is one record, in one transaction, with its ledger row.
//
// THE LEDGER AND THE WRITE COMMIT TOGETHER. A ledger saying "attributed" about
// a row whose write rolled back is worse than no ledger at all: the next run
// reads it, skips the record, and the correction is lost silently and forever.
func (h attributionHandlers) attributeOne(
	ctx context.Context, batchRef string, row crmcontracts.SourceAttributionRow,
) (outcome, reason string, err error) {
	// THE GRANT IS TAKEN BEFORE THE CLAIM, not inside the store below.
	//
	// The ledger row is claimed first so that two concurrent batches cannot both
	// decide they are newer — but that puts a write before the store's own
	// auth.Require, and a caller without it could then insert a claim and read
	// back whether the record was already attributed. The object grant is
	// checked here so the whole transaction, bookkeeping included, sits behind
	// it; the store checks it again for its own callers.
	//
	// The RECORD'S OWN object, read from the same map the dispatch reads. A
	// single administrative grant here would let a caller holding `deal:update`
	// alone reach a contact's byline, and a grant hard-coded to `activity`
	// would let them reach all five record types on an activity's authority.
	object, ok := attributableObjects[row.ObjectType]
	if !ok {
		// Unreachable through the route — attributionRowsRefused answered this
		// before any row was written — but a missing entry must refuse rather
		// than fall through to an empty object name, which auth.Require would
		// read as a grant nobody holds and every row would 403 with no reason.
		return "", "", fmt.Errorf("compose: %q has no RBAC object", row.ObjectType)
	}
	if err := auth.Require(ctx, object, principal.ActionUpdate); err != nil {
		return "", "", err
	}
	err = database.WithWorkspaceTx(ctx, h.db.Pool(), func(tx pgx.Tx) error {
		// The revision gate, read under the row's own lock so two batches
		// racing cannot both decide they are newer.
		// THE ACTIVITY IS WRITTEN FIRST, and the ledger row is taken afterwards
		// by a conditional upsert that is the whole of the race protection.
		//
		// An earlier version claimed the ledger first, to stop two concurrent
		// batches both deciding they were newer. It did stop that, and bought a
		// worse bug: a row the writer then REFUSED had to hand its claim back,
		// and the hand-back could only delete the row the upsert had just
		// written over. Apply revision 10, offer a revision 11 the writer
		// skips, and the delete takes 10's record with it — after which a
		// delayed retry of revision 9 finds an empty ledger and installs a
		// stale author over the good one. Sequential, no concurrency needed.
		//
		// Writing the activity first means a skip leaves no trace to undo. The
		// upsert below still settles the race, because `WHERE source_revision <
		// excluded.source_revision` is evaluated by Postgres under the row lock
		// the conflicting insert waits for: the loser updates nothing and
		// RETURNING yields no row. It also takes the locks in the order the
		// erasure takes them — activity, then ledger — so the two cannot
		// deadlock against each other.
		// DID ANYTHING ACTUALLY CHANGE? Not asked here, deliberately, and the
		// deliberateness is expensive experience. This layer twice tried to
		// answer it — by reading the ledger's `payload_hash` before calling the
		// store and skipping the write on a match — and both attempts were
		// defects.
		//
		// The first took `FOR UPDATE` on the ledger, which put this path's locks
		// in the order ledger→activity while the erasure takes activity→ledger:
		// a deadlock. The second dropped the lock and kept the skip, which is
		// worse. An unlocked read decides to skip the activity, a rival batch or
		// an erasure commits in the window before the upsert, and the ledger
		// then records an answer the activity does not carry — permanently,
		// because every later identical offer skips for the same reason. Against
		// an erased record it restores the name, the digest and the label the
		// erasure had just cleared.
		//
		// Both failures come from deciding outside the activity's own lock. So
		// the store decides, under that lock and behind the visibility check it
		// makes, and answers `unchanged` as a third outcome.
		in := storekit.SourceAuthorInput{AuthorName: row.SourceAuthorName}
		if row.SourceAuthorId != nil {
			id := ids.UUID(*row.SourceAuthorId)
			in.AuthorID = &id
		}
		got, why, err := h.attributeRecord(ctx, tx, row, in)
		if err != nil {
			return err
		}
		outcome, reason = string(got), why
		if got == storekit.SourceAuthorSkipped {
			// Nothing was written, so nothing is recorded. The reason is the
			// caller's to act on — fix the mapping, or accept that the record is
			// gone — and a ledger row would tell the next run there is nothing
			// left to try.
			return nil
		}
		// `applied` and `unchanged` BOTH fall through to the upsert. Unchanged
		// must still advance the revision: leaving an older one on record lets a
		// delayed batch carrying a DIFFERENT answer win the comparison below and
		// overwrite an answer that is already correct.
		var recorded bool
		if err := tx.QueryRow(ctx, `
			INSERT INTO source_attribution_repair
			    (object_type, object_id, source_author_id, source_author_name,
			     payload_hash, source_revision, batch_ref)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (object_type, object_id) DO UPDATE SET
			    source_author_id   = excluded.source_author_id,
			    source_author_name = excluded.source_author_name,
			    payload_hash       = excluded.payload_hash,
			    source_revision    = excluded.source_revision,
			    batch_ref          = excluded.batch_ref,
			    applied_at         = now()
			  WHERE source_attribution_repair.source_revision < excluded.source_revision
			RETURNING true`,
			string(row.ObjectType), row.ObjectId, row.SourceAuthorId, row.SourceAuthorName,
			attributionPayloadHash(row), row.SourceRevision, batchRef).Scan(&recorded); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// A concurrent batch carrying a NEWER answer got here first, so
				// this transaction's activity write must not stand either: it
				// would leave the row saying one thing and the ledger saying a
				// newer other. Rolling back is the whole transaction's business,
				// so the row is reported unchanged and the error returned.
				outcome = outcomeUnchanged
				return errRevisionSuperseded
			}
			return fmt.Errorf("compose: recording what the repair attributed: %w", err)
		}
		return nil
	})
	if errors.Is(err, errRevisionSuperseded) {
		// Not a failure: the record carries a newer answer than this batch had.
		return outcomeUnchanged, "", nil
	}
	if err != nil {
		return "", "", err
	}
	return outcome, reason, nil
}

// attributionPayloadHash records WHAT was applied, for an operator reading the
// ledger later. It no longer decides anything.
//
// It was once the "did anything change" comparison, read here before the store
// was called and used to skip the write on a match. That cost two review rounds
// — a lock-order deadlock, then a read-modify-write window that could commit a
// ledger row the activity did not agree with — because the decision needs the
// activity's own lock and this layer does not hold it. SetSourceAuthorTx now
// answers it, under that lock, by comparing the two author columns themselves.
//
// A DIGEST, not the values. An earlier version stored `id|name` verbatim, which
// put the author's name in a second column beside the one the Art. 17 erasure
// clears — so erasing the name left it plainly readable one column over.
//
// A DIGEST IS NOT ANONYMITY, though. It is unkeyed SHA-256 over a human name
// drawn from a staff list, so a few hundred guesses re-identify it, and the
// erasure empties this column along with the name rather than trusting the hash
// to protect it.
func attributionPayloadHash(row crmcontracts.SourceAttributionRow) string {
	id := ""
	if row.SourceAuthorId != nil {
		id = row.SourceAuthorId.String()
	}
	name := ""
	if row.SourceAuthorName != nil {
		name = *row.SourceAuthorName
	}
	sum := sha256.Sum256([]byte(id + "\x00" + name))
	return hex.EncodeToString(sum[:])
}

// RebuildAttributionGraph re-folds the who-knows-whom projection.
//
// ONE TRANSACTION for the whole fold, because the rebuild clears and refills:
// a reader arriving between those two statements would see an empty graph,
// and inside a transaction they see the old one until the new one commits.
// That is the same shape the periodic reconciler uses, for the same reason.
func (h attributionHandlers) RebuildAttributionGraph(w http.ResponseWriter, r *http.Request) {
	// Bound once, so the transaction and the fold inside it are demonstrably
	// the same context rather than two reads of the request that a later edit
	// could let drift apart.
	ctx := r.Context()
	// Human-only and admin-only, for the reason the repair itself is: the
	// contract's annotation declares it and these refuse it. A whole-
	// installation re-fold is neither an agent's to trigger nor a rep's.
	if err := auth.RequireHuman(ctx); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := auth.RequireAdmin(ctx); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := database.WithWorkspaceTx(ctx, h.db.Pool(), func(tx pgx.Tx) error {
		return search.RebuildEdges(ctx, tx)
	}); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, crmcontracts.AttributionRebuildResult{})
}
