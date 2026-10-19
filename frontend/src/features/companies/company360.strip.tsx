import { StatCard } from "../../design-system/atoms";
import { FactList } from "../../design-system/factlist";
import { StatStrip } from "../../design-system/statstrip";
import { omitted } from "../../design-system/surfacestate";
import {
  formatDate,
  formatDateAbbrev,
  formatMoneyCompact,
  formatNumber,
  formatTimeOfDay,
} from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useRecordZone } from "../../lib/recordzone";
import type { Company360 } from "./company360";
import { type HealthDimension, MoneyStat } from "./company360.money";
import { HealthStat, WITHHELD_READING } from "./company360health";
import { LastTouchStat } from "./company360lasttouch";
import {
  HEALTH_DIMENSION_LABEL,
  HEALTH_RATING_LABEL,
  useHealthReason,
} from "./companylookups";
import type { CompanyTab } from "./companytab";

/**
 * HealthCard is how the relationship stands, in the parts a reader can act on.
 *
 * Each line names a fact, not a single score. A score taken as the max over
 * the account's contacts lets one talkative contact speak for the account.
 * "No inbound for 90 days" says what to do, where "2/100" says only a mood.
 *
 * A part the server could not compute is absent, never zero. Zero is a claim
 * about the account; absence is a fact about the reading.
 */
// The rating vocabulary, worst first. The order is the worst-of rule: a
// verdict is the lowest-ranked rating among the dimensions that have one.
export type StateStrip = NonNullable<Company360["state_strip"]>;

// Whose move it is, in words. Exported because the daily brief's context band
// reads the same `engagement` field. companytoday.tsx takes the label from
// here rather than deriving it a second time.
export const ENGAGEMENT_LABELS: Record<
  NonNullable<StateStrip["engagement"]>["state"],
  MessageKey
> = {
  never_contacted: "co.strip.engagement.never_contacted",
  active: "co.strip.engagement.active",
  waiting_on_them: "co.strip.engagement.waiting_on_them",
  waiting_on_us: "co.strip.engagement.waiting_on_us",
  dormant: "co.strip.engagement.dormant",
};

// The two states that name a problem rather than a condition. Colouring only
// these keeps the brief from reading as a dashboard where every tile is lit.
export const ENGAGEMENT_TONE: Partial<
  Record<NonNullable<StateStrip["engagement"]>["state"], "warning">
> = {
  waiting_on_them: "warning",
  dormant: "warning",
};

/**
 * StateStrip is the readings row under the tab strip: five doors, always five.
 * They are open deals, invoiced, the relationship, the last contact, and what
 * is next, each a reading of the tab it opens. The account's standing is not
 * here. The verdict and its three dimensions sit directly under this row, so a
 * reading is said once.
 *
 * Every slot always draws, and says it has no reading when it has none. A slot
 * that vanishes hides which reading is missing. Only an empty state may claim
 * there is none (SurfaceState's rule). So the three absences are three words:
 * withheld is about the reader, unassessed about how much has been judged, and
 * "no date" about the account. "Never contacted" made up from a withheld
 * engagement is a conclusion a rep acts on, drawn from a permission.
 *
 * It never renders €0 when the figure is unavailable, a cross-currency sum
 * without its conversion source, or "revenue" that is only open deals.
 *
 * Each slot binds its own label and the narrow shape once, since both hold for
 * every branch. The label must not move, or the slot is read twice. Below the
 * two-up width a `row` slot folds to a full-width line (statstrip.css). The
 * fold belongs to the plate (`.stat-strip:has(...)`), so every card carries it.
 * Otherwise the row draws a bordered box among borderless ones.
 */
export function StateStrip({
  companyId,
  view,
  onOpenTab,
}: Readonly<{
  companyId: string;
  view?: Company360;
  // The tab each reading is a reading of. Optional, because a surface outside
  // the record page (the storybook, a mirror) has no tab strip to send anybody
  // to. A door with nowhere behind it is not drawn.
  onOpenTab?: (tab: CompanyTab) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const strip = view?.state_strip;
  if (!strip) {
    // Absent for two reasons, and only `sections_omitted` tells them apart.
    // The caller draws nothing while the composite read is in flight or failed.
    // So with a view here, the read succeeded and this one section did not:
    // it was withheld, or the account state has nothing here yet. Only the
    // first is worth saying. A missing row would read as "no readings for this
    // account", an empty state a permission boundary must never pose as.
    if (view && omitted(view, "state_strip")) {
      return (
        <section className="co-strip-withheld" aria-label={t("co.strip.title")}>
          <p className="surfacestate-withheld">{t("co.section.restricted")}</p>
        </section>
      );
    }
    return null;
  }
  // The stage the money slot reads to know whether there is a window to read.
  // When there is not, the slot names the stage under the word.
  const lifecycle = strip.account.lifecycle;
  // An absent section is named in `sections_omitted`, so a grant boundary is
  // never reported as an account nobody has assessed.
  const healthWithheld = view != null && omitted(view, "health");
  // Read once for the two cards on this section: last contact, and the
  // relationship card's outbound date.
  const touchWithheld = view != null && omitted(view, "last_touch");
  const door = (tab: CompanyTab) => onOpenTab && (() => onOpenTab(tab));
  return (
    // Five readings in one compact row of doors, each into the tab that holds
    // its rows, so the brief below shows without scrolling.
    <StatStrip
      label={t("co.strip.title")}
      testId="company-strip"
      density="compact"
    >
      <PipelineCard
        commercial={strip.commercial}
        dimension={view?.health?.commercial}
        locale={locale}
        recordZone={recordZone}
        onOpen={door("deals")}
        t={t}
      />
      {/* Money is a reading every account gets, not only a customer. On an
          account we never billed it says so, which is a fact about the
          account. An absent card is a hole the reader has to interpret. */}
      <MoneyStat
        companyId={companyId}
        locale={locale}
        lifecycle={lifecycle}
        dimension={view?.health?.payment}
        onOpen={door("finance")}
        t={t}
      />
      {/* The relationship and the last-exchange readings both come from the
          account's exchanges. One says how balanced the talk has been, the
          other how long ago the last word fell. The outbound date and its
          boundary ride along because the reading carries only the inbound
          side. */}
      <HealthStat
        health={view?.health}
        touchWithheld={touchWithheld}
        lastOutboundAt={view?.last_outbound_at ?? undefined}
        asOf={view?.as_of}
        locale={locale}
        withheld={healthWithheld}
        onOpen={door("timeline")}
        t={t}
      />
      <LastTouchStat
        view={view}
        withheld={touchWithheld}
        locale={locale}
        recordZone={recordZone}
        onOpen={door("timeline")}
        t={t}
      />
      <NextStat
        view={view}
        locale={locale}
        recordZone={recordZone}
        onOpen={door("timeline")}
        t={t}
      />
    </StatStrip>
  );
}

// What is next on the calendar with this account: the meeting's day, its
// subject and its hour. Nothing scheduled is a fact about the account and is
// said as one; a withheld calendar is said as withheld.
function NextStat({
  view,
  locale,
  recordZone,
  onOpen,
  t,
}: Readonly<{
  view?: Company360;
  locale: Locale;
  recordZone: string;
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  // This card reads next_meeting and nothing else, so its label says meeting
  // and its door goes where a company's meetings are. A card that names a
  // meeting but opens tasks reads as a contradiction on a company with a due
  // task and no meeting booked.
  //
  // History rather than a meetings tab, because a company has none
  // (companytab.ts). The contact record has one and sends the same card to
  // meetings (contactreadings.tsx), so one question is answered one way.
  //
  // Tasks get no door here: a card that reads meetings cannot also route to
  // the task list, and the tab strip reaches tasks directly.
  const slot = { label: t("co.strip.nextMeeting"), narrow: "row" } as const;
  if (!view || omitted(view, "next_meeting")) {
    return <StatCard onOpen={onOpen} {...slot} value={t(WITHHELD_READING)} />;
  }
  const meeting = view.next_meeting;
  if (!meeting) {
    return (
      <StatCard onOpen={onOpen} {...slot} value={t("co.strip.next.none")} />
    );
  }
  return (
    <StatCard
      onOpen={onOpen}
      {...slot}
      value={formatDateAbbrev(meeting.starts_at, locale, recordZone)}
      detail={join(
        meeting.subject,
        formatTimeOfDay(meeting.starts_at, locale, recordZone),
      )}
    />
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
function PipelineCard({
  commercial,
  dimension,
  locale,
  recordZone,
  onOpen,
  t,
}: Readonly<{
  commercial?: StripCommercial | null;
  // The commercial health reading, shown as this card's basis. The verdict on
  // the health card can then be checked against the deals it was read from.
  dimension?: HealthDimension;
  locale: Locale;
  recordZone: string;
  // Handed to every shape this reading takes: a door on only one would make
  // the way out look like a property of the figure.
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  const healthReason = useHealthReason();
  const basis = dimension ? (
    <FactList
      facts={[
        {
          key: "commercial",
          term: t(HEALTH_DIMENSION_LABEL.commercial),
          value: t(HEALTH_RATING_LABEL[dimension.rating]),
          note: healthReason(dimension),
        },
      ]}
    />
  ) : undefined;
  const slot = {
    basis,
    label: t("co.strip.pipeline"),
    narrow: "row",
  } as const;
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
