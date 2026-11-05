// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A sent edition keeps each question card's answer with the records behind
// every answer row. These are the four record types an analytics question
// counts, and retention, subject access and erasure reach the answers through
// them.

// heldQuestionSource is true while any record behind one of edition's kept
// answers, or a record it hangs on, is under a legal hold.
func heldQuestionSource() string {
	return `EXISTS(SELECT 1 FROM report_edition_question_source qs
 LEFT JOIN deal qd ON qs.source_type='deal' AND qd.id=qs.source_id
 LEFT JOIN company qdc ON qdc.id=qd.company_id
 LEFT JOIN company qdp ON qdp.id=qd.partner_company_id
 LEFT JOIN lead ql ON qs.source_type='lead' AND ql.id=qs.source_id
 LEFT JOIN project qp ON qs.source_type='project' AND qp.id=qs.source_id
 WHERE qs.edition_id=edition.id AND (
 COALESCE(qd.legal_hold,false) OR COALESCE(qdc.legal_hold,false) OR COALESCE(qdp.legal_hold,false)
 OR COALESCE(ql.legal_hold,false) OR COALESCE(qp.legal_hold,false)
 OR EXISTS(SELECT 1 FROM company pc WHERE pc.legal_hold AND (pc.id=qp.company_id
 OR pc.id IN (SELECT company_id FROM relationship WHERE project_id=qp.id AND kind='project_company')))
 OR EXISTS(SELECT 1 FROM relationship rel JOIN contact subject ON subject.id=rel.contact_id
 WHERE ((rel.deal_id=qd.id AND rel.kind='deal_stakeholder') OR (rel.project_id=qp.id AND rel.kind='project_stakeholder')) AND subject.legal_hold)
 OR EXISTS(SELECT 1 FROM contact lc WHERE lc.id IN (ql.promoted_contact_id, ql.from_contact_id) AND lc.legal_hold)
 OR (qs.source_type='activity' AND EXISTS(SELECT 1 FROM activity_link l
 LEFT JOIN contact ac ON ac.id=l.contact_id LEFT JOIN company ao ON ao.id=l.company_id
 LEFT JOIN deal ad ON ad.id=l.deal_id LEFT JOIN lead al ON al.id=l.lead_id LEFT JOIN project ap ON ap.id=l.project_id
 WHERE l.activity_id=qs.source_id AND (COALESCE(ac.legal_hold,false) OR COALESCE(ao.legal_hold,false)
 OR COALESCE(ad.legal_hold,false) OR COALESCE(al.legal_hold,false) OR COALESCE(ap.legal_hold,false))))))`
}

// subjectQuestionSources selects the kept answer records that are about the
// contact bound as contact, whose addresses are bound as emails.
func subjectQuestionSources(contact, emails string) string {
	return `(qs.source_type='deal' AND qs.source_id IN (SELECT deal_id FROM relationship WHERE contact_id=` + contact + ` AND kind='deal_stakeholder')) OR
 (qs.source_type='project' AND qs.source_id IN (SELECT project_id FROM relationship WHERE contact_id=` + contact + ` AND kind='project_stakeholder')) OR
 (qs.source_type='activity' AND qs.source_id IN (SELECT activity_id FROM activity_link WHERE contact_id=` + contact + `)) OR
 (qs.source_type='lead' AND qs.source_id IN (SELECT id FROM lead WHERE ` + leadTwinOf(contact, emails) + `))`
}

// leadTwinOf selects the leads that are the subject bound as contact, before or
// after promotion, merge or conversion, or by their address in emails. These are
// the leads anonymizeLeadTwins wipes. Its statement keeps its own copy, since
// waivers key on that statement's text, and a test in this package holds the
// two equal.
func leadTwinOf(contact, emails string) string {
	return `promoted_contact_id = ` + contact + `
		     OR from_contact_id = ` + contact + `
		     OR from_contact_id IN (SELECT id FROM contact WHERE merged_into_id = ` + contact + `)
		     OR id IN (SELECT converted_from_lead_id FROM contact WHERE id = ` + contact + ` AND converted_from_lead_id IS NOT NULL)
		     OR (email IS NOT NULL AND lower(email) = ANY(` + emails + `))`
}

// sarQuestionSection hands over each kept answer row a record about the
// subject was counted in.
func sarQuestionSection(pkg *SARPackage, contactID ids.ContactID, emails []string) sarSection {
	args := []any{contactID.UUID, emails}
	p := strings.Split(storekit.Placeholders(args), ",")
	return sarSection{&pkg.ReportingQuestionAnswers, `SELECT DISTINCT q.entity,q.answer->'rows'->qs.row_index AS answer_row,e.captured_at,e.redacted_at
 FROM report_edition_question_source qs
 JOIN report_edition_question q ON q.edition_id=qs.edition_id AND q.card_id=qs.card_id
 JOIN report_edition e ON e.id=qs.edition_id WHERE ` + subjectQuestionSources(p[0], p[1]), args}
}

// redactSubjectQuestions removes every kept answer a record about the subject
// was counted in, from each edition holding one.
func redactSubjectQuestions(ctx context.Context, tx pgx.Tx, subject ids.ContactID, emails []string) error {
	args := []any{subject.UUID, emails}
	p := strings.Split(storekit.Placeholders(args), ",")
	if err := redactQuestionEditions(ctx, tx, `SELECT DISTINCT qs.edition_id FROM report_edition_question_source qs WHERE `+subjectQuestionSources(p[0], p[1]), args...); err != nil {
		return err
	}
	return scrubSubjectBindings(ctx, tx, subject, emails)
}

// redactQuestionSource removes every kept answer one of these records was
// counted in.
//
//craft:ignore naked-any Bound SQL parameters carry pgx's heterogeneous value types.
func redactQuestionSource(ctx context.Context, tx pgx.Tx, kind string, sources []ids.UUID) error {
	if len(sources) == 0 {
		return nil
	}
	p := strings.Split(storekit.Placeholders([]any{kind, sources}), ",")
	return redactQuestionEditions(ctx, tx, `SELECT DISTINCT edition_id FROM report_edition_question_source WHERE source_type=`+p[0]+` AND source_id=ANY(`+p[1]+`)`, kind, sources)
}

// redactQuestionEditions deletes all of each edition's kept answers and marks
// it redacted. A whole edition's answers go, since a row's neighbours can
// restate what the removed row held.
//
//craft:ignore naked-any Bound SQL parameters carry pgx's heterogeneous value types.
func redactQuestionEditions(ctx context.Context, tx pgx.Tx, editions string, args ...any) error {
	if err := storekit.FenceReportProjection(ctx, tx); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, editions, args...)
	if err != nil {
		return err
	}
	reached, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil || len(reached) == 0 {
		return err
	}
	p := storekit.Placeholders([]any{reached})
	if _, err := tx.Exec(ctx, "DELETE FROM report_edition_question WHERE edition_id=ANY("+p+")", reached); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "UPDATE report_edition SET redacted_at=COALESCE(redacted_at,now()) WHERE id=ANY("+p+")", reached)
	return err
}
