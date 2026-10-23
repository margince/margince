// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// What the repair found, beyond the rule's own outcomes.
const (
	// DealUnknownProvenance is mail the record does not tie to a mailbox, or
	// whose mailbox's seat is gone. It is left alone.
	DealUnknownProvenance DealOutcome = "unknown_provenance"
	// DealHumanKept is mail a colleague filed on a deal. It is never touched.
	DealHumanKept DealOutcome = "human_kept"
	// DealUnknownLink is a deal link with no audit row naming its writer, such
	// as a connector's own ref. Nothing proves it wrong, so it is left alone.
	DealUnknownLink DealOutcome = "unknown_provenance_link"
	// DealMachineConfirmed is a link the rule wrote and still finds.
	DealMachineConfirmed DealOutcome = "machine_confirmed"
	// DealMachineMoved is a link the rule wrote that now goes to another deal.
	DealMachineMoved DealOutcome = "machine_moved"
	// DealMachineRetracted is a link the rule wrote and no longer finds.
	DealMachineRetracted DealOutcome = "machine_retracted"
	// DealMachineRetractedRefused is a link the rule wrote that now belongs
	// on a deal the capturing seat may not file on. The old link is taken
	// back and nothing is written instead.
	DealMachineRetractedRefused DealOutcome = "machine_retracted_target_refused"
)

// MaxDealRepairPreview bounds one preview page, as the contract states.
const MaxDealRepairPreview = 1000

// dealRepairReason is the evidence every repair write records.
const dealRepairReason = "captured_mail_deal_repair"

// capturedMailSQL selects the mail the deal repair inspects: live mail a
// connector captured. Callers append the cursor and order.
const capturedMailSQL = `
	SELECT a.id FROM activity a
	 WHERE a.kind = 'email' AND a.channel_provider IS NULL
	   AND a.captured_by LIKE 'connector:%'
	   AND a.archived_at IS NULL AND a.restricted_at IS NULL`

// dealProvenanceSQL classifies a mail's deal links by their audit rows. It
// answers how many there are and one of them. It answers whether a colleague
// ever filed the mail on a deal, and whether the rule wrote that one link.
var dealProvenanceSQL = `
	SELECT count(l.deal_id), min(l.deal_id::text),
	       ` + provenance.HumanDealFilingSQL("$1") + `,
	       EXISTS (SELECT 1 FROM audit_log m
	                WHERE m.entity_type = 'activity' AND m.entity_id = $1 AND m.action = 'activity_relink'
	                  AND m.after->>'entity_type' = 'deal' AND m.actor_type IN ('connector', 'system')
	                  AND m.after->>'entity_id' = min(l.deal_id::text))
	  FROM activity_link l WHERE l.activity_id = $1 AND l.entity_type = 'deal'`

// DealRepair files mail captured before the deal rule under the deal the rule
// finds for it now, and re-checks the links the rule itself wrote. Each mail
// is decided as the connector of a mailbox that imported it, with that seat's
// live authority. The repair writes as its own actor, with its reason. It
// asks nobody and never touches a link a colleague made.
type DealRepair struct {
	Candidates DealCandidates
	Authority  authz.Resolver
	Filer      DealFiler
}

// BatchTx repairs one page of mail. A second run changes nothing, because
// every mail then decides the way it is filed.
func (r DealRepair) BatchTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned, err := capturedMail(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	changed := 0
	for _, id := range scanned {
		outcome, err := r.repair(ctx, tx, ids.From[ids.ActivityKind](id))
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if outcome.Changed() {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

// DealRepairPreview is what BatchTx would do to one page of captured mail.
type DealRepairPreview struct {
	Outcomes map[DealOutcome]int
	// Last is the last mail inspected; zero for an empty page.
	Last ids.UUID
	// Complete is false when more captured mail follows the page.
	Complete bool
	// Inspected counts the mails the page read.
	Inspected int
}

// PreviewTx is what BatchTx would do to limit mails after after, by outcome,
// decided the same way and rolled back.
func (r DealRepair) PreviewTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (DealRepairPreview, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return DealRepairPreview{}, err
	}
	if limit < 1 || limit > MaxDealRepairPreview {
		return DealRepairPreview{}, apperrors.ErrInvalidArgument
	}
	scanned, err := capturedMail(ctx, tx, after, limit+1)
	if err != nil {
		return DealRepairPreview{}, err
	}
	out := DealRepairPreview{Outcomes: map[DealOutcome]int{}, Complete: len(scanned) <= limit}
	scanned = scanned[:min(len(scanned), limit)]
	if len(scanned) > 0 {
		out.Last = scanned[len(scanned)-1]
	}
	out.Inspected = len(scanned)
	counts := out.Outcomes
	for _, id := range scanned {
		trial, err := tx.Begin(ctx)
		if err != nil {
			return DealRepairPreview{}, fmt.Errorf("capture: opening a deal repair preview: %w", err)
		}
		outcome, err := r.repair(ctx, trial, ids.From[ids.ActivityKind](id))
		if rollback := trial.Rollback(ctx); err == nil && rollback != nil {
			err = rollback
		}
		if err != nil {
			return DealRepairPreview{}, err
		}
		counts[outcome]++
	}
	return out, nil
}

// Changed reports whether the repair wrote for this outcome.
func (o DealOutcome) Changed() bool {
	return o == DealLinked || o == DealMachineMoved || o == DealMachineRetracted || o == DealMachineRetractedRefused
}

// repair classifies one mail by its deal links' provenance and acts on the
// classes the record supports.
//
// The activity row is locked first and held to the end, so a colleague's
// relink cannot land between the classification and the write. A colleague
// who filed the mail and filed it back is still a colleague's choice.
func (r DealRepair) repair(ctx context.Context, tx pgx.Tx, mail ids.ActivityID) (DealOutcome, error) {
	if live, err := lockForDeal(ctx, tx, mail); err != nil || !live {
		return DealNone, err
	}
	var links int
	var linked *string
	var human, machine bool
	if err := tx.QueryRow(ctx, dealProvenanceSQL, mail).Scan(&links, &linked, &human, &machine); err != nil {
		return "", fmt.Errorf("capture: reading a mail's deal provenance: %w", err)
	}
	switch {
	case human:
		return DealHumanKept, nil
	case links == 0:
		return r.asMailbox(ctx, tx, mail, func(f dealFiling) (DealOutcome, error) {
			verdict, err := fileOnDeal(f, tx, mail, r.Candidates, nil)
			return verdict.Outcome, err
		})
	case links == 1 && machine && linked != nil:
		current, err := ids.Parse(*linked)
		if err != nil {
			return "", fmt.Errorf("capture: reading a mail's deal link: %w", err)
		}
		return r.asMailbox(ctx, tx, mail, func(f dealFiling) (DealOutcome, error) {
			return recheckMachineDeal(f, tx, mail, current, r.Candidates)
		})
	default:
		return DealUnknownLink, nil
	}
}

// asMailbox runs decide as each mailbox that imported the mail, as live
// capture runs once per capture. It stops at the first change or confirmation.
func (r DealRepair) asMailbox(
	ctx context.Context, tx pgx.Tx, mail ids.ActivityID, decide func(dealFiling) (DealOutcome, error),
) (DealOutcome, error) {
	var capturedBy string
	if err := tx.QueryRow(ctx, `SELECT captured_by FROM activity WHERE id = $1 AND archived_at IS NULL AND restricted_at IS NULL`, mail).Scan(&capturedBy); err != nil {
		return "", fmt.Errorf("capture: reading who captured a mail: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT user_id FROM capture_import WHERE activity_id = $1 ORDER BY user_id`, mail)
	if err != nil {
		return "", fmt.Errorf("capture: reading the mailboxes a mail came from: %w", err)
	}
	seats, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return "", fmt.Errorf("capture: reading the mailboxes a mail came from: %w", err)
	}
	outcome := DealUnknownProvenance
	name, ok := strings.CutPrefix(capturedBy, "connector:")
	if !ok || name == "" {
		return outcome, nil
	}
	for _, seat := range seats {
		acting, err := connectorActing(ctx, r.Authority, name, ids.From[ids.UserKind](seat))
		if errors.Is(err, apperrors.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", err
		}
		f := dealFiling{decider: acting, writer: ctx, filer: r.Filer, evidence: map[string]any{"reason": dealRepairReason, "decided_as": seat}}
		decided, err := decide(f)
		if err != nil || decided.Changed() || decided == DealMachineConfirmed {
			return decided, err
		}
		outcome = decided
	}
	return outcome, nil
}

// recheckMachineDeal decides a mail the rule filed on current as if it were
// unfiled. A link the rule still finds stays. One it no longer finds is taken
// back, and the deal it finds now, if any, is written instead.
func recheckMachineDeal(f dealFiling, tx pgx.Tx, mail ids.ActivityID, current ids.UUID, candidates DealCandidates) (DealOutcome, error) {
	if live, err := lockForDeal(f.writer, tx, mail); err != nil || !live {
		return DealNone, err
	}
	facts, err := readDealFacts(f.decider, tx, mail, true)
	if err != nil || facts.outcome != "" {
		return facts.outcome, err
	}
	verdict, err := decideDeal(f.decider, tx, facts, candidates)
	if err != nil {
		return "", err
	}
	if verdict.Outcome == DealLinked && verdict.Deal == current {
		return DealMachineConfirmed, nil
	}
	return replaceMachineDeal(f, tx, mail, current, verdict)
}

// replaceMachineDeal takes the rule's old link back and writes the deal the
// rule finds now. The new deal's admission is asked before anything is taken
// back, so a refusal is known and reported, never a move that wrote nothing.
func replaceMachineDeal(f dealFiling, tx pgx.Tx, mail ids.ActivityID, current ids.UUID, verdict DealVerdict) (DealOutcome, error) {
	target := false
	if verdict.Outcome == DealLinked {
		var err error
		if target, err = mayFileOnDeal(f, tx, verdict.Deal); err != nil {
			return "", err
		}
	}
	retracted, err := retractMachineDealLink(f, tx, mail, current)
	if err != nil || !retracted {
		return DealNotWritable, err
	}
	switch {
	case verdict.Outcome != DealLinked:
		return DealMachineRetracted, nil
	case !target:
		return DealMachineRetractedRefused, nil
	}
	written, err := linkActivityToDeal(f, tx, mail, verdict.Deal)
	if err != nil {
		return "", err
	}
	if written != DealLinked {
		return DealMachineRetractedRefused, nil
	}
	return DealMachineMoved, nil
}

func capturedMail(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, capturedMailSQL+` AND a.id > $1 ORDER BY a.id LIMIT $2`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("capture: listing mail for the deal repair: %w", err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("capture: listing mail for the deal repair: %w", err)
	}
	return scanned, nil
}

// EstimateDealRepair bounds the work: how many captured mails the repair will
// inspect, at most maximum. PreviewTx says what it will do with them.
func EstimateDealRepair(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (`+capturedMailSQL+` LIMIT $1) AS candidates`, maximum+1).Scan(&count)
	return min(count, maximum), count > maximum, err
}
