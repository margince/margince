import { formatNumber } from "../format/format";
import type { Locale, Translator } from "../i18n";
import { citationOpensRecord } from "./citationroute";
import type { CitationReceipt, Cited, CitedKind } from "./citations";

export type CitationChip =
  | ({
      openable: true;
      entityType: CitedKind;
      entityId: string;
      count: number;
      // The record's own name, when the citation carried one (a deal's
      // name, an activity's subject). Absent on a grouped chip: a count
      // speaks for several records, and one name would claim the rest.
      name?: string;
    } & CitationReceipt)
  | ({
      openable: false;
      entityType: CitedKind;
      // The record's id where the chip stands for one. This chip opens
      // nothing, so the id tells two chips of one kind apart when neither
      // label carries a name.
      entityId?: string;
      count: number;
      // A chip a reader cannot open needs its name more than an openable
      // one: there is no click to find out which record it means. Without
      // it an activity reads "activity", the run of identical labels the
      // counted chip avoids. Absent once the chip counts several.
      name?: string;
    } & CitationReceipt);

/** What a citation carries beyond the record it points at. */
function receiptOf(cited: Cited): CitationReceipt {
  return { quote: cited.quote, at: cited.at, origin: cited.origin };
}

/** Whether a citation has anything to rest on: words, a date or an origin. */
function hasReceipt(cited: Cited): boolean {
  return Boolean(cited.quote || cited.at || cited.origin);
}

/**
 * citationChips turns a sentence's raw evidence into what a reader should see.
 *
 * Three reductions, all of which the raw list gets wrong on its own. The same
 * record cited twice is one source, not two. Several records of a kind the app
 * cannot open are one statement about that kind. One by one they would read
 * "activity activity activity", which says less than the count. Several receipt
 * citations of the same kind (`groupable`) are one counted chip too. It opens
 * the first and steps through the rest. A receipt has no name of its own, so
 * ten profile fields would read "profile field" ten times. `deal`/`contact`
 * stay one chip per record: each opens its own screen rather than a shared
 * stepper, so collapsing them would drop every record after the first.
 *
 * Order is first-seen, so the chips follow the sentence's own reasoning.
 */
export function citationChips(
  evidence: readonly Cited[],
  // Asked about the citation, not about its kind. Whether an activity opens
  // depends on the row. One carrying an email summary this reader may receive
  // opens the message, and one carrying none opens nothing. A kind-only
  // predicate gives both rows one answer: two dead controls, or a hidden
  // live one.
  openable: (cited: Cited) => boolean,
  groupable: (entityType: CitedKind) => boolean = () => false,
): CitationChip[] {
  const chips: CitationChip[] = [];
  const seen = new Map<string, CitationChip>();
  const groups = new Map<CitedKind, CitationChip>();
  for (const cited of evidence) {
    const identity = `${cited.entity_type}:${cited.entity_id}`;
    const already = seen.get(identity);
    if (already !== undefined) {
      nameRepeat(already, cited);
      continue;
    }
    const isOpenable = openable(cited);
    // Its own chip: a record with a page of its own, or one carrying a
    // receipt. A citation with a receipt is never folded into a count. The
    // quote is about one record, and a chip for three activities showing one
    // message's words would claim the other two said the same.
    if ((isOpenable && !groupable(cited.entity_type)) || hasReceipt(cited)) {
      const own = ownChip(cited, isOpenable);
      seen.set(identity, own);
      chips.push(own);
      continue;
    }
    const grouped = groups.get(cited.entity_type);
    if (grouped === undefined) {
      // The record's chip is the one just made. Recorded per branch rather
      // than once above, because a record joining an existing group lands on
      // that group's chip and not on a new one.
      const chip: CitationChip = isOpenable
        ? {
            openable: true,
            entityType: cited.entity_type,
            entityId: cited.entity_id,
            count: 1,
          }
        : {
            openable: false,
            entityType: cited.entity_type,
            count: 1,
            name: cited.name,
          };
      seen.set(identity, chip);
      groups.set(cited.entity_type, chip);
      chips.push(chip);
      continue;
    }
    seen.set(identity, grouped);
    grouped.count += 1;
    // A chip speaking for several records carries none of their names. A
    // chip reading "Slots for the pilot review" over three activities claims
    // the other two are that thread as well.
    grouped.name = undefined;
  }
  return chips;
}

// The same record cited twice is one source, so the chip stays. The server
// names a record per citation, and nothing promises the first mention carried
// the name. Dropping the repeat could throw away the only name there is. One
// record may speak with its own name; `count === 1` says it is not a group.
function nameRepeat(held: CitationChip, cited: Cited) {
  if (held.name === undefined && held.count === 1 && cited.name) {
    held.name = cited.name;
  }
}

// A chip standing for this one record, with whatever receipt it carries.
function ownChip(cited: Cited, isOpenable: boolean): CitationChip {
  return isOpenable
    ? {
        openable: true,
        entityType: cited.entity_type,
        entityId: cited.entity_id,
        count: 1,
        name: cited.name,
        ...receiptOf(cited),
      }
    : {
        openable: false,
        entityType: cited.entity_type,
        entityId: cited.entity_id,
        count: 1,
        name: cited.name,
        ...receiptOf(cited),
      };
}

// The citation kinds that open a receipt, not a screen: where the value came
// from and what could not be recorded. Only these can be stepped through.
const RECEIPT_KINDS = [
  "fact",
  "profile_field",
] as const satisfies readonly CitedKind[];

export const RECEIPT_CITATIONS: ReadonlySet<string> = new Set(RECEIPT_KINDS);

function opensReceipt(kind: string): kind is CitedSibling["entityType"] {
  return RECEIPT_CITATIONS.has(kind);
}

type CitationDoors = {
  onOpenRecord?: (entityType: string, entityId: string) => void;
  onOpenReceipt?: OpenReceipt;
  siblings: readonly CitedSibling[];
};

// What pressing a chip does, or undefined where the host has no door for its
// kind. It is the one answer the chip's openability and its click both read.
export function doorOf(kind: CitedKind, id: string, doors: CitationDoors) {
  const { onOpenRecord, onOpenReceipt, siblings } = doors;
  if (opensReceipt(kind)) {
    const cited = { entityType: kind, entityId: id };
    return onOpenReceipt && (() => onOpenReceipt(cited, siblings));
  }
  if (onOpenRecord && citationOpensRecord(kind)) {
    return () => onOpenRecord(kind, id);
  }
  return undefined;
}

/**
 * The message behind a citation, when there is one this reader may open.
 *
 * Asked here once rather than at each call site. The citation has to be an
 * activity, and the server has to have sent the canonical email row for it. It
 * sends that only for an email this reader may receive a summary of. Whether
 * the host mounts a drawer to open it into is the caller's question.
 *
 * A `withheld` summary means the row exists and the words are not this
 * reader's. It still renders, as a reference with no subject and no control,
 * because drawing nothing would say the message did not happen.
 */
export function emailOf(cited: Cited): Cited["email_summary"] | undefined {
  return cited.entity_type === "activity"
    ? (cited.email_summary ?? undefined)
    : undefined;
}

/** One steppable citation, in the receipt's own shape. */
export type CitedSibling = {
  entityType: (typeof RECEIPT_KINDS)[number];
  entityId: string;
};

/** Opens one cited receipt, with the sentence's receipts to step through. */
export type OpenReceipt = (
  cited: CitedSibling,
  siblings: readonly CitedSibling[],
) => void;

// The sentence's receipt-bearing citations, once each, in the order it cites
// them. Mapped here at the one place that knows both shapes: the wire is
// snake_case and the drawer's CitedRecord is not.
export function dedupeCited(evidence: readonly Cited[]): CitedSibling[] {
  const seen = new Set<string>();
  const out: CitedSibling[] = [];
  for (const each of evidence) {
    const key = `${each.entity_type}:${each.entity_id}`;
    if (!opensReceipt(each.entity_type) || seen.has(key)) {
      continue;
    }
    seen.add(key);
    out.push({ entityType: each.entity_type, entityId: each.entity_id });
  }
  return out;
}

// The cited messages, once each, in the order the sentence names them.
// A message keeps whichever citation the server named first. Two mentions of
// one activity carry the same summary row, so the later one adds nothing.
export function dedupeMessages(evidence: readonly Cited[]): Cited[] {
  const seen = new Set<string>();
  const out: Cited[] = [];
  for (const cited of evidence) {
    if (!emailOf(cited) || seen.has(cited.entity_id)) {
      continue;
    }
    seen.add(cited.entity_id);
    out.push(cited);
  }
  return out;
}

/**
 * What one chip says. The answer is the same whether or not it can be
 * opened, which is why it is decided once here.
 *
 * A chip that stands for one record names that record. "deal" tells a reader
 * nothing they could not already see, and the deal's own name tells them which.
 * A chip a reader cannot open needs that most, because no click shows which
 * activity it means. A bare "activity" is the run of labels this design avoids.
 *
 * A chip that stands for several names the count and the kind instead. It
 * carries no name by then (citationChips drops it): one member's name would
 * read as though it spoke for the rest. A grouped receipt chip opens the first,
 * and the drawer's stepper reaches the others.
 *
 * The kind label is also what a record with no name falls back to. The server
 * sends `name` when it has one, and nothing here invents one when it does not.
 */
export function chipLabel(
  chip: CitationChip,
  t: Translator,
  locale: Locale,
): string {
  return (
    chip.name ??
    (chip.count === 1
      ? t(`co.brief.cite.${chip.entityType}`)
      : t(`co.brief.cite.${chip.entityType}.many`, {
          count: formatNumber(chip.count, locale),
        }))
  );
}
