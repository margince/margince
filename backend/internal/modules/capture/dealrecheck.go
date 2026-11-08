// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The historical recheck: a deal link the rule wrote on a suggestion, before
// the topic check existed, is asked about once. A confident "not about it"
// takes the link back, audited. A fit or doubt keeps it and is remembered. No
// answer keeps it. A colleague's filing or decline is never touched.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DealRecheckWindowDays bounds the recheck to mail of the last 90 days, by
// when it was sent or held.
const DealRecheckWindowDays = 90

const dealRecheckReason = "captured_activity_deal_recheck"

// RecheckCandidatesTx is the captured activities after after, up to ceiling,
// that carry one deal link, are within the window of asOf, and were never
// topic-checked. It returns at most limit of them, in id order.
func (r DealRepair) RecheckCandidatesTx(ctx context.Context, tx pgx.Tx, asOf time.Time, after, ceiling ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`SELECT a.id FROM activity a
 WHERE a.kind IN ('email','meeting') AND %s
 AND a.occurred_at >= $%d::timestamptz - make_interval(days => %d) -- occurred_at raw: an age of whole days, the recheck window
 AND a.deal_topic_digest IS NULL AND a.id > $%d AND a.id <= $%d
 AND (SELECT count(*) FROM activity_link l WHERE l.activity_id = a.id AND l.entity_type = 'deal') = 1
 ORDER BY a.id LIMIT $%d`, capturedConnectorActivitySQL, arg(asOf), DealRecheckWindowDays, arg(after), arg(ceiling), arg(limit))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("capture: listing deal links to recheck: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// Recheck asks the topic check about one activity's machine deal link, in its
// own transactions, and reports whether it asked the model. Without a Check
// it does nothing.
func (r DealRepair) Recheck(ctx context.Context, db TxRunner, activity ids.ActivityID) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil || r.Check == nil {
		return false, err
	}
	return checkedFiling(ctx, db, r.Check, func(tx pgx.Tx, topic *dealTopicState) (*topicQuestion, error) {
		return r.recheckTx(ctx, tx, activity, topic)
	})
}

// recheckTx is one transaction of Recheck, which passes it a topic state.
func (r DealRepair) recheckTx(ctx context.Context, tx pgx.Tx, mail ids.ActivityID, topic *dealTopicState) (*topicQuestion, error) {
	if live, err := lockForDeal(ctx, tx, mail); err != nil || !live {
		return nil, err
	}
	links, linked, human, machine, err := readDealProvenance(ctx, tx, mail)
	if err != nil || human || links != 1 || !machine || linked == nil {
		return nil, err
	}
	current, err := ids.Parse(*linked)
	if err != nil {
		return nil, fmt.Errorf("capture: reading a mail's deal link: %w", err)
	}
	var question *topicQuestion
	_, err = r.asMailbox(ctx, tx, mail, dealRecheckReason, func(f dealFiling) (DealOutcome, error) {
		f.topic = topic
		outcome, asked, err := r.recheckTopic(f, tx, mail, current)
		question = asked
		return outcome, err
	})
	return question, err
}

// recheckTopic settles one suggested link by the check's answer. Only a
// confident "not about it" takes the link back. Doubt keeps it, and is
// remembered like a fit, so the recheck asks about it once.
func (r DealRepair) recheckTopic(f dealFiling, tx pgx.Tx, mail ids.ActivityID, current ids.UUID) (DealOutcome, *topicQuestion, error) {
	suggested, err := r.suggestedLink(f, tx, mail, current)
	if err != nil || !suggested {
		return DealNone, nil, err
	}
	deals := []ids.UUID{current}
	digest, err := f.topic.check.Digest(f.decider, tx, mail, deals)
	if err != nil || digest == "" {
		return DealNone, nil, err
	}
	answer := f.topic.answer
	switch {
	case answer == nil:
		return dealTopicPending, &topicQuestion{decider: f.decider, mail: mail, deals: deals, digest: digest}, nil
	case answer.digest != digest || !answer.answered:
		return DealNone, nil, nil
	case !answer.topic.Confident || !answer.topic.Deal.IsZero():
		return DealMachineConfirmed, nil, f.filer.StampDealTopic(f.writer, tx, mail, digest)
	}
	retracted, err := retractMachineDealLink(f, tx, mail, current)
	if err != nil || !retracted {
		return DealNone, nil, err
	}
	return DealMachineRetracted, nil, f.filer.StampDealTopic(f.writer, tx, mail, digest)
}

// suggestedLink answers whether the rule's link on current rests on a
// suggestion. Where the rule still files the mail there, its basis says so.
// Where it no longer reaches current, as for a closed deal, the link is a
// suggestion unless the mail's project or proven thread names it.
func (r DealRepair) suggestedLink(f dealFiling, tx pgx.Tx, mail ids.ActivityID, current ids.UUID) (bool, error) {
	facts, err := readDealFacts(f.decider, tx, mail, true)
	if err != nil || facts.outcome != "" {
		return false, err
	}
	verdict, err := decideDeal(f.decider, tx, facts, r.Candidates)
	if err != nil {
		return false, err
	}
	if verdict.Outcome == DealLinked && verdict.Deal == current {
		return verdict.Basis.NeedsTopicCheck(), nil
	}
	explicit, err := explicitDealEvidence(f.decider, tx, mail.UUID, current)
	return !explicit, err
}

// explicitDealEvidence answers whether the mail's project, or a proven mail of
// its conversation, files it on deal. A seat that may not read deals reads it
// as evidence, so nothing is taken back on what it cannot see.
func explicitDealEvidence(ctx context.Context, tx pgx.Tx, mail, deal ids.UUID) (bool, error) {
	if !auth.ReadGranted(ctx, "deal") {
		return true, nil
	}
	var args []any
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	mailArg, dealArg := arg(mail), arg(deal)
	var explicit bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity_link l JOIN deal d ON d.project_id = l.project_id
		   WHERE l.activity_id = `+mailArg+` AND l.entity_type = 'project' AND d.id = `+dealArg+`)
		 OR EXISTS (SELECT 1 FROM activity self
		   JOIN activity a ON a.id <> self.id AND `+correspondence.ConversationSQL("a", "self")+`
		   JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = `+dealArg+`
		   WHERE self.id = `+mailArg+` AND `+correspondence.ProvenSQL("self")+` AND `+correspondence.ProvenSQL("a")+`
		     AND a.restricted_at IS NULL)`, args...).Scan(&explicit)
	if err != nil {
		return false, fmt.Errorf("capture: reading the evidence for a deal link: %w", err)
	}
	return explicit, nil
}
