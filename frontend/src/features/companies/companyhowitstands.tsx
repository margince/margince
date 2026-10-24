import type { components } from "../../api/schema";
import {
  HEALTH_DIMENSION_LABEL,
  useHealthReason,
  usePaymentHealth,
} from "../../components/companylookups";
import { EmptyState, Skeleton } from "../../design-system/atoms";
import { HowItStands } from "../../design-system/howitstands";
import { Panel, PanelBody } from "../../design-system/panel";
import type { HealthRating } from "../../design-system/readingtone";
import {
  StandingDims,
  type StandingPeek,
} from "../../design-system/standingdims";
import { HealthRing } from "../../design-system/standingring";
import { StandingTimeline } from "../../design-system/standingtimeline";
import { omitted } from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { formatDayMonth } from "../../format/format.dates";
import {
  type Locale,
  type PluralTranslator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { activityMarks } from "../../lib/standingmarks";
import {
  companyDimRows,
  companyVerdict,
  type RatedDimension,
} from "./companystanding";
import { companyTimeline } from "./companystanding.events";
import { StandingFigures } from "./companystanding.figures";
import { contactsInTouch, openDeals } from "./companystanding.peeks";
import type { CompanyTab } from "../../lib/companytab";

type Company360 = components["schemas"]["Company360"];

/**
 * How the account stands: the worst rated dimension as the word, what is owed
 * as the sentence. Then the three ratings as a ring and rows, the account's
 * time, and its four figures. Every part is read from the CRM's own rows. So
 * the sources pill never claims a model.
 *
 * While the 360 is in flight the card holds its place and no word. A neutral
 * "Not assessed" before the read lands would be a call nobody made.
 */
export function CompanyHowItStands({
  companyId,
  view,
  loading,
  failed,
  onOpenTab,
}: Readonly<{
  companyId: string;
  view?: Company360;
  loading: boolean;
  failed: boolean;
  onOpenTab?: (tab: CompanyTab) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const payment = usePaymentHealth(companyId);
  const healthReason = useHealthReason();
  if (loading) {
    return (
      <Panel>
        <PanelBody>
          <Skeleton width="100%" height={240} />
        </PanelBody>
      </Panel>
    );
  }
  if (failed || !view) {
    return (
      <Panel>
        <PanelBody>
          <EmptyState>{t("co.section.unavailable")}</EmptyState>
        </PanelBody>
      </Panel>
    );
  }
  const dimensions = ratedDimensions({
    view,
    payment,
    healthReason,
    plural,
    locale,
  });
  const withheld = omitted(view, "health");
  const verdict = companyVerdict({ dimensions, withheld, moment: view.moment });
  const word = t(verdict.word);
  const words = { t, plural, locale, zone };
  const { events, gap } = companyTimeline(view, words);
  const peeks: Partial<Record<RatedDimension["key"], StandingPeek>> = {
    relationship: contactsInTouch(view, words),
    commercial: openDeals(view, words),
  };
  return (
    <HowItStands
      // Intl throws on an unparseable date, and a server one release out of
      // step answers without `as_of`.
      date={
        Number.isNaN(Date.parse(view.as_of))
          ? t("co.spine.today")
          : formatDayMonth(view.as_of, locale, zone)
      }
      verdict={{ word, sentence: verdict.sentence, tone: verdict.tone }}
      hero={
        <HealthRing
          arcs={dimensions.map((dimension) => ({
            key: dimension.key,
            label: t(HEALTH_DIMENSION_LABEL[dimension.key]),
            rating: dimension.rating,
          }))}
          word={word}
          tone={verdict.tone}
        />
      }
      dims={
        <StandingDims
          dims={companyDimRows(dimensions, withheld).map((row) => ({
            key: row.key,
            label: t(row.label),
            value: t(row.value),
            sub: row.sub,
            tone: row.tone,
            peek: peeks[row.key],
          }))}
        />
      }
      timeline={
        <StandingTimeline
          label={view.company.display_name}
          asOf={view.as_of}
          gap={gap}
          events={events}
          marks={activityMarks(view.activities?.data ?? [], {
            t,
            locale,
            zone,
            asOf: view.as_of,
            onOpen: onOpenTab && (() => onOpenTab("timeline")),
          })}
        />
      }
      figures={
        <StandingFigures
          companyId={companyId}
          view={view}
          onOpenTab={onOpenTab}
          openDeals={peeks.commercial}
        />
      }
    />
  );
}

function ratedDimensions({
  view,
  payment,
  healthReason,
  plural,
  locale,
}: Readonly<{
  view: Company360;
  payment?: {
    rating: HealthRating;
    reason: string;
    onTime?: { count: number; total: number };
  };
  healthReason: ReturnType<typeof useHealthReason>;
  plural: PluralTranslator;
  locale: Locale;
}>): RatedDimension[] {
  const { relationship, commercial } = view.health ?? {};
  return [
    {
      key: "relationship",
      rating: relationship?.rating,
      reason: relationship && healthReason(relationship),
    },
    {
      key: "commercial",
      rating: commercial?.rating,
      reason: commercial && healthReason(commercial),
    },
    {
      key: "payment",
      rating: payment?.rating,
      // A count of settled invoices says more than the habit sentence. The
      // exception is money overdue now, which is why the rating is at risk.
      reason:
        payment?.onTime && payment.rating !== "at_risk"
          ? plural("co.health.payment.paidOnTime", payment.onTime.total, {
              count: formatNumber(payment.onTime.count, locale),
              total: formatNumber(payment.onTime.total, locale),
            })
          : payment?.reason,
    },
  ];
}
