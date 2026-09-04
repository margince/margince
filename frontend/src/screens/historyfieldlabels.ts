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
// (screens/organizations.tsx) names company profile facts, and today.merge's
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
  ["assignee_id", "history.field.assignee_id"],
  ["body", "history.field.body"],
  ["candidate_org_key", "history.field.candidate_org_key"],
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
  ["organization_id", "history.field.organization_id"],
  ["owner_id", "history.field.owner_id"],
  ["parent_org_id", "history.field.parent_org_id"],
  ["partner_attribution", "history.field.partner_attribution"],
  ["partner_org_id", "history.field.partner_org_id"],
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
  ["wait_until", "history.field.wait_until"],
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
  const key = HISTORY_FIELD_LABELS.get(field);
  return key ? t(key) : field.replaceAll("_", " ");
}

// The same map as a lookup, for the census that holds it against the contract.
export function historyFieldLabelKey(field: string): MessageKey | undefined {
  return HISTORY_FIELD_LABELS.get(field);
}

// Every field this map claims a word for — the census reads it to hold the
// map against the contract in BOTH directions.
export function historyFieldLabelled(): string[] {
  return [...HISTORY_FIELD_LABELS.keys()];
}
