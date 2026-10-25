export {
  RELATIONSHIP_TYPE_LABELS,
  type RelationshipType,
} from "../format/companyrelationships";

import type { components } from "../api/schema";
import {
  HEALTH_RATINGS,
  type HealthRating,
} from "../design-system/readingtone";
import { activityDay } from "../format/activitywhen";
import { formatDateAbbrev, formatNumber } from "../format/format";
import {
  type Locale,
  type PluralBase,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useRecordZone } from "../lib/recordzone";
import { useFinanceSummary } from "./financesummary";

// A leaf for the small lookup tables the company page and its rail both need
// to draw the same enums the same way. Neither company360.tsx nor
// companies.tsx imports from here going the OTHER direction, so both may
// import this without a cycle: it is exactly the shared home the two owe each
// other rather than each keeping its own copy that can drift.

type Company = components["schemas"]["Company"];
type Lifecycle = NonNullable<Company["lifecycle"]>;

// Where the account stands with us (PO-DDL-4, ADR-0079/A124), in the words a
// reader sees rather than the wire enum.
export const LIFECYCLE_LABELS: Record<Lifecycle, MessageKey> = {
  unknown: "company.lifecycle.unknown",
  target: "company.lifecycle.target",
  prospect: "company.lifecycle.prospect",
  opportunity: "company.lifecycle.opportunity",
  customer: "company.lifecycle.customer",
  former_customer: "company.lifecycle.former_customer",
  disqualified: "company.lifecycle.disqualified",
};

// Kept in wire order so a picker built off it reads as a progression rather
// than an alphabet. Shared for the same reason LIFECYCLE_LABELS is: the list
// and the labels are the two halves of one enum, and companies.tsx and
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

/**
 * One size band as a count of employees: "51–200 employees". The wire value is
 * a bare range, so every screen that draws a band goes through here to say what
 * the range counts.
 */
export function formatSizeBand(t: Translator, band: string): string {
  return t("company.sizeBandValue", { band: band.replace("-", "–") });
}

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

type HealthDimension = components["schemas"]["HealthDimension"];
type HealthReasonCode = components["schemas"]["HealthDimensionReasonCode"];

// The sentence each reason code stands for, the values it cannot be said
// without, and the one a sentence that changes with a number is counted by.
// The server also sends the English sentence, which a dimension without a code
// (payment, rated here), a code this build does not know and a code missing a
// value it needs all fall back to.
type ReasonParam = "days" | "count" | "total" | "at";
type ReasonMessage = { needs: readonly ReasonParam[] } & (
  | { key: MessageKey }
  | { plural: PluralBase; by: "days" | "count"; today?: MessageKey }
);

const HEALTH_REASON: Record<HealthReasonCode, ReasonMessage> = {
  never_written: { needs: [], key: "co.health.reason.neverWritten" },
  quiet: { needs: ["days"], plural: "co.health.reason.quiet", by: "days" },
  meeting_booked: { needs: ["at"], key: "co.health.reason.meetingBooked" },
  last_met: {
    needs: ["days"],
    plural: "co.health.reason.lastMet",
    by: "days",
    today: "co.health.reason.lastMetToday",
  },
  single_threaded: { needs: [], key: "co.health.reason.singleThreaded" },
  several_contacts: {
    needs: ["count"],
    plural: "co.health.reason.severalContacts",
    by: "count",
  },
  deals_all_stalled: {
    needs: ["count"],
    plural: "co.health.reason.dealsAllStalled",
    by: "count",
  },
  deals_some_stalled: {
    needs: ["count", "total"],
    plural: "co.health.reason.dealsSomeStalled",
    by: "count",
  },
  deals_none_stalled: {
    needs: ["count"],
    plural: "co.health.reason.dealsNoneStalled",
    by: "count",
  },
};

const REASON_NUMBERS = ["days", "count", "total"] as const;

/** The numbers a reason names, grouped for this reader. */
function reasonParams(
  values: NonNullable<HealthDimension["reason_params"]>,
  locale: Locale,
): Record<string, string> {
  const params: Record<string, string> = {};
  for (const name of REASON_NUMBERS) {
    const value = values[name];
    if (value != null) {
      params[name] = formatNumber(value, locale);
    }
  }
  return params;
}

/** Says a health dimension's reason in the reader's language. */
export function useHealthReason(): (
  dimension: Pick<HealthDimension, "reason" | "reason_code" | "reason_params">,
) => string {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  return (dimension) => {
    const message =
      dimension.reason_code && HEALTH_REASON[dimension.reason_code];
    const values = dimension.reason_params ?? {};
    if (!message || message.needs.some((name) => values[name] == null)) {
      return dimension.reason;
    }
    const params = reasonParams(values, locale);
    if (values.at) {
      const precision = values.at_precision ?? "instant";
      params.at = activityDay(
        values.at,
        precision,
        locale,
        zone,
        formatDateAbbrev,
      );
    }
    if ("key" in message) {
      return t(message.key, params);
    }
    const counted = values[message.by] ?? 0;
    if (counted === 0 && message.today) {
      return t(message.today, params);
    }
    return plural(message.plural, counted, params);
  };
}

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
export function usePaymentHealth(companyId?: string) {
  const t = useT();
  const { locale } = useLocale();
  const { data } = useFinanceSummary(companyId ?? "");
  if (!companyId || !data) {
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
  // Counted against the same threshold the rating reads, so "3 of 4 paid on
  // time" can never sit beside a rating that judged by another line.
  const settled = data.payment_behaviour ?? [];
  const onTime =
    settled.length > 0
      ? {
          count: settled.filter((late) => late <= PAYMENT_LATE_DAYS).length,
          total: settled.length,
        }
      : undefined;
  if (median > PAYMENT_LATE_DAYS) {
    return {
      rating: "good" as const,
      reason: t("co.health.payment.late", {
        days: formatNumber(median, locale),
      }),
      onTime,
    };
  }
  return {
    onTime,
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
      HEALTH_RATINGS.includes(rating as HealthRating),
    );
  if (present.length === 0) {
    return { rated: 0 };
  }
  const worst = HEALTH_RATINGS.find((rating) => present.includes(rating));
  return { overall: worst, rated: present.length };
}
