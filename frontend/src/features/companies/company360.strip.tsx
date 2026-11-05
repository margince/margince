import { StatCard } from "../../design-system/atoms";
import type { PeekContent } from "../../design-system/peek";
import {
  formatDate,
  formatMoneyCompact,
  formatNumber,
} from "../../format/format";
import { type Locale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { Company360 } from "./company360";
import { WITHHELD_READING } from "./company360health";
import "./company360.css";

type EngagementState = NonNullable<
  NonNullable<Company360["state_strip"]>["engagement"]
>["state"];

// Whose move it is, in words. The standing timeline's silence gap names the
// state with these, so one engagement reads one way across the page.
export const ENGAGEMENT_LABELS: Record<EngagementState, MessageKey> = {
  never_contacted: "co.strip.engagement.never_contacted",
  active: "co.strip.engagement.active",
  waiting_on_them: "co.strip.engagement.waiting_on_them",
  waiting_on_us: "co.strip.engagement.waiting_on_us",
  dormant: "co.strip.engagement.dormant",
};

/** The readings row withheld from this reader, said as withheld so it never
 *  reads as an account with no readings. */
export function StripWithheld() {
  const t = useT();
  return (
    <section className="co-strip-withheld" aria-label={t("co.strip.title")}>
      <p className="surfacestate-withheld">{t("co.section.restricted")}</p>
    </section>
  );
}

/**
 * A median days-after-due as a sentence.
 *
 * Negative days mean they pay before the due date. "-4 days after due" is a
 * puzzle; "typically 4 days early" is the reading. The KPI slot and the
 * finance card share it, so the two cannot describe earliness differently.
 */
export function medianDaysLabel(
  median: number,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string {
  return median < 0
    ? t("finance.medianEarly", { days: formatNumber(Math.abs(median), locale) })
    : t("finance.medianAfterDue", { days: formatNumber(median, locale) });
}

type StripCommercial = NonNullable<
  NonNullable<Company360["state_strip"]>["commercial"]
>;

// The open deals, labelled as what they are: their summed value, never
// "potential" and never "revenue". Unpriced when nothing on the account
// carries a convertible figure. A €0 would claim deals worth nothing, when
// the page in fact cannot price them.
export function PipelineCard({
  commercial,
  locale,
  recordZone,
  onOpen,
  peek,
  t,
}: Readonly<{
  commercial?: StripCommercial | null;
  // Which deals the count is of.
  peek?: PeekContent;
  locale: Locale;
  recordZone: string;
  // Handed to every shape this reading takes: a door on only one would make
  // the way out look like a property of the figure.
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  // No basis: the Commercial row beside the ring already reads its rating.
  const slot = { label: t("co.strip.pipeline"), peek } as const;
  if (!commercial) {
    // A null `commercial` means the caller has no deal grant. So this is the
    // reader's boundary, not an account with nothing running. "No open deals"
    // here would be a conclusion a rep acts on, made up from a permission.
    return <StatCard onOpen={onOpen} value={t(WITHHELD_READING)} {...slot} />;
  }
  // No open deals is not an unpriced one. Saying "no convertible amount" about
  // an account that has nothing open reports a data problem where the truth is
  // that nothing is running.
  if (commercial.open_count === 0) {
    return (
      <StatCard onOpen={onOpen} value={t("co.strip.noOpenDeals")} {...slot} />
    );
  }
  const { open_pipeline_minor_base: value, base_currency: currency } =
    commercial;
  const stalled =
    commercial.stalled_count > 0
      ? t("co.strip.stalled", {
          count: formatNumber(commercial.stalled_count, locale),
        })
      : undefined;
  if (value == null || !currency) {
    // Open deals with no priceable figure still say how many there are. The
    // count is a fact and the money is not, so the count takes its place. The
    // unpriced note stays beside the stalled one, because a reader told only
    // "1 stalled" cannot know nothing was priced.
    return (
      <StatCard
        onOpen={onOpen}
        value={formatNumber(commercial.open_count, locale)}
        detail={join(t("co.strip.unpriced"), stalled)}
        tone={stalled ? "warning" : undefined}
        {...slot}
      />
    );
  }
  // A sum across currencies must name its rate source and date. A total over
  // only some open deals must say so. So a partial total names its share, and a
  // converted one names the oldest rate date behind it.
  const partial = commercial.priced_count < commercial.open_count;
  const converted =
    commercial.converted_count > 0 && commercial.fx_as_of
      ? t("co.strip.convertedAsOf", {
          count: formatNumber(commercial.converted_count, locale),
          date: formatDate(commercial.fx_as_of, locale, recordZone),
        })
      : undefined;
  return (
    <StatCard
      onOpen={onOpen}
      value={formatMoneyCompact(value, currency, locale)}
      tone={stalled ? "warning" : undefined}
      detail={join(
        partial
          ? t("co.strip.pricedPartly", {
              priced: formatNumber(commercial.priced_count, locale),
              total: formatNumber(commercial.open_count, locale),
            })
          : t("co.strip.openDeals", {
              count: formatNumber(commercial.open_count, locale),
            }),
        converted,
        stalled,
      )}
      {...slot}
    />
  );
}

// One detail line from the parts that apply. A card has room for one, and
// dropping a part because another is present loses a qualification when it
// matters most.
function join(...parts: (string | undefined)[]): string {
  return parts.filter(Boolean).join(" · ");
}
