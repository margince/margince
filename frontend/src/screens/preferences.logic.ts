import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

export type PurposeView =
  components["schemas"]["PreferenceCenter"]["purposes"][number];

// key → subscribed. The toggle is binary; the wire state is ternary.
export type Draft = Record<string, boolean>;

/**
 * Whether this row reads as ON.
 *
 * It answers from the server's `choice`, never from the raw state, and
 * the difference is the whole reason that field exists. `state` is
 * `unknown` both for a marketing lane nobody opted into and for direct
 * correspondence nobody has objected to — off in the first case, ON in
 * the second — so a page reading the raw value told a reader they were
 * "not subscribed" to ordinary replies their sender is entitled to send
 * under legitimate interest, and showed an always-on lane as switched
 * off.
 *
 * Only an explicit objection is off. That is the shape of the law the
 * engine applies: consent lanes need a grant (which is what makes their
 * choice `opted_out` until one exists), while correspondence and
 * transactional mail run on a basis the recipient may object to rather
 * than one they must first give.
 */
export function displayOn(purpose: PurposeView): boolean {
  return purpose.choice !== "opted_out";
}

export function initialDraft(purposes: PurposeView[]): Draft {
  const draft: Draft = {};
  for (const purpose of purposes) {
    // A locked row is on regardless of what the record says, so the draft
    // starts where the row is drawn rather than where its raw state sits.
    draft[purpose.key] = purpose.locked || displayOn(purpose);
  }
  return draft;
}

// Locked purposes are excluded unconditionally: the server refuses them and the
// toggle is disabled, so a draft that claims one moved is noise, never a pending
// change.
//
// A DOUBLE-OPT-IN GRANT IS NO LONGER EXCLUDED, and that exclusion is worth
// remembering rather than just deleting. It was correct while the server
// refused such a write: submitting one would 422 and lose the rest of the save
// with it, so the grant was stripped and the withdrawal beside it kept.
//
// The server now takes that choice and mails a confirmation link instead of
// refusing it. Stripping it here would mean the subject ticked the box, pressed
// save, and nothing at all happened — the same dead end the old block was
// built to be honest about, arriving one layer up.
export function dirtyKeys(purposes: PurposeView[], draft: Draft): string[] {
  return purposes
    .filter((purpose) => !purpose.locked)
    .filter((purpose) => draft[purpose.key] !== displayOn(purpose))
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

/**
 * The sentence under a row, naming what is true rather than guessing.
 *
 * Three states, not two. "Subscribed" and "not subscribed" only describe
 * a lane somebody opts INTO; applied to direct correspondence they told a
 * reader they were "not subscribed" to the ordinary replies their sender
 * is entitled to send, which is both wrong and alarming. A lane running
 * on no objection says so.
 */
export function stateLineKey(purpose: PurposeView, on: boolean): MessageKey {
  if (!on) {
    return "prefs.optedOut";
  }
  return purpose.choice === "opted_in"
    ? "prefs.subscribed"
    : "prefs.noObjection";
}

/**
 * Whether a row shows as on, draft included.
 *
 * A LOCKED row is on, full stop, and no draft entry can move it. It
 * carries an "always on" badge and a disabled control, so rendering it
 * unchecked said the opposite of the badge beside it — and the server
 * refuses to change it either way, which makes any other value a claim
 * the product cannot honour.
 */
export function rowIsOn(purpose: PurposeView, draft: Draft): boolean {
  if (purpose.locked) {
    return true;
  }
  return draft[purpose.key] ?? displayOn(purpose);
}

// subjectMayGrant answers whether this SUBJECT can be granted anything at all,
// as opposed to whether a given purpose needs a confirmation round trip.
//
// The server sends one flag for both: can_opt_in is
// `!locked && !grant_needs_confirmation && grantable` (preferenceview.go), and
// only `grantable` is about the subject — an archived or Art. 17 anonymised
// record whose erasure destroyed the capability a fresh grant would re-open.
// The server refuses those with `cannot_grant` however the page asks.
//
// Recovered rather than asked for, because the wire carries no separate field:
// among the purposes the round trip does NOT apply to, at least one offering
// opt-in means the subject is eligible. None of them offering it means the
// refusal is about the subject.
//
// NO SUCH PURPOSES AT ALL is read as eligible. An installation whose every
// marketing purpose needs a round trip tells us nothing about the subject, and
// blocking on no evidence would refuse a live contact their own subscription.
export function subjectMayGrant(purposes: PurposeView[]): boolean {
  const decidable = purposes.filter(
    (purpose) => !purpose.locked && !purpose.grant_needs_confirmation,
  );
  return (
    decidable.length === 0 || decidable.some((purpose) => purpose.can_opt_in)
  );
}
