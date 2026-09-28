import { formatDate } from "../format/format";
import type { Locale, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { humanizeKind } from "./approvalkind";

// What a reader SEES of a staged proposal, per kind.
//
// Split from approvalkind.ts, which keeps the WRITE half (what a reader may
// change before accepting) and the kind vocabulary itself. The two halves share
// only the kind as a key: this one is a catalogue that grows with every kind the
// server learns to stage, and it had grown to two thirds of a file whose name
// promises something smaller.

// Without this a card falls back to printing the payload's own JSON keys and
// values: `deal_id  01a03781-9083-…`, `flags  ["unrealistic_stale"]`,
// `target_version  —`. That is the database row, shown to somebody who was
// asked to make a business decision. The information they actually need is
// usually in there — `basis` on a close-date correction is written by the
// server expressly as the plain-language reason — but it arrives as row four
// of eight, in the same grey type as a uuid.
//
// So a kind may declare what each field IS. Three consequences follow from the
// declaration, and they are the whole point:
//
//   - a field nobody declares is NOT SHOWN. Identifiers, versions and
//     dedupe keys are how the software finds a record, not why a contact should
//     agree to something. They stay reachable under the detail dialog's
//     technical disclosure for whoever genuinely needs them.
//   - `lead` is the sentence the card leads with, under the headline. At most
//     one per kind.
//   - everything else declared renders as a labelled fact, in the order
//     declared rather than in JSON key order.
//
// A kind that declares nothing keeps the old behaviour, so the raw-args kinds
// (an agent tool's arguments, an automation rule's action) are unchanged: they
// carry no typed payload for this map to describe, and inventing labels for a
// bag of unknown keys would be guessing.
export type DisplayField = {
  readonly field: string;
  /** What the field is CALLED. A wire name is a payload path, not a caption. */
  readonly label: MessageKey;
  /**
   * How to read the value.
   *
   * `prose` is a sentence the server wrote for a human. `date` is a date-only
   * wire string, rendered on the reader's calendar. `enum` looks the value up
   * in `optionLabels` — a raw enum on screen ("unrealistic_stale") is a wire
   * token wearing a caption. `text` is a value that is already a word: a name,
   * an address, a stage.
   */
  readonly as: "prose" | "text" | "date" | "enum";
  /**
   * Promotes this field to the card's lead sentence rather than a labelled
   * fact. At most one per kind, and only ever a field that answers "why am I
   * being asked this?" — the reason, never the value being proposed.
   */
  readonly lead?: true;
  /** What each enum value is CALLED. Required in spirit for `as: "enum"`. */
  readonly optionLabels?: Readonly<Record<string, MessageKey>>;
};

// The §11 hygiene findings, spelled as deals.CloseDateFlag spells them
// (modules/deals/closedate.go). A flag with no entry here renders as its own
// humanized token rather than vanishing: an unnamed finding is still a finding,
// and a card that silently drops one tells the reader less than an ugly word
// would.
const CLOSE_DATE_FLAGS = {
  overdue: "approval.field.closeDateFlag.overdue",
  missing: "approval.field.closeDateFlag.missing",
  unrealistic_soon: "approval.field.closeDateFlag.unrealistic_soon",
  unrealistic_stale: "approval.field.closeDateFlag.unrealistic_stale",
} as const satisfies Readonly<Record<string, MessageKey>>;

export const DISPLAY_FIELDS: Readonly<Record<string, readonly DisplayField[]>> =
  {
    // The question is "is this deal still alive?", and `basis` is the server's
    // own sentence saying why it is being asked. The two dates are what the
    // reader weighs; the deal id and the target version are how the write finds
    // its row.
    close_date_correction: [
      {
        field: "basis",
        label: "approval.field.basis",
        as: "prose",
        lead: true,
      },
      {
        field: "previous_close_date",
        label: "approval.field.previous_close_date",
        as: "date",
      },
      {
        field: "expected_close_date",
        label: "approval.field.expected_close_date",
        as: "date",
      },
      {
        field: "flags",
        label: "approval.field.flags",
        as: "enum",
        optionLabels: CLOSE_DATE_FLAGS,
      },
    ],
    // `because` is the signal's own words for why the stage should move. The two
    // stages already render as a proper old→new comparison on the card
    // (diffsOf reads the current_/proposed_ pair), so they are deliberately NOT
    // repeated here — declaring them would print the same fact twice.
    lifecycle_change: [
      {
        field: "because",
        label: "approval.field.because",
        as: "prose",
        lead: true,
      },
    ],
    // A lead captured from a company's own site. The reader is deciding whether
    // this is a real contact worth keeping, so the snippet that named them leads.
    site_lead: [
      {
        field: "evidence_snippet",
        label: "approval.field.evidence_snippet",
        as: "prose",
        lead: true,
      },
      { field: "name", label: "approval.field.name", as: "text" },
      { field: "role", label: "approval.field.role", as: "text" },
      {
        field: "published_email",
        label: "approval.field.published_email",
        as: "text",
      },
    ],
    // Is this address a contact worth keeping? The address and who it belongs to
    // are the question; the disposition, owner and activity ids are plumbing.
    capture_counterparty: [
      { field: "display_name", label: "approval.field.name", as: "text" },
      { field: "email", label: "approval.field.email", as: "text" },
      { field: "domain", label: "approval.field.domain", as: "text" },
    ],
    // Two records that look like one contact. The names are the whole comparison.
    linkedin_match: [
      {
        field: "connection_name",
        label: "approval.field.connection_name",
        as: "text",
      },
      {
        field: "connection_company",
        label: "approval.field.connection_company",
        as: "text",
      },
      {
        field: "contact_name",
        label: "approval.field.contact_name",
        as: "text",
      },
    ],
    // A message that was scheduled and then stopped. Why it stopped is the whole
    // question, and the SUMMARY already carries that sentence: the server maps
    // the reason code to prose in one place (compose/scheduledsendheld.go,
    // heldReasonText) and composes it into the headline. Declaring `reason` here
    // would print the same fact a second time, in a second vocabulary that would
    // drift from the first. Only the moment it was meant to go is left.
    scheduled_send_held: [
      {
        field: "scheduled_at",
        label: "approval.field.scheduled_at",
        as: "text",
      },
    ],
    // A captured message that collided with a lead already here. Accepting
    // fills the lead's EMPTY fields from it and never replaces a value
    // somebody typed, so each captured value is drawn beside what the lead
    // already holds: the pairs that differ are the ones the accept will
    // silently drop, and a card that showed only the four captured values
    // asked for a decision whose outcome the reader could not see.
    //
    // Not a current_/proposed_ diff, which is what diffsOf would draw here: a
    // strike-through old value beside a new one says the new one replaces it,
    // and that is the opposite of what accepting does to an occupied field.
    //
    // So a PAIR on the card is a conflict the accept will leave alone, and a
    // captured value standing alone is one that lands — the lead has nothing
    // there. That reading is the same one diffsOf states for its own pairs, and
    // it is why an empty current value resolving to null is right rather than a
    // gap: there is no value to caption.
    //
    // The captured keys are capture's Go field names (see
    // compose/capturecollision.go — they are on disk in every pending row),
    // which is exactly why they need captions. A card staged before the
    // current_ values were carried reads as all-additions for its last 72
    // hours, which is the staging TTL and the whole life of the oldest such row.
    merge_records: [
      {
        field: "current_full_name",
        label: "approval.field.leadNameNow",
        as: "text",
      },
      { field: "FullName", label: "approval.field.capturedName", as: "text" },
      {
        field: "current_company_name",
        label: "approval.field.leadCompanyNow",
        as: "text",
      },
      {
        field: "CompanyName",
        label: "approval.field.capturedCompany",
        as: "text",
      },
      {
        field: "current_title",
        label: "approval.field.leadTitleNow",
        as: "text",
      },
      { field: "Title", label: "approval.field.capturedTitle", as: "text" },
      // The address the collision was found on: the lead has it by definition,
      // so there is nothing to compare it against and nothing to fill.
      { field: "Email", label: "approval.field.email", as: "text" },
    ],
    // A follow-up the overnight pass drafted. Its subject and body already
    // render as a draft on the card, so only the date it proposes is left.
    deal_follow_up: [
      { field: "due_date", label: "approval.field.due_date", as: "date" },
    ],
    // A rename read off the company's own site. The old and new names render as
    // an old→new comparison already; the normalization key is internal.
    company_name_promotion: [],
    // An imported card the dedupe pass refused to create beside its
    // near-match. Every field the approval would write is captioned: the
    // decider must see the whole create, not its headline.
    vcard_create: [
      { field: "full_name", label: "approval.field.name", as: "text" },
      { field: "emails", label: "approval.field.email", as: "text" },
      { field: "company", label: "approval.field.company", as: "text" },
      { field: "title", label: "approval.field.title", as: "text" },
      { field: "phones", label: "approval.field.phone", as: "text" },
      { field: "url", label: "approval.field.url", as: "text" },
      { field: "address", label: "approval.field.address", as: "text" },
    ],
    // A price the provider published. Both are decimal strings, already values a
    // reader recognises.
    fx_rate_proposal: [
      { field: "from_currency", label: "approval.field.currency", as: "text" },
      { field: "rate", label: "approval.field.rate", as: "text" },
      {
        field: "expected_prior_rate",
        label: "approval.field.prior_rate",
        as: "text",
      },
    ],
    ai_model_rate_proposal: [
      { field: "provider", label: "approval.field.provider", as: "text" },
      { field: "model_id", label: "approval.field.model", as: "text" },
      {
        field: "input_per_mtok",
        label: "approval.field.input_per_mtok",
        as: "text",
      },
      {
        field: "output_per_mtok",
        label: "approval.field.output_per_mtok",
        as: "text",
      },
    ],
    // A proposed stage move. `because` leads, because a rep deciding needs
    // the reason before the checklist: the criteria say WHAT is settled and
    // the sentence says why that adds up to a move.
    stage_progression: [
      {
        field: "because",
        label: "approval.field.because",
        as: "prose",
        lead: true,
      },
      {
        field: "from_stage_name",
        label: "approval.field.from_stage",
        as: "text",
      },
      { field: "to_stage_name", label: "approval.field.to_stage", as: "text" },
    ],
    // Read out of a call transcript. The step is the proposal; the evidence
    // chips beneath carry the quoted lines it was read from.
    transcript_proposal: [
      {
        field: "summary",
        label: "approval.field.step",
        as: "prose",
        lead: true,
      },
      { field: "owner", label: "approval.field.owner", as: "text" },
      // The day the transcript stated, editable before the task exists. The
      // calendar control rather than a text box, because the payload wants
      // 2026-09-08 and a reviewer typing the date the way they say it out loud
      // writes something acceptance refuses.
      //
      // Empty is the ordinary reading and stays editable: a next step nobody
      // dated is not overdue, and a reviewer who knows the deadline can supply
      // it here rather than opening the task afterwards to add one.
      { field: "due_date", label: "approval.field.due_date", as: "date" },
    ],
    // An automation composed this reply. Subject and body are the draft the card
    // already renders; `intent` is why the rule fired.
    held_draft: [
      {
        field: "intent",
        label: "approval.field.intent",
        as: "prose",
        lead: true,
      },
      { field: "to", label: "approval.field.to", as: "text" },
    ],
    // The agent hit a ceiling and is asking for room. What it was doing and how
    // much it has used are the question; the passport id is not.
    volume_release: [
      { field: "tool", label: "approval.field.tool", as: "text" },
      { field: "observed", label: "approval.field.observed", as: "text" },
      { field: "limit", label: "approval.field.limit", as: "text" },
      { field: "allowance", label: "approval.field.allowance", as: "text" },
    ],
  };

/** What a kind shows, or nothing when it has declared no display policy. */
export function displayFields(kind: string): readonly DisplayField[] {
  // Own-property only, for the reason editableStrings checks it: `kind` is a
  // wire string, and one spelled `constructor` would otherwise find a function
  // on Object's prototype and crash the queue rather than falling back.
  return Object.hasOwn(DISPLAY_FIELDS, kind) ? DISPLAY_FIELDS[kind] : [];
}

/**
 * One declared field against one payload, ready for the card to draw.
 *
 * Returns null for a field the payload does not carry, which is ordinary
 * rather than exceptional: `previous_close_date` is absent on a deal that
 * never had one, and `expected_prior_rate` on the first rate ever recorded.
 * The card drops those rather than printing an empty row, so the reader sees
 * what is true instead of a blank where a fact would be.
 */
function displayValue(
  entry: DisplayField,
  raw: unknown,
  t: Translator,
  formatDay: (value: string) => string,
): string | null {
  if (raw === null || raw === undefined) {
    return null;
  }
  if (entry.as === "enum") {
    // A list or a single code, both spelled the same way on the wire across
    // kinds: `flags` is an array, `reason` is one string.
    const codes = Array.isArray(raw) ? raw : [raw];
    const words = codes.flatMap((code) => {
      if (typeof code !== "string" || code === "") {
        return [];
      }
      const key = entry.optionLabels?.[code];
      // An unmapped code degrades to its own words rather than vanishing. A
      // finding the server raised and the card silently dropped would tell the
      // reader less than an unpolished word does.
      return [key ? t(key) : humanizeKind(code)];
    });
    return words.length > 0 ? words.join(", ") : null;
  }
  if (typeof raw !== "string") {
    // Numbers reach the reader as their own digits; anything structured is not
    // a value this tier can caption, so it is left out rather than JSON-dumped
    // under a label that would then be lying about what it introduces.
    return typeof raw === "number" || typeof raw === "boolean"
      ? String(raw)
      : null;
  }
  if (raw.trim() === "") {
    return null;
  }
  return entry.as === "date" ? formatDay(raw) : raw;
}

/**
 * What this proposal shows, resolved into the reader's language.
 *
 * The card is handed finished strings rather than a policy to interpret: which
 * fields a kind shows is the product's vocabulary, and a design-system
 * primitive that looked it up would be a second author of it.
 */
export function resolveDisplay(
  kind: string,
  change: Readonly<Record<string, unknown>>,
  t: Translator,
  formatDay: (value: string) => string,
): readonly ResolvedDisplayField[] {
  const resolved = displayFields(kind).map((entry) => ({
    field: entry.field,
    label: t(entry.label),
    value: displayValue(
      entry,
      Object.hasOwn(change, entry.field) ? change[entry.field] : undefined,
      t,
      formatDay,
    ),
    lead: entry.lead,
  }));
  const byField = new Map(resolved.map((entry) => [entry.field, entry]));
  return resolved.filter((entry) => {
    const supersedes = supersededBy.get(entry.field);
    if (supersedes === undefined) {
      return true;
    }
    const other = byField.get(supersedes);
    return (
      other === undefined || other.value === null || other.value !== entry.value
    );
  });
}

/**
 * Which field's value makes another one redundant, when the two agree.
 *
 * ONE case, declared rather than inferred. A close-date correction on a stale
 * deal proposes the date the deal already carries — the sweep keeps the date and
 * asks a contact instead of guessing a new one — so the card printed "Date on it
 * now 01.10.2026" directly above "Proposed date 01.10.2026". Two captions over
 * one value is not a comparison; it reads as a fault in the card.
 *
 * Declared per pair, because the general rule ("drop any field whose value
 * another field already showed") is wrong and was briefly shipped here. A
 * LinkedIn match whose connection and contact share a name is a match at its
 * most obvious, and collapsing it deleted both sides of the comparison the card
 * exists to draw. A quota release observed 40 against a limit of 40 is an agent
 * exactly at its ceiling, and the coincidence IS the news. Two fields agreeing
 * is ordinarily information, not repetition — only a declared before/after pair
 * makes it noise.
 *
 * The CURRENT half is dropped and the proposal kept: a card showing only "the
 * date on it now" hides what accepting would do, which inverts the question.
 */
const supersededBy = new Map<string, string>([
  ["previous_close_date", "expected_close_date"],
]);

/** A DisplayField with its label and value resolved for one payload. */
export type ResolvedDisplayField = Readonly<{
  field: string;
  label: string;
  value: string | null;
  lead?: true;
}>;

/**
 * How a staged date-only value reads on screen.
 *
 * The same `formatDate` the deal pages put a close date through, so the inbox
 * and the record cannot print one date two ways. A malformed value is shown as
 * written rather than as "Invalid Date": the payload is what the server staged,
 * and a reader who can see the raw string can say so.
 */
export function stagedDayFormatter(
  locale: Locale,
  zone: string,
): (value: string) => string {
  return (value) => {
    const day = formatDate(value, locale, zone);
    return day.includes("Invalid") ? value : day;
  };
}
