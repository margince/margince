import type { components } from "../api/schema";
import { forReader } from "../format/collate";
import type { Locale, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

// How the facts read off a company's website are arranged for display.
//
// The reader's problem is volume, not shortage: one real account returns
// ninety-odd rows, in which "Shop Devs" and "Shop-Devs" are two entries and
// the same platform is listed under product, service and capability. Rendered
// flat and unranked, the card buries the page and the reader stops believing
// any of it.
//
// Grouping by canonical value is what collapses those, so this module is
// presentation: it decides what the reader sees ONE of, and in what order.
// The server is the authority on which facts exist and it is being taught the
// same canonical form; once it is, this stays correct and simply has nothing
// left to collapse.

type OrganizationFact = components["schemas"]["OrganizationFact"];
type FactField = OrganizationFact["field"];
type FactCategory = OrganizationFact["category"];

/**
 * canonical is the form two spellings of one fact share.
 *
 * Case, punctuation and spacing carry no meaning in a scraped value, so
 * "Shop-Devs", "Shop Devs" and "shop devs" collapse to one key. Diacritics are
 * deliberately KEPT: in German they distinguish real words, and folding them
 * would merge facts that differ.
 */
export function canonical(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, " ")
    .trim();
}

/** FACT_FIELD_LABELS names every fact field in the reader's language.
 *
 * The profile-field map in common.tsx covers a different vocabulary, so a fact
 * field fell through its fallback and rendered as raw English snake_case —
 * "served industry", "company size" — inside an otherwise German page.
 */
export const FACT_FIELD_LABELS: Record<FactField, MessageKey> = {
  founded_year: "co.factField.founded_year",
  employee_range: "co.factField.employee_range",
  phone: "co.factField.phone",
  contact_email: "co.factField.contact_email",
  location: "co.factField.location",
  service: "co.factField.service",
  product: "co.factField.product",
  capability: "co.factField.capability",
  served_industry: "co.factField.served_industry",
  company_size: "co.factField.company_size",
  geography: "co.factField.geography",
  language: "co.factField.language",
  certification: "co.factField.certification",
  partner: "co.factField.partner",
  named_customer: "co.factField.named_customer",
  technology: "co.factField.technology",
  quantified_outcome: "co.factField.quantified_outcome",
  mail_provider: "co.factField.mail_provider",
  email_security: "co.factField.email_security",
  hosting_provider: "co.factField.hosting_provider",
  operated_service: "co.factField.operated_service",
};

export function factFieldLabelKey(field: FactField): MessageKey {
  return FACT_FIELD_LABELS[field];
}

/**
 * OFFERING_RANK breaks the tie when one offering is filed under several fields.
 *
 * A company that sells a platform lists it as a product, a service and a
 * capability across its own pages, and all three land as separate facts. The
 * reader wants the thing once, named the most concrete way it was found.
 */
const OFFERING_RANK: Partial<Record<FactField, number>> = {
  product: 0,
  service: 1,
  capability: 2,
};

export type FactGroup = {
  category: FactCategory;
  facts: OrganizationFact[];
};

const CATEGORY_ORDER: FactCategory[] = [
  "company",
  "offering",
  "market",
  "signal",
];

/** better reports whether `a` should survive a collapse against `b`. */
function better(a: OrganizationFact, b: OrganizationFact): boolean {
  // A human-held value outranks anything a read proposed, whatever the model
  // scored it and whatever field it was filed under: somebody looked at this
  // one. This has to be decided BEFORE the offering rank, or a site read's
  // `product` silently hides a human's `service` for the same offering —
  // machine over human, which is the one ordering this must never produce.
  if ((a.source === "human") !== (b.source === "human")) {
    return a.source === "human";
  }
  const rankA = OFFERING_RANK[a.field] ?? 0;
  const rankB = OFFERING_RANK[b.field] ?? 0;
  if (rankA !== rankB) {
    return rankA < rankB;
  }
  const confA = a.confidence ?? 0;
  const confB = b.confidence ?? 0;
  if (confA !== confB) {
    return confA > confB;
  }
  // Same rank, same confidence: the longer value is the more specific one, and
  // the one a reader learns more from.
  return a.value.length > b.value.length;
}

/**
 * groupFacts collapses duplicate spellings and orders what is left.
 *
 * Facts collapse within a category, not across the whole set: a company that
 * is both a `partner` and a `named_customer` is two true statements, and the
 * categories are what keep them apart. Inside `offering` the collapse spans
 * fields, because product/service/capability are three ways of saying the one
 * thing (see OFFERING_RANK); elsewhere the field is part of the identity.
 */
export function groupFacts(
  facts: readonly OrganizationFact[],
  t: Translator,
  locale: Locale,
): FactGroup[] {
  const groups = new Map<FactCategory, Map<string, OrganizationFact>>();
  for (const fact of facts) {
    const byKey = groups.get(fact.category) ?? new Map();
    groups.set(fact.category, byKey);
    // The SERVER's value_key is the identity whenever it sent one. Facts read
    // off a real site are "Name - what it does", and the server keys them on
    // the normalized name before that separator; recomputing from the whole
    // displayed string left "Frontic - Commerce platform" and "Frontic -
    // Storefront delivery" as two rows, so the collapse this module exists for
    // did nothing on exactly the shape production sends. canonical() remains
    // the fallback for a fact that carries no key.
    const identity = fact.value_key?.trim() || canonical(fact.value);
    // Inside offering the field drops out of the identity so the three
    // spellings of one offering meet; everywhere else it stays part of it.
    const key =
      fact.category === "offering" ? identity : `${fact.field} ${identity}`;
    const held = byKey.get(key);
    if (!held || better(fact, held)) {
      byKey.set(key, fact);
    }
  }
  return CATEGORY_ORDER.filter((category) => groups.has(category)).map(
    (category) => ({
      category,
      facts: [...(groups.get(category) ?? new Map()).values()].sort((a, b) =>
        order(a, b, t, locale),
      ),
    }),
  );
}

// Within a category: the most confident first, then by field and value.
//
// Both are RENDERED in the group, so the tiebreaker is a list a person scans
// and it orders in that reader's own alphabet. Two things follow, and the field
// half is the one that is easy to get half right:
//
//   - The field sorts by its TRANSLATED LABEL, not by the wire identifier. The
//     row draws `t(factFieldLabelKey(field))`, so comparing `employee_range`
//     against `founded_year` orders by words the reader never sees — in German
//     "Mitarbeitende" comes after "Gegründet" while the identifiers go the
//     other way, and the list reads as unsorted.
//   - Both halves go through `forReader`, because code-unit order puts every
//     accented vowel after Z: "Ähnliche Marken" below "Zielgruppe".
//
// Two facts of equal confidence with the same label and value are the same
// fact, so this is still a total order on what a reader can tell apart.
function order(
  a: OrganizationFact,
  b: OrganizationFact,
  t: Translator,
  locale: Locale,
): number {
  const conf = (b.confidence ?? 0) - (a.confidence ?? 0);
  if (conf !== 0) {
    return conf;
  }
  return (
    forReader(
      t(factFieldLabelKey(a.field)),
      t(factFieldLabelKey(b.field)),
      locale,
    ) || forReader(a.value, b.value, locale)
  );
}
