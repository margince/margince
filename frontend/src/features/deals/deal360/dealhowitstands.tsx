// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../../api/schema";
import {
  HowItStands,
  type StandingSources,
  type StandingVerdict,
} from "../../../design-system/howitstands";
import type { ReadingTone } from "../../../design-system/readingtone";
import {
  type StandingDim,
  StandingDims,
} from "../../../design-system/standingdims";
import { formatDayMonth, formatNumber } from "../../../format/format";
import {
  type Locale,
  type Translator,
  translatePlural,
  useLocale,
  useT,
} from "../../../i18n";
import { dealRoleLabel } from "../../../lib/recordlabels";
import { useRecordZone } from "../../../lib/recordzone";
import { routeHash } from "../../../lib/router";
import { DealStageLadder } from "../deals.stepper";
import { DealFigures, DealTimeline } from "./dealfigures";
import { closeWords } from "./dealheaderfacts";
import { committeeRows } from "./dealoverviewrows";
import { useStageTimes } from "./dealstagehistory";
import {
  type Committee,
  closeTone,
  committeeOf,
  committeeTone,
  type DealStandingInputs,
  momentumOf,
  momentumTone,
} from "./dealstanding";

type Deal = components["schemas"]["Deal"];

export type DealStandingHead = Readonly<{
  verdict: StandingVerdict;
  sources: StandingSources;
  // The instant the verdict describes; absent, the page's own read.
  readAt?: string;
}>;

// The ladder sits under the readings rather than beside them: seven stages
// do not fit beside the verdict word.
export function DealHowItStands({
  head,
  inputs,
  onOpenHistory,
}: Readonly<{
  head: DealStandingHead;
  inputs: DealStandingInputs;
  onOpenHistory?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const { deal, stages, ladder } = inputs;
  const subs = useStageSubs(inputs);
  return (
    <HowItStands
      date={formatDayMonth(head.readAt ?? inputs.asOf, locale, zone)}
      sources={head.sources}
      verdict={head.verdict}
      heroLayout="below"
      hero={
        <DealStageLadder
          deal={deal}
          stages={stages}
          variant="bars"
          tone={ladderTone(deal, head.verdict.tone)}
          subs={subs}
          advancing={ladder.advancing}
          advanceRefused={ladder.advanceRefused}
          refusedReasonId={ladder.refusedReasonId}
          onAdvance={ladder.onAdvance}
        />
      }
      dims={
        <StandingDims
          label={t("deal.standing.dims")}
          dims={dealDims(inputs, { t, locale, zone })}
        />
      }
      timeline={<DealTimeline inputs={inputs} onOpenHistory={onOpenHistory} />}
      figures={<DealFigures inputs={inputs} />}
    />
  );
}

// A won deal stands on a green rung and a lost one on a grey one, whatever
// the last reading said. An open deal stands in the verdict's colour.
function ladderTone(deal: Deal, verdictTone: ReadingTone): ReadingTone {
  if (deal.status === "won") {
    return "success";
  }
  return deal.status === "lost" ? "neutral" : verdictTone;
}

// The line under each rung: how many days the deal spent there. The history
// is a human's read only, so for any other reader no rung has a line.
function useStageSubs(
  inputs: DealStandingInputs,
): ReadonlyMap<string, ReactNode> {
  const { locale } = useLocale();
  const times = useStageTimes(inputs);
  return new Map(
    [...(times ?? [])].map(([id, time]) => [
      id,
      translatePlural(
        locale,
        time.current ? "standing.daysNow" : "standing.days",
        time.days,
        { count: formatNumber(time.days, locale) },
      ),
    ]),
  );
}

type Words = Readonly<{ t: Translator; locale: Locale; zone: string }>;

function dealDims(inputs: DealStandingInputs, words: Words): StandingDim[] {
  const { deal, coverage } = inputs;
  return [
    momentumDim(deal, words),
    riskDim(inputs, words),
    coverageDim(
      committeeOf(coverage.coverage, coverage.withheld),
      coverage,
      words,
    ),
  ];
}

// What the records already flag, counted rather than scored: nothing on the
// wire rates a deal's risk, so the reading is the findings themselves. A
// finding the page cannot see is not "none found".
export function riskDim(
  { deal, card, coverage }: DealStandingInputs,
  { t, locale, zone }: Words,
): StandingDim {
  const base = { key: "risk", label: t("deal.standing.risk") };
  const closeDays = closeWords(deal, locale, zone, t).daysLeft;
  const signals = [
    ...(card?.reply_to
      ? [{ key: "reply", title: t("deal.standing.risk.replyOwed") }]
      : []),
    ...(closeTone(deal, closeDays) === "danger"
      ? [{ key: "close", title: t("deal.standing.risk.closePassed") }]
      : []),
    ...(coverage.withheld ? [] : (coverage.coverage?.risks ?? [])).map(
      (risk, index) => ({
        key: `${risk.kind}-${index}`,
        title: t(`coverage.risk.${risk.kind}`),
        sub: risk.summary,
      }),
    ),
  ];
  if (signals.length > 0) {
    return {
      ...base,
      value: translatePlural(
        locale,
        "deal.standing.risk.found",
        signals.length,
        {
          count: formatNumber(signals.length, locale),
        },
      ),
      sub: signals
        .slice(0, 2)
        .map((signal) => signal.title)
        .join(" · "),
      tone: "warning",
      peek: { head: t("deal.standing.peek.risk"), items: signals },
    };
  }
  // "None found" is a finding only about a committee somebody has named. On a
  // deal nobody sits on, no rule has anything to flag, which is no all-clear.
  const committee = committeeOf(coverage.coverage, coverage.withheld);
  if (card !== undefined && committee.state === "read" && committee.named > 0) {
    return { ...base, value: t("deal.standing.risk.none"), tone: "success" };
  }
  return {
    ...base,
    value: t(
      coverage.withheld
        ? "record.notShown"
        : committee.state === "read" && card !== undefined
          ? "standing.notAssessed"
          : "deal.standing.committee.unread",
    ),
    tone: "neutral",
  };
}

function momentumDim(deal: Deal, { t, locale, zone }: Words): StandingDim {
  const momentum = momentumOf(deal);
  const day = (iso: string) => formatDayMonth(iso, locale, zone);
  const base = {
    key: "momentum",
    label: t("deal.standing.momentum"),
    tone: momentumTone(momentum),
  };
  switch (momentum.state) {
    case "stalled":
      return {
        ...base,
        value: t("deal.stalledBadge"),
        sub: momentum.since
          ? t("deal.standing.momentum.stalledSub", {
              date: day(momentum.since),
            })
          : undefined,
      };
    case "on_hold":
      return {
        ...base,
        value: t("deal.standing.momentum.onHold"),
        sub: t("deal.standing.momentum.onHoldSub", {
          date: day(momentum.until),
        }),
      };
    case "active":
      return {
        ...base,
        value: t("deal.standing.momentum.active"),
        sub: t("deal.standing.momentum.activeSub", {
          date: day(momentum.last),
        }),
      };
    default:
      return { ...base, value: t("deal.standing.momentum.quiet") };
  }
}

function coverageDim(
  committee: Committee,
  { coverage, withheld }: DealStandingInputs["coverage"],
  { t, locale }: Words,
): StandingDim {
  const base = {
    key: "coverage",
    label: t("deal.standing.coverage"),
    tone: committeeTone(committee),
  };
  if (committee.state === "unknown") {
    return {
      ...base,
      value: t(withheld ? "record.notShown" : "deal.standing.committee.unread"),
    };
  }
  const { seats, gaps } = committeeRows(coverage, t, (role) =>
    dealRoleLabel(role, t),
  );
  return {
    ...base,
    peek:
      seats.length + gaps.length > 0
        ? {
            on: "both",
            head: t("deal.standing.peek.committee"),
            items: [
              ...seats.map((seat) => ({
                key: seat.key,
                title: seat.name ?? t("coverage.seatWithheld"),
                sub: seat.sub,
                href: seat.name
                  ? routeHash({ screen: "contacts", id: seat.key })
                  : undefined,
              })),
              ...gaps.map((gap) => ({
                key: gap.key,
                title: gap.title,
                right: t("deal.committee.legendGap"),
              })),
            ],
          }
        : undefined,
    value:
      committee.named === 0
        ? t("deal.standing.committee.none")
        : translatePlural(
            locale,
            "deal.standing.committee.named",
            committee.named,
            { count: formatNumber(committee.named, locale) },
          ),
    sub:
      committee.gaps > 0
        ? translatePlural(
            locale,
            "deal.standing.committee.gaps",
            committee.gaps,
            { count: formatNumber(committee.gaps, locale) },
          )
        : translatePlural(
            locale,
            "deal.standing.committee.engaged",
            committee.engaged,
            { count: formatNumber(committee.engaged, locale) },
          ),
  };
}
