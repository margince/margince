import type { components } from "../api/schema";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useFinanceSummary } from "./common";
import type { Grounding } from "./record360";

// A leaf for the small lookup tables the company page and its rail both need
// to draw the same enums the same way. Neither company360.tsx nor
// organizations.tsx imports from here going the OTHER direction, so both may
// import this without a cycle: it is exactly the shared home the two owe each
// other rather than each keeping its own copy that can drift.

type Organization = components["schemas"]["Organization"];
type Lifecycle = NonNullable<Organization["lifecycle"]>;

// Where the account stands with us (PO-DDL-4, ADR-0079/A124), in the words a
// reader sees rather than the wire enum.
export const LIFECYCLE_LABELS: Record<Lifecycle, MessageKey> = {
  unknown: "org.lifecycle.unknown",
  target: "org.lifecycle.target",
  prospect: "org.lifecycle.prospect",
  opportunity: "org.lifecycle.opportunity",
  customer: "org.lifecycle.customer",
  former_customer: "org.lifecycle.former_customer",
  disqualified: "org.lifecycle.disqualified",
};

// Kept in wire order so a picker built off it reads as a progression rather
// than an alphabet. Shared for the same reason LIFECYCLE_LABELS is: the list
// and the labels are the two halves of one enum, and organizations.tsx and
// the rail's own Details grid (companyraildetails.tsx) both build a
// lifecycle picker off it — two copies means the two screens can offer
// different choices for the same field.
export const LIFECYCLE_OPTIONS = [
  "unknown",
  "target",
  "prospect",
  "opportunity",
  "customer",
  "former_customer",
  "disqualified",
] as const;

// What the account is TO US, multi-valued (ADR-0079/A124). Typed against the
// schema union, so a value added upstream fails the build here rather than
// reaching a reader as a raw enum.
export type RelationshipType = NonNullable<
  Organization["relationship_types"]
>[number];

// Beside LIFECYCLE_LABELS because the header draws both vocabularies on one
// line and they OVERLAP: `customer` is a member of each. Two modules cannot
// notice that; one can, which is what relationshipBadges below does.
export const RELATIONSHIP_TYPE_LABELS: Record<RelationshipType, MessageKey> = {
  customer: "org.relType.customer",
  partner: "org.relType.partner",
  supplier: "org.relType.supplier",
  investor: "org.relType.investor",
  portfolio_company: "org.relType.portfolio_company",
  competitor: "org.relType.competitor",
  other: "org.relType.other",
};

/**
 * The relationship types worth drawing beside the account's lifecycle: every
 * one whose label the lifecycle badge is not already printing.
 *
 * An account can be a partner AND a customer, so dropping the second reading
 * would make a true thing look untrue — the two are kept. What is dropped is
 * the same WORD twice: an account whose lifecycle is `customer` and whose
 * relationship types include `customer` is the ordinary shape of a customer,
 * and the header used to render "Customer" beside "Customer" from two fields
 * that happened to agree, reading as a second reading confirming the first.
 *
 * Compared on the rendered LABEL rather than the enum key, because what a
 * reader sees twice is the word: today only `customer` collides by key, and a
 * translation that spells two different keys the same way is the same defect
 * with nothing in the type system to catch it.
 */
export function relationshipBadges(
  org: Pick<Organization, "lifecycle" | "relationship_types">,
  t: (key: MessageKey) => string,
): RelationshipType[] {
  const standing = t(LIFECYCLE_LABELS[org.lifecycle ?? "unknown"]);
  return (org.relationship_types ?? []).filter(
    (relType) =>
      t(RELATIONSHIP_TYPE_LABELS[relType]) !== standing &&
      !spokenFor(relType, org.lifecycle),
  );
}

/**
 * Whether the lifecycle already says what this type says, in its own tense.
 *
 * The label comparison above catches the word repeated exactly. It cannot
 * catch the tense: an account that has stopped buying is a FORMER customer and
 * still carries the `customer` type, because that is what it was — and printed
 * together they read as "Former customer" and "Customer" on one line, which is
 * not two facts but one fact and its own contradiction.
 */
function spokenFor(
  relType: RelationshipType,
  lifecycle: Organization["lifecycle"],
): boolean {
  return lifecycle
    ? (LIFECYCLE_SPEAKS_FOR[relType]?.includes(lifecycle) ?? false)
    : false;
}

// The types a lifecycle speaks for, in any of its tenses. By KEY rather than by
// label, because the two words differ on purpose and only the enum says they
// are the same relationship.
const LIFECYCLE_SPEAKS_FOR: Partial<
  Record<RelationshipType, readonly string[]>
> = {
  customer: ["customer", "former_customer"],
};

// The seven wire size bands, same sharing reason as LIFECYCLE_OPTIONS above.
export const SIZE_BAND_OPTIONS = [
  "1-10",
  "11-50",
  "51-200",
  "201-500",
  "501-1000",
  "1001-5000",
  "5000+",
] as const;

// Exported so worstOf (company360.tsx) reads the same three ranks and the
// same order rather than keeping its own copy of the scale it ranks against.
export const HEALTH_RANK = ["at_risk", "good", "strong"] as const;
export type HealthRating = (typeof HEALTH_RANK)[number];

export const HEALTH_RATING_LABEL: Record<HealthRating, MessageKey> = {
  at_risk: "co.health.rating.atRisk",
  good: "co.health.rating.good",
  strong: "co.health.rating.strong",
};

export const HEALTH_DIMENSION_LABEL: Record<
  "relationship" | "commercial" | "payment",
  MessageKey
> = {
  relationship: "co.health.dim.relationship",
  commercial: "co.health.dim.commercial",
  payment: "co.health.dim.payment",
};

// How many days past due a median has to run before it reads as a habit worth
// naming. Named rather than inlined so the threshold is one number a reader
// can find and argue with.
export const PAYMENT_LATE_DAYS = 5;

/**
 * Payment health, read from the finance summary the page already fetches.
 *
 * Rated from the median days after due, which is the reading that says how they
 * PAY rather than what they owe: one late invoice is an exception, a habit is a
 * habit. Below the sample floor the server sends no median, and this returns
 * nothing: "pays on time" concluded from four invoices is a claim about a
 * habit nobody has observed yet.
 *
 * Overdue money outranks the median. An account that pays promptly and has
 * money outstanding right now is at risk today, whatever its habit.
 */
export function usePaymentHealth(orgId?: string) {
  const t = useT();
  const { locale } = useLocale();
  const { data } = useFinanceSummary(orgId ?? "");
  if (!orgId || !data) {
    return undefined;
  }
  const overdue = data.overdue?.amount_minor ?? 0;
  if (overdue > 0) {
    return {
      rating: "at_risk" as const,
      reason: t("co.health.payment.overdue"),
    };
  }
  const median = data.median_days_after_due;
  if (median == null) {
    return undefined;
  }
  if (median > PAYMENT_LATE_DAYS) {
    return {
      rating: "good" as const,
      reason: t("co.health.payment.late", {
        days: formatNumber(median, locale),
      }),
    };
  }
  return {
    rating: "strong" as const,
    reason:
      median < 0
        ? t("finance.medianEarly", {
            days: formatNumber(Math.abs(median), locale),
          })
        : t("co.health.payment.onTime"),
  };
}

/**
 * The account's health as its named dimensions and one verdict over them.
 *
 * `overall` is the WORST rating present, never an average: an average lets a
 * strong relationship hide a payment problem, and payment problems are the ones
 * a rep must not miss. It is also a sentence a reader can check — "at risk,
 * because payment is at risk" — where a composite number is not.
 *
 * A dimension with no rating is not in the verdict, and the card says how many
 * it was computed from. Three-of-three and one-of-three are different claims.
 */
export function worstOf(
  dimensions: ReadonlyArray<{ rating?: string } | undefined>,
): { overall?: HealthRating; rated: number } {
  const present = dimensions
    .map((dimension) => dimension?.rating)
    .filter((rating): rating is HealthRating =>
      HEALTH_RANK.includes(rating as HealthRating),
    );
  if (present.length === 0) {
    return { rated: 0 };
  }
  const worst = HEALTH_RANK.find((rating) => present.includes(rating));
  return { overall: worst, rated: present.length };
}

/**
 * The account's standing: the health verdict as the word a reader sees, the
 * tone that word is drawn in, and how many of the three dimensions it was read
 * from.
 *
 * ONE spelling, because two surfaces state it — the readings row's health card
 * and the 360's own verdict head. Computed twice they would agree until one of
 * them learned about payment and the other did not, and the page would then
 * carry two answers to "how is this account".
 *
 * `label` is absent when nothing was rated. A head that said "unknown" would
 * be the card inventing a call it was not given.
 */
export function useAccountStanding(
  orgId: string,
  health?: {
    relationship?: { rating: HealthRating; reason: string };
    commercial?: { rating: HealthRating; reason: string };
  },
): {
  overall?: HealthRating;
  rated: number;
  payment?: { rating: HealthRating; reason: string };
  restsOn: Grounding[];
} {
  const t = useT();
  const payment = usePaymentHealth(orgId);
  const dimensions = [
    { key: "relationship" as const, health: health?.relationship },
    { key: "commercial" as const, health: health?.commercial },
    { key: "payment" as const, health: payment },
  ];
  const { overall, rated } = worstOf(
    dimensions.map((dimension) => dimension.health),
  );
  // Only the dimensions that were actually rated: an unrated dimension did not
  // contribute to the call, so listing it under "what this rests on" would
  // claim a reading nobody took.
  const restsOn = dimensions.flatMap((dimension) =>
    dimension.health
      ? [
          {
            key: dimension.key,
            quote: dimension.health.reason,
            from: t(HEALTH_DIMENSION_LABEL[dimension.key]),
          },
        ]
      : [],
  );
  return { overall, rated, payment, restsOn };
}

// The tone the standing pill is drawn in. `strong` is the all-clear and
// `at_risk` is the alarm; `good` sits between them and takes the warn tint
// rather than the calm one, because "good" here means "not failing yet" and a
// green pill over it reads as nothing to do.
export const HEALTH_STANDING_TONE: Record<
  HealthRating,
  "calm" | "warn" | "danger"
> = {
  strong: "calm",
  good: "warn",
  at_risk: "danger",
};
