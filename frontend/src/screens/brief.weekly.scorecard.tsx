// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { StatCard } from "../design-system/atoms";
import { PanelBody, PanelGroupHead } from "../design-system/panel";
import { StatStrip } from "../design-system/statstrip";
import { formatNumber } from "../format/format";
import { type Translator, useLocale, useT } from "../i18n";
import type { WeeklyReview } from "./brief.queries";

// The interval between the two blocks is declared beside the week's own, in
// the sheet the panel that holds this one imports. One sheet per surface, not
// one per file that draws part of it.
import "./brief.weekly.css";

// How well the week went, beside what happened in it.
//
// A GROUP inside the week's panel rather than a panel of its own: a titled
// boxed surface standing in another panel's column reads as a second product,
// and the pane already has one head. `PanelGroupHead` is the catalog's name
// for exactly this — the group named one level in, under the panel's title.
//
// Two blocks, each drawn only when the server sent it. An ABSENT block is not a
// block of zeros: a rep who carried no leads did not score zero on the funnel,
// and drawing them an empty row reads as failure at something nobody asked of
// them. The server decides; this file never substitutes a default.

type Scorecard = NonNullable<WeeklyReview["scorecard"]>;
type LeadBlock = NonNullable<Scorecard["lead"]>;
type DealBlock = NonNullable<Scorecard["deal"]>;

export function ScorecardPanel({
  scorecard,
}: Readonly<{ scorecard: Scorecard | undefined }>) {
  const t = useT();
  // A review written before scorecards existed carries none. Nothing is drawn
  // rather than a panel of blanks, which would state a judgement nobody made.
  if (!scorecard) return null;
  const { lead, deal } = scorecard;
  if (!lead && !deal) return null;

  return (
    <>
      <PanelGroupHead title={t("brief.weekly.scorecard.title")} level="h3" />
      <PanelBody className="brief-weekly-scorecard">
        {lead && <LeadBlockStrip block={lead} t={t} />}
        {deal && <DealBlockStrip block={deal} t={t} />}
      </PanelBody>
    </>
  );
}

function LeadBlockStrip({
  block,
  t,
}: Readonly<{ block: LeadBlock; t: Translator }>) {
  const { locale } = useLocale();
  const n = (value: number) => formatNumber(value, locale);
  return (
    <StatStrip
      label={t("brief.weekly.scorecard.leadBlock")}
      testId="scorecard-lead"
    >
      <StatCard
        narrow="row"
        label={t("brief.weekly.scorecard.advanced")}
        value={n(block.advanced)}
        detail={t("brief.weekly.scorecard.advancedBasis")}
      />
      {/* The BREACH is the figure, and what was answered in time is the line
          under it. A card that led with the leads answered in target put the
          reassurance where the reading goes and left the week's one actionable
          number as a footnote. */}
      <StatCard
        narrow="row"
        label={t("brief.weekly.scorecard.answeredInTarget")}
        value={n(block.breached)}
        detail={
          block.breached === 0
            ? t("brief.weekly.scorecard.allInTarget")
            : t("brief.weekly.scorecard.answeredDetail", {
                count: n(block.answered_in_target),
              })
        }
      />
      <StatCard
        narrow="row"
        label={t("brief.weekly.scorecard.meetingsHeld")}
        value={n(block.meetings_held)}
        detail={t("brief.weekly.scorecard.meetingsBasis", {
          booked: n(block.meetings_booked),
          noShow: n(block.meetings_no_show),
        })}
      />
      {/* The partial figure is drawn ONLY when it is non-zero, and it says the
          three counts above are a floor. A zero would be a reassurance nobody
          asked for; a non-zero is a caveat the reader needs. */}
      {block.meetings_partial_history > 0 && (
        <StatCard
          narrow="row"
          label={t("brief.weekly.scorecard.partialHistory")}
          value={n(block.meetings_partial_history)}
          detail={t("brief.weekly.scorecard.partialHistoryBasis")}
        />
      )}
    </StatStrip>
  );
}

function DealBlockStrip({
  block,
  t,
}: Readonly<{ block: DealBlock; t: Translator }>) {
  const { locale } = useLocale();
  const n = (value: number) => formatNumber(value, locale);
  // THREE CARDS READ AGAINST ONE DENOMINATOR, and a week with no open deal has
  // none. "0 of 0 open deals" states a coverage nobody could have and the bar
  // under it draws a share of nothing, so both give way to the plain fact that
  // there was nothing to cover. One arm, because the three are read across as
  // one comparison and a week where two said it differently would be two weeks.
  const noOpen = block.open === 0;
  const ofOpen = noOpen
    ? t("brief.weekly.scorecard.noOpen")
    : t("brief.weekly.scorecard.ofOpen", { total: n(block.open) });
  const share = (filled: number) =>
    noOpen ? undefined : { filled, total: block.open };
  return (
    <StatStrip
      label={t("brief.weekly.scorecard.dealBlock")}
      testId="scorecard-deal"
    >
      <StatCard
        narrow="row"
        label={t("brief.weekly.scorecard.advances")}
        value={n(block.advances)}
        detail={t("brief.weekly.scorecard.regressionsDetail", {
          count: n(block.regressions),
        })}
      />
      {/* Absent when no deal changed stage. A median of nothing is not zero
          days, so the card is omitted rather than drawn as 0. */}
      {block.median_days_in_stage != null && (
        <StatCard
          narrow="row"
          label={t("brief.weekly.scorecard.medianDaysInStage")}
          value={n(block.median_days_in_stage)}
          detail={t("brief.weekly.scorecard.medianBasis")}
        />
      )}
      <StatCard
        narrow="row"
        label={t("brief.weekly.scorecard.withNextStep")}
        value={n(block.with_next_step)}
        meter={share(block.with_next_step)}
        detail={ofOpen}
      />
      <StatCard
        narrow="row"
        label={t("brief.weekly.scorecard.multiThreaded")}
        value={n(block.multi_threaded)}
        meter={share(block.multi_threaded)}
        detail={
          noOpen
            ? t("brief.weekly.scorecard.noOpen")
            : t("brief.weekly.scorecard.multiThreadedBasis", {
                total: n(block.open),
              })
        }
      />
      <StatCard
        narrow="row"
        label={t("brief.weekly.scorecard.closeDateSound")}
        value={n(block.close_date_sound)}
        meter={share(block.close_date_sound)}
        detail={ofOpen}
      />
      <StatCard
        narrow="row"
        label={t("brief.weekly.scorecard.forecastMoves")}
        value={n(block.forecast_up)}
        detail={t("brief.weekly.scorecard.forecastMovesBasis", {
          down: n(block.forecast_down),
        })}
      />
      {/* Drawn only when non-zero, the same rule the partial-history card
          follows: a caveat the reader needs, never a reassurance nobody asked
          for. Absent (rather than zero) means the week was scored before the
          reconstruction existed, and that draws nothing either — there is no
          shortfall to report, because the question was never asked of it. */}
      {block.unreconstructible != null && block.unreconstructible > 0 && (
        <StatCard
          narrow="row"
          label={t("brief.weekly.scorecard.unreconstructible")}
          value={n(block.unreconstructible)}
          detail={t("brief.weekly.scorecard.unreconstructibleBasis")}
        />
      )}
    </StatStrip>
  );
}
