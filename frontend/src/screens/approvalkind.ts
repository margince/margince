import { formatDate } from "../format/format";
import type { Locale, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

// What a staged proposal is, in words a reader recognises.
//
// `approval.kind` is a wire enum — `site_lead`, `fx_rate_proposal` — and it was
// rendered verbatim wherever a proposal was listed. A reader deciding whether
// to accept twenty-five of something needs to know what that something is, and
// snake_case in the German UI is not a translation of anything.
//
// The set is the approvals module's grant maps, and this map is pinned against
// them by backend/gates/frontendapprovalkinds_test.go, which DERIVES the corpus
// rather than restating it. The gate this replaced compared against a list
// hand-copied into the frontend's own test, and a mirror of a mirror agrees
// with itself: eleven stageable kinds had no label and two labels named kinds
// the server had already dropped.
//
// What holds a kind to those maps is the compose-side census over every
// production staging site, not the staging writer itself — that one inserts
// the kind it is handed. So this map covers what the product stages, and the
// fallback below is what a kind reaching a reader some other way gets.
//
// A kind that still slips through falls back to its own words rather than its
// identifier: it must degrade to "site lead", never to a token that only makes
// sense to whoever wrote the server.

export const KIND_LABEL: Readonly<Record<string, MessageKey>> = {
  advance_deal: "approval.kind.advance_deal",
  progress_deal: "approval.kind.advance_deal",
  promote_lead: "approval.kind.promote_lead",
  archive_record: "approval.kind.archive_record",
  merge_records: "approval.kind.merge_records",
  update_record: "approval.kind.update_record",
  create_record: "approval.kind.create_record",
  send_email: "approval.kind.send_email",
  // Named for what the reader has to DO, not for what produced it: the row is
  // an email waiting to be read and released, and "held draft" describes its
  // state in a queue rather than the decision in front of them.
  held_draft: "approval.kind.held_draft",
  book_meeting: "approval.kind.book_meeting",
  coldstart: "approval.kind.coldstart",
  // Not a change to a record — a question about a credential's volume, which is
  // why its label says what a yes DOES rather than naming an object.
  quota_release: "approval.kind.quota_release",
  enrich: "approval.kind.enrich",
  deepread: "approval.kind.deepread",
  linkedin_match: "approval.kind.linkedin_match",
  site_lead: "approval.kind.site_lead",
  close_date_correction: "approval.kind.close_date_correction",
  deal_follow_up: "approval.kind.deal_follow_up",
  capture_counterparty: "approval.kind.capture_counterparty",
  org_name_promotion: "approval.kind.org_name_promotion",
  vcard_create: "approval.kind.vcard_create",
  lifecycle_change: "approval.kind.lifecycle_change",
  transcript_proposal: "approval.kind.transcript_proposal",
  fx_rate_proposal: "approval.kind.fx_rate_proposal",
  ai_model_rate_proposal: "approval.kind.ai_model_rate_proposal",
  disqualify_lead: "approval.kind.disqualify_lead",
  advance_project_phase: "approval.kind.advance_project_phase",
  assign_owner: "approval.kind.assign_owner",
  commit_import: "approval.kind.commit_import",
  emit_flow_event: "approval.kind.emit_flow_event",
  relink_activity: "approval.kind.relink_activity",
  relink_thread: "approval.kind.relink_thread",
  relink_activities: "approval.kind.relink_activities",
  // Distinct from held_draft above, and not a second spelling of it: that one
  // is a reply automation COMPOSED and is waiting to be sent, this one is a
  // message that was already scheduled and got stopped. Different lifecycles,
  // so different words.
  scheduled_send_held: "approval.kind.scheduled_send_held",
  send_account_email: "approval.kind.send_account_email",
  send_message: "approval.kind.send_message",
};

// What a reader may CHANGE before accepting, per kind.
//
// The inline editor's default is every string field of the proposed_change,
// rendered as a text box. That default is right for a rename — the value IS
// prose — and wrong for a proposal built out of identifiers and enums. Editing
// `organization_id` re-aims the proposal at another record, and the server
// refuses that (assertSameEntityRefs); editing `proposed_lifecycle` by typing
// produces an invalid stage, and the server refuses that too. Both refusals
// are correct and neither is a thing to show a reader who was only trying to
// answer the question in front of them.
//
// So a kind may declare which fields it offers and what each one accepts. A
// kind that declares nothing keeps the default, which is why adding this
// changed no existing surface.
export type EditableField =
  | { readonly field: string; readonly as: "text"; readonly label?: MessageKey }
  | {
      readonly field: string;
      /**
       * A date-only wire value. It gets the calendar control rather than a text
       * box: the payload wants `2026-09-27`, and a reader typing the date the
       * way they say it out loud writes something the server refuses.
       */
      readonly as: "date";
      readonly label?: MessageKey;
    }
  | {
      readonly field: string;
      /**
       * Prose that runs to paragraphs rather than a line. An email body in a
       * single-line input is technically editable and practically unreadable:
       * the reader can see about eight words of what they are being asked to
       * put their name on.
       */
      readonly as: "textarea";
      readonly label?: MessageKey;
    }
  | {
      readonly field: string;
      readonly as: "choice";
      /**
       * What the field is CALLED. The wire name is a payload path, not a
       * caption — without this the editor asks a reader to set
       * "proposed_lifecycle".
       */
      readonly label?: MessageKey;
      readonly options: readonly string[];
      /**
       * What each option is CALLED. Without it the editor offers the wire
       * enum, so a German inbox asks a reader to choose "former_customer".
       * Optional so a choice field whose values are already words needs
       * nothing.
       */
      readonly optionLabels?: Readonly<Record<string, MessageKey>>;
    };

const ORG_LIFECYCLE_STAGES = [
  "unknown",
  "target",
  "prospect",
  "opportunity",
  "customer",
  "former_customer",
  "disqualified",
] as const;

// The same catalog keys the account page's stage badge reads, so the inbox and
// the record cannot call one stage two things. Keyed off the list above:
// a stage added there with no entry here fails the type.
const ORG_LIFECYCLE_LABELS: Readonly<
  Record<(typeof ORG_LIFECYCLE_STAGES)[number], MessageKey>
> = {
  unknown: "org.lifecycle.unknown",
  target: "org.lifecycle.target",
  prospect: "org.lifecycle.prospect",
  opportunity: "org.lifecycle.opportunity",
  customer: "org.lifecycle.customer",
  former_customer: "org.lifecycle.former_customer",
  disqualified: "org.lifecycle.disqualified",
};

export const EDITABLE_FIELDS: Readonly<
  Record<string, readonly EditableField[]>
> = {
  // The stage is the whole question. Everything else in the payload — which
  // account, which signal, the stage it is in now — is what the question is
  // ABOUT, and a reader who disagrees with any of that says no rather than
  // editing it into a different question.
  lifecycle_change: [
    {
      field: "proposed_lifecycle",
      as: "choice",
      label: "org.lifecycle",
      options: ORG_LIFECYCLE_STAGES,
      optionLabels: ORG_LIFECYCLE_LABELS,
    },
  ],
  // An automation-composed email waiting for a human to read, correct and
  // release. The words are the whole question, so both of them are offered.
  //
  // Declaring the fields also NARROWS what the editor shows, and here that is
  // the point rather than a side effect: the payload also carries the
  // addressee, the consent purpose and the anchor, and every one of those is
  // something the approver is agreeing TO rather than something to retype. The
  // server refuses an edited anchor outright (it is an entity reference, and
  // edit scope pins those), so offering it would only invite a refusal.
  held_draft: [
    { field: "subject", as: "text", label: "decision.draftSubject" },
    { field: "body", as: "textarea", label: "decision.draftBody" },
  ],
  // The date is the entire question, and it is the only thing here a person may
  // change. Undeclared, the generic editor offered every string in the payload:
  // the deal's uuid as a text box to retype, the server's own reason sentence
  // as if it were the reader's to rewrite, and the previous date beside the
  // proposed one with nothing saying which was which.
  close_date_correction: [
    {
      field: "expected_close_date",
      as: "date",
      label: "approval.field.expected_close_date",
    },
  ],
};

// What a reader SEES of a proposal, per kind — the read-side counterpart of
// EDITABLE_FIELDS above.
//
// Without this a card falls back to printing the payload's own JSON keys and
// values: `deal_id  01a03781-9083-…`, `flags  ["unrealistic_stale"]`,
// `target_version  —`. That is the database row, shown to somebody who was
// asked to make a business decision. The information they actually need is
// usually in there — `basis` on a close-date correction is written by the
// server expressly as the plain-language reason — but it arrives as row four
// of eight, in the same grey monospace as a uuid.
//
// So a kind may declare what each field IS. Three consequences follow from the
// declaration, and they are the whole point:
//
//   - a field nobody declares is NOT SHOWN. Identifiers, versions and
//     dedupe keys are how the software finds a record, not why a person should
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
    // this is a real person worth keeping, so the snippet that named them leads.
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
    // Two records that look like one person. The names are the whole comparison.
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
      { field: "person_name", label: "approval.field.person_name", as: "text" },
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
    // A captured message that collided with a lead already here; accepting fills
    // the lead's empty fields from it. The keys are capture's Go field names
    // (see compose/capturecollision.go — they are on disk in every pending row),
    // which is exactly why they need captions.
    merge_records: [
      { field: "FullName", label: "approval.field.name", as: "text" },
      { field: "Email", label: "approval.field.email", as: "text" },
      { field: "CompanyName", label: "approval.field.company", as: "text" },
      { field: "Title", label: "approval.field.title", as: "text" },
    ],
    // A follow-up the overnight pass drafted. Its subject and body already
    // render as a draft on the card, so only the date it proposes is left.
    deal_follow_up: [
      { field: "due_date", label: "approval.field.due_date", as: "date" },
    ],
    // A rename read off the company's own site. The old and new names render as
    // an old→new comparison already; the normalization key is internal.
    org_name_promotion: [],
    // An imported card the dedupe pass refused to create beside its
    // near-match. The card's own facts are the decision.
    vcard_create: [
      { field: "full_name", label: "approval.field.name", as: "text" },
      { field: "emails", label: "approval.field.email", as: "text" },
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
    quota_release: [
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
 * asks a person instead of guessing a new one — so the card printed "Date on it
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

/** humanize turns an unmapped wire enum into readable words. */
export function humanizeKind(kind: string): string {
  return kind.replaceAll("_", " ");
}

export function approvalKindLabel(kind: string, t: Translator): string {
  const key = KIND_LABEL[kind];
  return key ? t(key) : humanizeKind(kind);
}
