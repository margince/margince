import { formatDateTime, formatNumber } from "../format/format";
import { toMajorUnits } from "../format/minorunits";
import type { Locale, useT } from "../i18n";
import type {
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
export function subjectHref(item: WorklistItem): string | undefined {
  if (!item.subject) {
    return undefined;
  }
  const { type, id } = item.subject;
  switch (type) {
    case "person":
      return `#/people/${id}`;
    case "organization":
      return `#/organizations/${id}`;
    case "deal":
      return `#/deals/${id}`;
    case "lead":
      return `#/leads/${id}`;
    case "project":
      return `#/projects/${id}`;
    default:
      // An activity is a timeline entry rather than a record with a page of its
      // own. Naming it on the card is honest; promising navigation is not.
      return undefined;
  }
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
      // Through the shared scale, never a hard-coded hundred: a currency with
      // no minor unit (JPY, VND, KRW) is understated a hundredfold by the
      // arithmetic that looks obviously right for euros.
      return value.minor == null
        ? null
        : formatNumber(
            toMajorUnits(value.minor, value.currency ?? "EUR"),
            locale,
          );
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

// One fact behind an item's rank.
export function reasonText(
  reason: WorklistReason,
  t: T,
  locale: Locale,
  zone: string,
): string {
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
  if (!comparison || comparison.comparator === "order") {
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

// One item's headline.
//
// The server sends a sentence where it HAS one — an approval summary composed
// at staging time, a deal's own name, a message's subject. Where it has none
// the sentence would have to be invented, and the client writes it from the
// source instead.
export function itemTitle(item: WorklistItem, t: T): string {
  if (item.title) {
    return item.title;
  }
  return t(`worklist.untitled.${item.source}` as const);
}
