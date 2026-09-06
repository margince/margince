// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What the engine can DO to one over-age record: the executor table, the two
// questions the authoring surface asks of it, and the dispatch.
//
// Split from retention.go because the table is two things at once — the dispatch
// AND the authorable set — and both the nightly pass and the write path consult
// it. Keeping it beside the pass made it read as an implementation detail of the
// loop, which is exactly the reading that let scope and action be validated
// independently.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// retentionExecutor applies one action to one record inside the pass's audited
// transaction.
type retentionExecutor func(s *RetentionService, ctx context.Context, tx pgx.Tx, id ids.UUID) error

// retentionActions is every `object_type/action` pair the engine can perform —
// the dispatch table AND the authorable set, deliberately one map.
//
// Scope and action are chosen INDEPENDENTLY by whoever authors a policy, but
// only some of their combinations have an executor: there is no way to archive
// an ai_call_payload or to anonymize an activity. A write path that validated
// each half separately would admit `deal/won` + `erase`, and the pass would then
// abort on the first due record and stay red every night until somebody deleted
// the row — taking every LATER policy with it, because policies are ordered.
// Storage limitation would stop installation-wide, silently. So membership here
// is what ParseRetentionScope is for scopes: the one gate, consulted by the
// validator and by the pass.
//
// person/erase is registered with a NIL executor: it owns its own transaction
// (the Art. 17 cascade is ~30 statements plus object-store deletes), so apply
// dispatches it before opening one. Nil means "runs outside the transaction",
// never "unsupported" — membership is the key, not the value.
var retentionActions = map[string]retentionExecutor{
	"person/erase":          nil,
	"activity/archive":      (*RetentionService).archiveActivity,
	"activity/erase":        (*RetentionService).eraseActivityContent,
	"deal/archive":          (*RetentionService).archiveDeal,
	"ai_call_payload/erase": (*RetentionService).erasePayload,
	"lead/anonymize":        (*RetentionService).anonymizeLead,
	"person/anonymize":      (*RetentionService).anonymizePerson,
}

// The executors. Named methods rather than closures in the table above, because
// each runs a by-id UPDATE and updateguard_test.go walks named functions to
// assert every one of them either carries a concurrency guard or is a ratified
// exception — an anonymous function is invisible to it, and the retention sweep's
// deliberately unguarded absolute writes would stop being checked at all.

func (s *RetentionService) archiveActivity(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if _, err := tx.Exec(ctx, `UPDATE activity SET archived_at = now() WHERE id = $1`, id); err != nil {
		return err
	}
	return s.invalidateGraph(ctx, tx, id)
}

func (*RetentionService) archiveDeal(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE deal SET archived_at = now() WHERE id = $1`, id)
	return err
}

// erasePayload deletes the row outright rather than scrubbing it in place —
// unlike activity/erase there is no metadata half of this record left to keep:
// ai_call_payload IS the special-category-adjacent content, and ai_call (the
// metadata row it FK-cascades from) survives untouched. The retention audit entry
// carries no payload bytes, only policy metadata.
func (*RetentionService) erasePayload(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM ai_call_payload WHERE id = $1`, id)
	return err
}

func (*RetentionService) anonymizePerson(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return anonymizePersonRecord(ctx, tx, id)
}

// SupportsRetentionAction reports whether the engine can perform this action on
// this object type. The authoring surface refuses a pair it answers false for.
func SupportsRetentionAction(objectType, action string) bool {
	_, ok := retentionActions[objectType+"/"+action]
	return ok
}

// ActionsForScope is every action a given scope may be authored with, sorted, so
// a refusal can name the alternatives instead of leaving the caller to guess at
// a set the contract's two independent enums do not express.
func ActionsForScope(objectType string) []string {
	out := make([]string, 0, 3)
	for _, action := range []string{actionArchive, actionAnonymize, actionErase} {
		if SupportsRetentionAction(objectType, action) {
			out = append(out, action)
		}
	}
	return out
}

// apply runs ONE action on ONE record in one audited transaction.
func (s *RetentionService) apply(ctx context.Context, pol retentionPolicy, id ids.UUID) error {
	pair := pol.ObjectType + "/" + pol.Action
	executor, supported := retentionActions[pair]
	if !supported {
		// Unreachable through the authoring surface, which refuses an
		// unsupported pair, and through the pass, which skips the policy before
		// selecting a record. Kept because an unsupported pair must never be
		// mistaken for a completed action.
		return fmt.Errorf("retention: no executor for %s", pair)
	}
	if executor == nil {
		return s.eraser.ErasePerson(ctx, id, "retention")
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := executor(s, ctx, tx, id); err != nil {
			return err
		}
		// Retention audits under the verb of the action it ran —
		// archive, anonymize and erase are all in the closed audit
		// vocabulary (0053) — so a governance read can tell a retention
		// anonymize from a user edit, and the field-history projection
		// can treat anonymize/erase as its scrub boundary instead of
		// parsing payload shapes. The policy metadata rides the evidence
		// column, and before/after stay nil: this row records that a
		// policy acted, not a field diff, so a projectable verb like
		// archive must carry no payload the field-history diff could
		// mistake for record fields.
		auditID, err := storekit.AuditWithEvidence(ctx, tx, pol.Action, pol.ObjectType, id, nil, nil, map[string]any{
			evidenceKeyRetentionAction: pol.Action, "policy": pol.ID, "retain_days": pol.RetainDays,
		})
		if err != nil {
			return err
		}
		policyID := pol.ID
		return storekit.EmitEventForEntity(ctx, tx, auditID, pol.ObjectType, id, retentionAppliedPayload(pol.Action, &policyID, nil))
	})
}

// eraseActivityContent is the activity/erase action. Transcript free-text is
// the special-category risk; the record of the meeting stays, its content goes
// — including any attached recording/transcript file (objects first, so the
// purge shares the person-erase durability guarantee).
//
// `raw` goes with `body`. It is the re-parseable original the schema names, so
// clearing the parsed copy and leaving the source erases nothing — the content
// is one parse away. Nothing in this tree populates the column, which is why
// only a gate will ever notice if this stops: piicoverage_test.go declares the
// assignments this statement IS.
//
// `counterparty_email` and the channel identity (`source_id`, `thread_key`)
// deliberately stay, and that is where this statement parts company with its
// two siblings. The retention action's contract is that the RECORD of the
// meeting survives and its content goes, and who it was with is the record.
// The difference is declared in piicoverage_test.go's retentionKeeps for
// `activity`, so reversing it fails the gate rather than passing silently — the
// data-layer guard `activity_restriction_lift_erases` exists because the same
// kind of difference was once carried in prose and went short.
func (s *RetentionService) eraseActivityContent(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := tx.Exec(ctx,
		// language goes with the text: it was read from the body this statement
		// is emptying, so keeping it would answer one question about content
		// that no longer exists.
		`UPDATE activity SET body = NULL, raw = NULL, subject = $2, language = NULL, archived_at = coalesce(archived_at, now()) WHERE id = $1`,
		id, erasedActivitySubject)
	if err == nil {
		// Everything the text left behind — the verbatim provider original, the
		// vectors, the provenance of fields that are now gone, the transcript
		// readings, the proposals quoting it, the attachments and the
		// transmitted copy.
		//
		// The SAME call the restriction lift and the controller's release make.
		// This list used to live here and a shorter one lived there, and the
		// shorter one was missing the provider original and the quoting
		// proposals — which is what a second list does, whatever the comment
		// beside it promises.
		err = s.eraser.purgeContentDerivedFrom(ctx, tx, id, theClockRanOut)
	}
	if err == nil {
		// Not in the shared helper, and that is the difference between the two
		// arms rather than an omission from one. This arm removes an
		// interaction the relationship aggregates counted, so it re-folds them
		// in the same transaction; the lift paths emit their own events and the
		// bus consumer that handles them is the backstop there. Folding it in
		// would make the shared helper need a seam two of its three callers do
		// not have.
		err = s.invalidateGraph(ctx, tx, id)
	}
	return err
}
