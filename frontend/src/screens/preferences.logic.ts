import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

export type PurposeView =
  components["schemas"]["PreferenceCenter"]["purposes"][number];

// key → subscribed. The toggle is binary; the wire state is ternary.
export type Draft = Record<string, boolean>;

// Default-deny: only an explicit grant is a subscription. "unknown" (no
// record) and "withdrawn" both mean we may not send, so both read as off.
export function displayOn(state: PurposeView["state"]): boolean {
  return state === "granted";
}

export function initialDraft(purposes: PurposeView[]): Draft {
  const draft: Draft = {};
  for (const purpose of purposes) {
    draft[purpose.key] = displayOn(purpose.state);
  }
  return draft;
}

// Locked purposes are excluded unconditionally: the server refuses them and the
// toggle is disabled, so a draft that claims one moved is noise, never a pending
// change.
//
// A double-opt-in purpose is excluded only in the GRANT direction, and for a
// different reason: the server refuses that write too, but the withdrawal
// beside it is honoured. Filtering the whole purpose would strip somebody's
// unsubscribe; filtering neither would submit a choice that 422s and lose the
// rest of the save with it.
export function dirtyKeys(purposes: PurposeView[], draft: Draft): string[] {
  return purposes
    .filter((purpose) => !purpose.locked)
    .filter(
      (purpose) => !(purpose.grant_needs_confirmation && draft[purpose.key]),
    )
    .filter((purpose) => draft[purpose.key] !== displayOn(purpose.state))
    .map((purpose) => purpose.key);
}

// Only changed purposes become choices. Each choice appends an immutable
// proof row, so submitting an untouched purpose would fabricate a decision
// the subject never made. `wording` carries the exact sentence rendered at
// the toggle — the wording shown IS the wording stored.
export function toChoices(
  purposes: PurposeView[],
  draft: Draft,
  wordingOf: (key: string) => string,
): Array<{
  purpose_key: string;
  state: "granted" | "withdrawn";
  wording: string;
}> {
  const changed = new Set(dirtyKeys(purposes, draft));
  return purposes
    .filter((purpose) => changed.has(purpose.key))
    .map((purpose) => ({
      purpose_key: purpose.key,
      state: draft[purpose.key] ? ("granted" as const) : ("withdrawn" as const),
      wording: wordingOf(purpose.key),
    }));
}

/**
 * What to CALL each purpose the product seeds itself.
 *
 * `purpose.label` comes from the database, where the seeded catalog is
 * written in English — so a German page printed "Business
 * correspondence" inside German prose, and the generic wording sentence
 * wrapped an English noun in German quotes: „Business correspondence
 * senden."
 *
 * An operator may define purposes of their own, and those keep whatever
 * they were named: this map covers the three the product plants, which
 * are the ones every installation has and the only ones whose English is
 * ours rather than a customer's.
 */
export const PURPOSE_LABEL_KEYS: Record<string, MessageKey> = {
  business_correspondence: "prefs.purpose.business_correspondence",
  marketing_email: "prefs.purpose.marketing_email",
  transactional: "prefs.purpose.transactional",
};

/** The purpose's name in the reader's language, or the catalog's own. */
export function labelOf(
  t: (key: MessageKey, vars?: Record<string, string>) => string,
  purpose: PurposeView,
): string {
  const key = PURPOSE_LABEL_KEYS[purpose.key];
  return key ? t(key) : purpose.label;
}
