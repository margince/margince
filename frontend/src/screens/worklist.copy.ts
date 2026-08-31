import { ENTITY, isEntityKind } from "../app/entity";
import { routeHash } from "../app/router";
import {
  formatDate,
  formatDateTime,
  formatMoney,
  formatNumber,
} from "../format/format";
import type { Locale, useT } from "../i18n";
import { settingsAddress } from "./settings";
import type {
  Worklist,
  WorklistComparison,
  WorklistItem,
  WorklistReason,
  WorklistValue,
} from "./worklist.queries";

type T = ReturnType<typeof useT>;

// The queue's words.
//
// The server sends facts and closed vocabularies; every sentence on the page is
// written here, in the reader's own language. That split is why the ranking can
// be explained at all: a reason composed server-side would reach a German
// reader in English, so what travels is `{kind, value}` and the phrase is the
// client's to write.

// Which record an item points at, as an address the router understands.
//
// Through the entity registry, never a switch written here: the record types
// have route names of their own (`contacts`, not `people`), and a second
// spelling of them sends a reader to a page that does not exist. An activity
// resolves to nothing on purpose — it is a timeline entry rather than a record
// with a page, so naming it on the row is honest and linking it is not.
export function subjectHref(item: WorklistItem): string | undefined {
  const subject = item.subject;
  if (!subject || !isEntityKind(subject.type)) {
    return undefined;
  }
  return routeHash(ENTITY[subject.type].route(subject.id));
}

// Where a row goes when it names no record of its own.
//
// Most rows point at a record and reach it through the entity registry. A few
// name a QUEUE instead — a data-subject request is worked on the privacy
// screen and nowhere else — and without a destination those rows are a
// sentence a reader cannot follow, on the one lane whose whole argument is a
// legal clock somebody has to answer.
//
// A destination is not a verb: these rows still offer no action, because the
// queue cannot perform one. It is the difference between telling somebody
// where a room is and claiming to have opened the door.
// Through settingsAddress, not a path spelled here: the settings registry
// decides whether a tab sits under the admin segment, and a second spelling of
// that decision would keep pointing at the old address the day it moves.
const SOURCE_QUEUE: Partial<Record<WorklistItem["source"], string>> = {
  dsr: routeHash(settingsAddress("privacy")),
};

// The address a row's headline links to: its record where it has one, the
// queue that owns it where it does not, and nothing at all for a row that is
// neither — a system condition fixed on a settings screen the card does not
// pretend to know.
export function rowHref(item: WorklistItem): string | undefined {
  return subjectHref(item) ?? SOURCE_QUEUE[item.source];
}

// One comparator value, in the reader's notation.
//
// A magnitude written in the wrong notation is a different number to the person
// reading it: a German reader reads "1.234", and the coerced form says
// something else.
function valueText(
  value: WorklistValue | undefined,
  locale: Locale,
  zone: string,
): string | null {
  if (!value) {
    return null;
  }
  switch (value.kind) {
    case "date":
      return value.date ? formatDateTime(value.date, locale, zone) : null;
    case "money":
      // Both halves or nothing. A euro sign on a figure that might be dong is
      // worse than no figure, and the scale itself is wrong without the
      // currency — so a value that names none says nothing at all.
      return value.minor == null || !value.currency
        ? null
        : formatMoney(value.minor, value.currency, locale);
    case "days":
      return value.days == null ? null : formatNumber(value.days, locale);
    case "level":
      return value.level == null ? null : formatNumber(value.level, locale);
    default:
      return null;
  }
}

// The reasons that read differently with a figure in them. Spelled as a set
// rather than inferred from whether a value arrived: a value can travel for a
// reason whose sentence has nowhere to put it, and a key composed from that
// would not exist.
const VALUED_REASONS = {
  waiting_days: true,
  quiet_days: true,
  expected_revenue: true,
  material: true,
  below_material: true,
} as const;

type ValuedReason = keyof typeof VALUED_REASONS;

function valued(kind: WorklistReason["kind"]): kind is ValuedReason {
  return kind in VALUED_REASONS;
}

// Every reason this client has a sentence for.
//
// A newer server sending a reason this build does not know must not print
// `worklist.because.customer_escalated` at a reader — a missing translation
// returns its own key, so an unrecognised value has to be caught here rather
// than discovered on screen.
const KNOWN_REASONS = {
  pinned: true,
  buyer_wrote_last: true,
  waiting_days: true,
  overdue: true,
  due_today: true,
  closing_soon: true,
  expected_revenue: true,
  material: true,
  below_material: true,
  quiet_days: true,
  no_champion: true,
  promised: true,
  approved_and_failed: true,
  blocks_customer_work: true,
  routine: true,
  repeated_failure: true,
  legal_deadline: true,
  meeting_soon: true,
} as const;

type KnownReason = keyof typeof KNOWN_REASONS;

function known(kind: WorklistReason["kind"]): kind is KnownReason {
  return kind in KNOWN_REASONS;
}

// The comparators this build can name, for the same reason.
const KNOWN_COMPARATORS = {
  pin: true,
  level: true,
  deadline: true,
  expected_revenue: true,
  waiting_days: true,
  relationship: true,
} as const;

function knownComparator(
  comparator: WorklistComparison["comparator"],
): comparator is keyof typeof KNOWN_COMPARATORS {
  return comparator in KNOWN_COMPARATORS;
}

// One fact behind an item's rank.
export function reasonText(
  reason: WorklistReason,
  t: T,
  locale: Locale,
  zone: string,
): string | null {
  if (!known(reason.kind)) {
    // A reason this build has no sentence for is DROPPED, not printed as its
    // own key. The row keeps its other reasons and says one thing less.
    return null;
  }
  const value = valueText(reason.value, locale, zone);
  if (value !== null && valued(reason.kind)) {
    return t(`worklist.because.${reason.kind}.value` as const, { value });
  }
  return t(`worklist.because.${reason.kind}` as const);
}

// The comparators whose sentence names BOTH sides. A level or a pin decided
// without a figure a reader would compare, so those say what decided and stop.
const PAIRED_COMPARATORS = {
  deadline: true,
  expected_revenue: true,
  waiting_days: true,
} as const;

type PairedComparator = keyof typeof PAIRED_COMPARATORS;

function paired(
  comparator: WorklistComparison["comparator"],
): comparator is PairedComparator {
  return comparator in PAIRED_COMPARATORS;
}

// Why this row sits above the next one.
//
// The comparator that DECIDED, with both sides' values — so a reader can check
// the order rather than trust it. `order` means every comparator tied and the
// ids broke it, which is not a reason a person needs to read, so it draws
// nothing at all.
export function comparisonText(
  comparison: WorklistComparison | undefined,
  t: T,
  locale: Locale,
  zone: string,
): string | null {
  if (
    !comparison ||
    comparison.comparator === "order" ||
    !knownComparator(comparison.comparator)
  ) {
    return null;
  }
  const mine = valueText(comparison.mine, locale, zone);
  const theirs = valueText(comparison.theirs, locale, zone);
  if (mine === null || theirs === null || !paired(comparison.comparator)) {
    return t(`worklist.above.${comparison.comparator}` as const);
  }
  return t(`worklist.above.${comparison.comparator}.pair` as const, {
    mine,
    theirs,
  });
}

// What happens if the reader does nothing.
export function consequenceText(item: WorklistItem, t: T): string | null {
  if (item.consequence === "none") {
    return null;
  }
  return t(`worklist.consequence.${item.consequence}` as const);
}

// The deal's own figures, in one line: what it is worth, and when it closes.
//
// The concept's sharpest example was a €160,100 deal reduced to "no contact for
// 83 days". The money was on the wire the whole time; only the row discarded it.
export function dealFactsText(
  item: WorklistItem,
  t: T,
  locale: Locale,
  zone: string,
): string | null {
  const deal = item.deal;
  if (!deal) {
    return null;
  }
  const parts: string[] = [];
  if (deal.amount_minor != null && deal.currency) {
    parts.push(formatMoney(deal.amount_minor, deal.currency, locale));
  }
  if (deal.expected_close_date) {
    parts.push(
      t("worklist.deal.closes", {
        date: formatDate(deal.expected_close_date, locale, zone),
      }),
    );
  }
  return parts.length > 0 ? parts.join(" · ") : null;
}

// Where the row's suggested step leads.
//
// The RECORD the thread belongs to, not the message: an activity is a timeline
// entry with no page of its own — `app/entity.ts` says so in as many words —
// so `#/activities/<id>` would be a control that goes nowhere.
//
// WHAT THIS IS NOT. It does not open a composer. The composer lives on the
// record page behind its own button, and no route opens it, so a link labelled
// "Draft the reply" would promise something the click does not do. The verb
// therefore says where it goes — the reader lands on the record with the
// message on its timeline, one press from the draft — and naming the step
// honestly is worth more than a label that overstates it.
//
// Opening the composer from here needs a deep link the app does not have. That
// is its own change, and the label moves back when it lands.
export function moveHref(item: WorklistItem): string | undefined {
  if (item.move?.action !== "draft_reply") {
    return undefined;
  }
  return subjectHref(item);
}

// One item's headline.
//
// The server sends a sentence where it HAS one — an approval summary composed
// at staging time, a deal's own name, a message's subject. Where it has none
// the sentence would have to be invented, and the client writes it from the
// source instead.
export function itemTitle(item: WorklistItem, t: T, locale: Locale): string {
  // A group names itself by what it holds and how much: "43 likely automated
  // senders" is the whole row, and a reader decides whether to open it from
  // that sentence alone.
  if (item.batch) {
    // "200+" where the read stopped at its own bound. A floor printed as a
    // total is a wrong number rather than a bounded one, and the reader has no
    // way to tell the two apart.
    const count = item.batch.at_least
      ? `${formatNumber(item.batch.count, locale)}+`
      : formatNumber(item.batch.count, locale);
    // An incident names WHAT is broken; a hygiene group names its kind.
    if (item.batch.key === "system_incident") {
      return t("worklist.batch.system_incident", {
        count,
        cause: item.batch.cause ?? t("worklist.batch.unnamedCause"),
      });
    }
    return t(`worklist.batch.${item.batch.key}` as const, { count });
  }
  if (item.title) {
    // A title that names no record, on a row that HAS one, gets the record's
    // name beside it. Eight rows reading "Follow up with the new lead" cannot
    // be told apart or put in order, and the lead's name is already on the row
    // — it was only the sentence that discarded it.
    const named = item.subject?.label;
    return named && !item.title.includes(named)
      ? `${item.title} · ${named}`
      : item.title;
  }
  if (!knownSource(item.source)) {
    return t("worklist.untitled.generic");
  }
  return t(`worklist.untitled.${item.source}` as const);
}

// Which source could not be read, and why, in words.
//
// The source name goes through the same known-source check the titles use, so
// a source this build has never heard of is described generically rather than
// printed as its own identifier.
export function sourceUnavailableText(
  missing: NonNullable<Worklist["sources_unavailable"]>[number],
  t: T,
): string {
  const name = knownSource(missing.source as WorklistItem["source"])
    ? t(`worklist.untitled.${missing.source as keyof typeof KNOWN_SOURCES}`)
    : t("worklist.untitled.generic");
  return missing.reason === "withheld"
    ? t("worklist.source.withheld", { source: name })
    : t("worklist.source.failed", { source: name });
}

// The sources this build can name without its own sentence.
const KNOWN_SOURCES = {
  approval: true,
  dedupe_candidate: true,
  task: true,
  brief_item: true,
  conversation_claim: true,
  customer_waiting: true,
  deal_at_risk: true,
  meeting: true,
  relationship_decay: true,
  failed_approval: true,
  dsr: true,
  sync_health: true,
  capture_health: true,
  ai_work_health: true,
  bounce: true,
  automation_run: true,
  notice: true,
  // A group of routine decisions, which names no single record.
  batch: true,
} as const;

function knownSource(
  source: WorklistItem["source"],
): source is keyof typeof KNOWN_SOURCES {
  return source in KNOWN_SOURCES;
}
