// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";
import { undoRefusalKey } from "./historyundo";

// The client half of a mirror: this registry, `en.ts` and the keys the receipt
// service emits name exactly the same sentences, and a gate in the backend
// fails in both directions when they stop doing so.
//
// The server sends a KEY because the product ships three languages, and a
// sentence composed server-side reaches a German reader in English. What it
// cannot send is a promise that this build carries the key: a newer server
// mints one this client predates, and rendering an unknown key resolves to the
// key itself on screen. So a key is NARROWED through the closed set below and
// an unrecognised one yields null, which the row draws as a missing sentence
// rather than as a raw identifier at a reader.

/** Every sentence `/magic` can send for a line. */
export const MAGIC_SENTENCE_KEYS = [
  "magic.action.forecast_changed",
  "magic.action.advance_stage",
  "magic.action.promote",
  "magic.action.update",
  "magic.action.assign",
  "magic.action.activity_relink",
  "magic.action.send_email",
  "magic.action.schedule",
  "magic.action.disqualify",
  "magic.action.automation_troubled",
  "magic.action.approval_coldstart",
  "magic.action.approval_send_email",
  "magic.action.approval_advance_deal",
  "magic.action.approval_deal_proposal_stage",
  "magic.action.approval_promote_lead",
  "magic.action.approval_overnight",
  "magic.action.approval_commitment_task",
  "magic.action.approval_commitment_task_named",
  "magic.action.approval_commitment_task_summary",
  "magic.action.approval_commitment_task_unverified",
  "magic.action.approval_capture_counterparty",
  "magic.action.approval_pending",
  "magic.action.approval_summary",
  "magic.action.decided_summary",
  "magic.action.decided_kind",
  "magic.action.capture_reauth_required",
  "magic.action.capture_connection_error",
  "magic.action.capture_sync_failing",
  "magic.action.capture_sync_stalled",
  "magic.action.capture_silent",
  "magic.action.capture_backfill_failed",
  "magic.action.mail_filed",
  "magic.action.company_profile_read",
  "magic.action.fields_changed",
  "magic.action.retention_lead_anonymize",
  "magic.action.retention_lead_archive",
  "magic.action.retention_contact_anonymize",
  "magic.action.retention_contact_erase",
  "magic.action.retention_activity_archive",
  "magic.action.retention_activity_erase",
  "magic.action.retention_deal_archive",
  "magic.action.create_contact",
  "magic.action.create_company",
  "magic.action.create_deal",
  "magic.action.create_lead",
  "magic.action.create_project",
  "magic.action.create_activity",
  "magic.action.create_activity_email",
  "magic.action.create_activity_meeting",
  "magic.action.create_activity_call",
  "magic.action.create_activity_note",
  "magic.action.create_activity_task",
  "magic.action.create_activity_message",
  "magic.action.archive_contact",
  "magic.action.archive_company",
  "magic.action.archive_deal",
  "magic.action.archive_lead",
  "magic.action.archive_project",
  "magic.action.create_offer",
  "magic.action.update_offer",
  "magic.action.archive_offer",
  "magic.action.create_contract",
  "magic.action.update_contract",
  "magic.action.archive_contract",
  "magic.action.create_deal_room",
  "magic.action.update_deal_room",
  "magic.action.archive_deal_room",
  "magic.action.create_stage_evidence",
  "magic.action.update_stage_evidence",
  "magic.action.archive_stage_evidence",
  "magic.action.create_introduction",
  "magic.action.update_introduction",
  "magic.action.archive_introduction",
  "magic.action.create_assignment",
  "magic.action.update_assignment",
  "magic.action.archive_assignment",
  "magic.action.create_attachment",
  "magic.action.update_attachment",
  "magic.action.archive_attachment",
  "magic.action.create_deal_suggestion",
  "magic.action.update_deal_suggestion",
  "magic.action.schedule_send",
  "magic.action.update_scheduled_send",
  "magic.action.create_invoice",
  "magic.action.update_invoice",
  "magic.action.create_payment",
  "magic.action.update_payment",
  "magic.action.archive_activity",
  "magic.action.archive_activity_email",
  "magic.action.archive_activity_meeting",
  "magic.action.archive_activity_call",
  "magic.action.archive_activity_note",
  "magic.action.archive_activity_task",
  "magic.action.archive_activity_message",
] as const satisfies readonly MessageKey[];

export type MagicSentenceKey = (typeof MAGIC_SENTENCE_KEYS)[number];

/** Every reason `/magic` can give for a line: why the machinery did it. */
export const MAGIC_WHY_KEYS = [
  "magic.why.forecast_review",
  "magic.why.forecast_date",
  "magic.why.forecast_date_basis",
  "magic.why.mail_filed",
  "magic.why.data_correction",
  "magic.why.public_records",
  "magic.why.site_read",
  "magic.why.site_read_unnamed",
  "magic.why.site_read_each",
  "magic.why.signature",
  "magic.why.retention",
] as const satisfies readonly MessageKey[];

/** Every name `/magic` or Done for you can give whoever acted. */
export const MAGIC_BY_KEYS = [
  "magic.by.retention",
  "magic.by.mail_filing",
  "magic.by.company_lookup",
  "magic.by.website_reader",
  "magic.by.signature_reader",
  "magic.by.overnight_agent",
  "magic.by.mail_reader",
  "magic.by.auto_apply",
  "magic.by.automation",
  "magic.by.lead_routing",
  "magic.by.mailbox",
  "magic.by.agent",
  "magic.by.system",
  "magic.by.data_correction",
  "magic.by.member",
  "magic.by.colleague",
] as const satisfies readonly MessageKey[];

/** Every consequence `/magic` can send for a line. */
export const MAGIC_CONSEQUENCE_KEYS = [
  "magic.consequence.stage_moved",
  "magic.consequence.lead_promoted",
  "magic.consequence.owner_changed",
  "magic.consequence.record_relinked",
  "magic.consequence.message_sent",
  "magic.consequence.meeting_booked",
  "magic.consequence.lead_disqualified",
  "magic.consequence.automation_did_nothing",
  "magic.consequence.capture_not_collecting",
  "magic.consequence.capture_may_be_incomplete",
  "magic.consequence.capture_history_incomplete",
] as const satisfies readonly MessageKey[];

// A lookup rather than a `Set` plus an assertion: `has` narrows nothing, so a
// caller would need a cast to hand the value on as a key, and the map's own
// VALUE is already that key with its type intact.
function registry(
  keys: readonly MessageKey[],
): ReadonlyMap<string, MessageKey> {
  return new Map(keys.map((key): [string, MessageKey] => [key, key]));
}

const SENTENCES = registry(MAGIC_SENTENCE_KEYS);
const WHYS = registry(MAGIC_WHY_KEYS);
const BYS = registry(MAGIC_BY_KEYS);
const CONSEQUENCES = registry(MAGIC_CONSEQUENCE_KEYS);

/** The sentence key this build carries, or null for one it predates. */
export function magicSentenceKey(key: string): MessageKey | null {
  return SENTENCES.get(key) ?? null;
}

/** The reason key this build carries, or null for one it predates. */
export function magicWhyKey(key: string): MessageKey | null {
  return WHYS.get(key) ?? null;
}

/** The consequence key this build carries, or null for one it predates. */
export function magicConsequenceKey(key: string): MessageKey | null {
  return CONSEQUENCES.get(key) ?? null;
}

/** The actor-name key this build carries, or null for one it predates. */
export function magicByKey(key: string): MessageKey | null {
  return BYS.get(key) ?? null;
}

// The two refusals this surface mints for itself: a lane whose rows never
// changed anything, and a build with no undo evaluator wired. Every other
// reason a line carries is the record history's own vocabulary, which already
// has words — so this reaches for that map instead of keeping a second copy of
// eleven sentences that would drift the first time one was reworded.
const MAGIC_UNDO_REASONS: Readonly<Record<string, MessageKey | undefined>> = {
  no_completed_change: "magic.undoReason.noCompletedChange",
  undo_not_evaluated: "magic.undoReason.notEvaluated",
};

// Refusals about the reader, not the change. A receipt says what happened to
// the reader's work; that they may not write it is no news on every row.
const READER_REFUSALS: ReadonlySet<string> = new Set([
  "not_writable_by_caller",
  "restore_archiver_required",
]);

/** Why this line cannot be taken back, in words, or null for a reason this build has none for or does not say. */
export function magicUndoReasonKey(
  reason: string | undefined,
): MessageKey | null {
  if (!reason || READER_REFUSALS.has(reason)) {
    return null;
  }
  return undoRefusalKey(reason) ?? MAGIC_UNDO_REASONS[reason] ?? null;
}
