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
  "magic.action.approval_promote_lead",
  "magic.action.approval_overnight",
  "magic.action.approval_transcript_proposal",
  "magic.action.approval_pending",
  "magic.action.capture_reauth_required",
  "magic.action.capture_connection_error",
  "magic.action.capture_sync_failing",
  "magic.action.capture_backfill_failed",
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
  "magic.consequence.awaits_your_decision",
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
const CONSEQUENCES = registry(MAGIC_CONSEQUENCE_KEYS);

/** The sentence key this build carries, or null for one it predates. */
export function magicSentenceKey(key: string): MessageKey | null {
  return SENTENCES.get(key) ?? null;
}

/** The consequence key this build carries, or null for one it predates. */
export function magicConsequenceKey(key: string): MessageKey | null {
  return CONSEQUENCES.get(key) ?? null;
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

/** Why this line cannot be taken back, in words, or null for a reason this build has none for. */
export function magicUndoReasonKey(
  reason: string | undefined,
): MessageKey | null {
  if (!reason) {
    return null;
  }
  return undoRefusalKey(reason) ?? MAGIC_UNDO_REASONS[reason] ?? null;
}
