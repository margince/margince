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

// A report card bound to a saved question keeps the question whole, filters
// included, in report_definition_revision.question_bindings. A filter value
// can name a subject by their id, a lead's id or their address. Erasure
// removes every kept question that names the subject, and subject access
// hands back the filters that do. TestABindableFilterNamesNoContactOrLead
// holds that no filter names them by another record's id.

// bindingNames is true when the binding at the JSON path filters on one of the
// values, a text[] expression. Values compare lower-case, as addresses do.
func bindingNames(binding, values string) string {
	filters := binding + "->'query'->'Filters'"
	return `EXISTS (SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(` + filters + `)='array' THEN ` + filters + ` ELSE '[]'::jsonb END) filter
 WHERE lower(filter->>'Value') = ANY(` + values + `))`
}

// subjectBindingValues lists the values a filter can name the subject by:
// their contact id, their addresses, and the ids of the leads that are them.
func subjectBindingValues(contact, emails string) string {
	return `(ARRAY[` + contact + `::uuid::text] || ` + emails + `::text[] || ARRAY(SELECT id::text FROM lead WHERE ` + leadTwinOf(contact, emails) + `))`
}

// scrubQuestionBindings removes every kept question that names one of the
// values from each report revision holding one. Its card then answers no
// reader, as a card whose question the revision does not hold.
//
//craft:ignore naked-any Bound SQL parameters carry pgx's heterogeneous value types.
func scrubQuestionBindings(ctx context.Context, tx pgx.Tx, values string, args ...any) error {
	_, err := tx.Exec(ctx, `UPDATE report_definition_revision r SET question_bindings=(
 SELECT COALESCE(jsonb_object_agg(b.key,b.value),'{}'::jsonb) FROM jsonb_each(r.question_bindings) b WHERE NOT `+bindingNames("b.value", values)+`)
 WHERE EXISTS (SELECT 1 FROM jsonb_each(r.question_bindings) b WHERE `+bindingNames("b.value", values)+`)`, args...)
	return err
}

// scrubSubjectBindings removes every kept question that names the subject.
func scrubSubjectBindings(ctx context.Context, tx pgx.Tx, subject ids.ContactID, emails []string) error {
	args := []any{subject.UUID, emails}
	p := strings.Split(storekit.Placeholders(args), ",")
	return scrubQuestionBindings(ctx, tx, subjectBindingValues(p[0], p[1]), args...)
}

// sarBindingSection hands over each kept question filter that names the
// subject, with the report and revision that keep it. Only the filters naming
// the subject are exported, since the others can name other records.
func sarBindingSection(pkg *SARPackage, contactID ids.ContactID, emails []string) sarSection {
	args := []any{contactID.UUID, emails}
	p := strings.Split(storekit.Placeholders(args), ",")
	values := subjectBindingValues(p[0], p[1])
	filters := "b.value->'query'->'Filters'"
	return sarSection{&pkg.ReportingQuestionBindings, `SELECT r.report_id,r.revision,b.key AS binding,
 (SELECT jsonb_agg(filter) FROM jsonb_array_elements(CASE WHEN jsonb_typeof(` + filters + `)='array' THEN ` + filters + ` ELSE '[]'::jsonb END) filter
  WHERE lower(filter->>'Value') = ANY(` + values + `)) AS filters
 FROM report_definition_revision r CROSS JOIN LATERAL jsonb_each(r.question_bindings) b WHERE ` + bindingNames("b.value", values), args}
}
