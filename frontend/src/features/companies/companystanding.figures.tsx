import type { PeekContent } from "../../design-system/peek";
import { ratingTone, statTone } from "../../design-system/readingtone";
import { StatStrip } from "../../design-system/statstrip";
import { omitted } from "../../design-system/surfacestate";
import { useLocale, useT } from "../../i18n";
import type { CompanyTab } from "../../lib/companytab";
import { useRecordZone } from "../../lib/recordzone";
import type { Company360 } from "./company360";
import { MoneyStat } from "./company360.money";
import { PipelineCard, StripWithheld } from "./company360.strip";
import { LastTouchStat } from "./company360lasttouch";
import { NextMeetingStat } from "./companynextmeeting";

/**
 * The figures at the foot of How it stands, in the design's order. They are
 * the next meeting, when we last spoke, what they bought this year and what
 * is open.
 * What they owe past due belongs to the Payment row and the Finance tab.
 * Each cell is the door to the tab that holds its rows.
 */
export function StandingFigures({
  companyId,
  view,
  onOpenTab,
  openDeals,
}: Readonly<{
  companyId: string;
  view: Company360;
  onOpenTab?: (tab: CompanyTab) => void;
  // Which deals the Open deals figure counts.
  openDeals?: PeekContent;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const strip = view.state_strip;
  const door = (tab: CompanyTab) => onOpenTab && (() => onOpenTab(tab));
  if (!strip && omitted(view, "state_strip")) {
    return <StripWithheld />;
  }
  const lifecycle = strip?.account.lifecycle ?? "unknown";
  return (
    <StatStrip
      density="plate"
      label={t("co.strip.title")}
      testId="company-figures"
    >
      <NextMeetingStat
        view={view}
        locale={locale}
        recordZone={recordZone}
        onOpen={door("timeline")}
        t={t}
      />
      <LastTouchStat
        view={view}
        withheld={omitted(view, "last_touch")}
        locale={locale}
        recordZone={recordZone}
        tone={statTone(ratingTone(view.health?.relationship?.rating))}
        onOpen={door("timeline")}
        t={t}
      />
      <MoneyStat
        companyId={companyId}
        locale={locale}
        lifecycle={lifecycle}
        dimension={view.health?.payment}
        onOpen={door("finance")}
        t={t}
      />
      <PipelineCard
        commercial={strip?.commercial}
        peek={openDeals}
        locale={locale}
        recordZone={recordZone}
        onOpen={door("deals")}
        t={t}
      />
    </StatStrip>
  );
}
