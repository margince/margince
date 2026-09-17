import type { MessageKey } from "../i18n/en";

// What a changed field is CALLED, for a reader who never sees a column name
// anywhere else on the record.
//
// The audit spine projects WIRE names — `amount_minor`, `owner_id` — because a
// contract shape carries the name the API binds, not a display label. The
// record page beside it names the same fields in the reader's own language, so
// a history that printed the column left one surface speaking two vocabularies
// about one field.
//
// It is its own map rather than a reuse of the three that already exist,
// because none of them is about this subject: `COLD_FIELD_LABELS`
// (screens/common.tsx) names the enrichment vocabulary, `PROFILE_FIELD_LABELS`
// (screens/companies.tsx) names company profile facts, and today.merge's
// map names the fields a merge compares. This one names the columns a record's
// UPDATE writes, and `historyfieldlabels.test.ts` derives that set from the
// contract so a field added upstream fails the gate instead of reaching a
// reader raw.
//
// A Map, not an object: `get` answers `MessageKey | undefined` with no cast,
// and no prototype key can answer for a field the server never sent.
const HISTORY_FIELD_LABELS = new Map<string, MessageKey>([
  ["address", "history.field.address"],
  ["amount_minor", "history.field.amount_minor"],
  ["expected_arr_minor", "history.field.expected_arr_minor"],
  ["assignee_id", "history.field.assignee_id"],
  ["body", "history.field.body"],
  ["candidate_company_key", "history.field.candidate_company_key"],
  ["company_name", "history.field.company_name"],
  ["currency", "history.field.currency"],
  ["description", "history.field.description"],
  ["display_name", "history.field.display_name"],
  ["domains", "history.field.domains"],
  ["due_at", "history.field.due_at"],
  ["email", "history.field.email"],
  ["emails", "history.field.emails"],
  ["ended_at", "history.field.ended_at"],
  ["expected_close_date", "history.field.expected_close_date"],
  ["first_name", "history.field.first_name"],
  ["forecast_category", "history.field.forecast_category"],
  ["full_name", "history.field.full_name"],
  ["fx_rate_date", "history.field.fx_rate_date"],
  ["fx_rate_to_base", "history.field.fx_rate_to_base"],
  ["industry", "history.field.industry"],
  ["is_done", "history.field.is_done"],
  ["last_name", "history.field.last_name"],
  ["legal_name", "history.field.legal_name"],
  ["lifecycle", "history.field.lifecycle"],
  ["linkedin_url", "history.field.linkedin_url"],
  ["lost_reason", "history.field.lost_reason"],
  ["meeting_status", "history.field.meeting_status"],
  ["name", "history.field.name"],
  ["occurred_at", "history.field.occurred_at"],
  ["company_id", "history.field.company_id"],
  ["owner_id", "history.field.owner_id"],
  ["parent_company_id", "history.field.parent_company_id"],
  ["partner_attribution", "history.field.partner_attribution"],
  ["partner_company_id", "history.field.partner_company_id"],
  ["phones", "history.field.phones"],
  ["project_id", "history.field.project_id"],
  ["relationship_types", "history.field.relationship_types"],
  ["remind_at", "history.field.remind_at"],
  ["score", "history.field.score"],
  ["score_override_reason", "history.field.score_override_reason"],
  ["size_band", "history.field.size_band"],
  ["social", "history.field.social"],
  ["source", "history.field.source"],
  ["started_at", "history.field.started_at"],
  ["status", "history.field.status"],
  ["subject", "history.field.subject"],
  ["target_end_date", "history.field.target_end_date"],
  ["title", "history.field.title"],
  ["visibility", "history.field.visibility"],
  ["wait_until", "history.field.wait_until"],
  ["description", "history.field.description"],
  ["commercial_motion", "history.field.commercial_motion"],
  ["priority", "history.field.priority"],
  ["acquisition_source", "history.field.acquisition_source"],
]);

// Fields a HAND-BUILT audit image names — a write described by its own
// free-form keys rather than by the before/after image of a contract column.
// They come from all over the tree: a consent withdrawal, a VAT consultation, a
// bounce report, a cohort repair, a reply verdict.
//
// It is a SEPARATE map from HISTORY_FIELD_LABELS on purpose. The census below
// derives that one from what an `Update<Type>Request` actually writes, and its
// own "no word for an unwritten field" direction would fail the moment one of
// these appeared there.
//
// Held by TestEveryProjectedAuditKeyHasALabel
// (backend/gates/historyfieldlabels_test.go), which walks the audit doors,
// resolves the images they carry, and fails in both directions: a writer's key
// with no word here, and a word here no writer emits. It also refuses a key
// this map and the contract map both claim unless that collision is ratified —
// the shape `kind` took, which already belongs to every activity's own audited
// create and would have mislabelled every activity in history as a suppression.
// That is why a key here is not the bare word a writer's struct field suggests:
// this lookup carries no entity context, so `suppression_kind` rather than
// `kind`.
const SYNTHETIC_AUDIT_FIELD_LABELS = new Map<string, MessageKey>([
  ["admission", "history.field.admission"],
  ["admission_reason", "history.field.admission_reason"],
  ["admission_source", "history.field.admission_source"],
  ["bounce", "history.field.bounce"],
  ["capture_question", "history.field.capture_question"],
  ["channel_identity", "history.field.channel_identity"],
  ["channel_username", "history.field.channel_username"],
  ["cohort_linked", "history.field.cohort_linked"],
  ["cohort_promoted", "history.field.cohort_promoted"],
  ["communication_basis", "history.field.communication_basis"],
  ["confirm_submission", "history.field.confirm_submission"],
  ["corrected", "history.field.corrected"],
  ["decided_by_level", "history.field.decided_by_level"],
  ["disposition", "history.field.disposition"],
  ["domain", "history.field.domain"],
  ["lifted_by", "history.field.lifted_by"],
  ["lifted_by_level", "history.field.lifted_by_level"],
  ["lifted_suppression", "history.field.lifted_suppression"],
  ["note", "history.field.note"],
  ["nudge_dismissal", "history.field.nudge_dismissal"],
  ["provider_claims_received", "history.field.provider_claims_received"],
  ["qualifying_event", "history.field.qualifying_event"],
  ["reachability", "history.field.reachability"],
  ["reason", "history.field.reason"],
  ["recorded_at_level", "history.field.recorded_at_level"],
  ["reply_verdict", "history.field.reply_verdict"],
  ["reply_verdict_by", "history.field.reply_verdict_by"],
  ["research_claims_accepted", "history.field.research_claims_accepted"],
  ["resolved_category", "history.field.resolved_category"],
  ["stopped", "history.field.stopped"],
  ["stops_carried", "history.field.stops_carried"],
  ["submission_decision", "history.field.submission_decision"],
  ["submission_id", "history.field.submission_id"],
  ["suppression_kind", "history.field.suppression_kind"],
  ["vat_checked_at", "history.field.vat_checked_at"],
  ["vat_consultation_number", "history.field.vat_consultation_number"],
  ["vat_number", "history.field.vat_number"],
  ["vat_registered_address", "history.field.vat_registered_address"],
  ["vat_registered_name", "history.field.vat_registered_name"],
  ["vat_requested", "history.field.vat_requested"],
  ["vat_status", "history.field.vat_status"],
]);

// The label a history row shows for one field.
//
// A field with no key falls back to its own name with the underscores spaced
// out — the same contract `coldFieldLabel` keeps, and the only honest answer
// for a workspace's own `cf_` column, whose label lives in the custom-field
// catalog rather than in any catalog this build ships.
export function historyFieldLabel(
  field: string,
  t: (key: MessageKey) => string,
): string {
  const key =
    HISTORY_FIELD_LABELS.get(field) ?? SYNTHETIC_AUDIT_FIELD_LABELS.get(field);
  return key ? t(key) : field.replaceAll("_", " ");
}

// The same map as a lookup, for the census that holds it against the contract.
export function historyFieldLabelKey(field: string): MessageKey | undefined {
  return HISTORY_FIELD_LABELS.get(field);
}

// The synthetic map's own keys, for the test that holds each one to an i18n
// key that actually exists — the one direction a Go-literal vocabulary can
// still be checked from this side of the contract.
export function syntheticAuditFieldLabelled(): string[] {
  return [...SYNTHETIC_AUDIT_FIELD_LABELS.keys()];
}

// Every field this map claims a word for — the census reads it to hold the
// map against the contract in BOTH directions.
export function historyFieldLabelled(): string[] {
  return [...HISTORY_FIELD_LABELS.keys()];
}
