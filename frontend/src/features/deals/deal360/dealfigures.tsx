// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import {
  StandingTimeline,
  type TimelineEvent,
  type TimelineGap,
} from "../../../design-system/standingtimeline";
import { StatCard } from "../../../design-system/statcard";
import { StatStrip } from "../../../design-system/statstrip";
import {
  activityDay,
  activityHasHappened,
  activityIsAhead,
} from "../../../format/activitywhen";
import {
  formatDayMonth,
  formatMoneyCompact,
  formatNumber,
  formatPercent,
  identifierNumber,
} from "../../../format/format";
import {
  type Locale,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { closeDayOf } from "../../../lib/closeday";
import { useRecordZone } from "../../../lib/recordzone";
import { activityMarks } from "../../../lib/standingmarks";
import { useStageTimes } from "./dealstagehistory";
import type { DealStandingInputs } from "./dealstanding";
import { lastInbound, nextStepOf, standingOffer } from "./dealstanding";
import { daysWaiting, waitingSince } from "./dealwaiting";

type Activity = components["schemas"]["Activity"];

type Words = Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  zone: string;
}>;

function useWords(): Words {
  const { locale } = useLocale();
  return { t: useT(), plural: usePlural(), locale, zone: useRecordZone() };
}

// The calls and meetings that already happened, newest kept: the axis has room
// for a handful of labels, and the history tab holds every one.
const DONE_EVENTS = 5;
const KIND_LABEL: Record<Activity["kind"], MessageKey> = {
  email: "timeline.kind.email",
  call: "timeline.kind.call",
  meeting: "timeline.kind.meeting",
  note: "timeline.kind.note",
  task: "timeline.kind.task",
  message: "timeline.kind.message",
};

export function DealFigures({
  inputs,
}: Readonly<{ inputs: DealStandingInputs }>) {
  const words = useWords();
  return (
    <StatStrip density="plate">
      <ValueFigure inputs={inputs} words={words} />
      <ProbabilityFigure inputs={inputs} words={words} />
      <DaysInStageFigure inputs={inputs} words={words} />
      <NextStepFigure inputs={inputs} words={words} />
    </StatStrip>
  );
}

type FigureProps = Readonly<{ inputs: DealStandingInputs; words: Words }>;

function ValueFigure({ inputs, words }: FigureProps) {
  const { deal, offers } = inputs;
  const { t, locale } = words;
  const offer = standingOffer(offers);
  let value = t("field.unset");
  if (deal.masked_fields?.includes("amount_minor")) {
    value = t("record.notShown");
  } else if (deal.amount_minor != null && deal.currency) {
    value = formatMoneyCompact(deal.amount_minor, deal.currency, locale);
  }
  return (
    <StatCard
      label={t("deals.amount")}
      value={value}
      detail={
        offer
          ? t("deal.standing.figure.offer", {
              number: offer.offer_number,
              revision: identifierNumber(offer.revision),
            })
          : undefined
      }
    />
  );
}

// The stage's own default, which is what the pipeline says a deal there is
// worth; nothing on the deal states a probability of its own.
function ProbabilityFigure({ inputs, words }: FigureProps) {
  const { t, locale } = words;
  const stage = inputs.stages.find((each) => each.id === inputs.deal.stage_id);
  return (
    <StatCard
      label={t("deal.standing.figure.probability")}
      value={
        stage
          ? formatPercent(stage.win_probability / 100, locale)
          : t("field.unset")
      }
      detail={stage ? t("deal.standing.figure.stageDefault") : undefined}
    />
  );
}

// Whole days since the deal arrived in its current stage, from the stage
// history. A closed deal stands in no open stage. A reader the history is
// refused to has no entry. Both read as not shown rather than as zero.
function DaysInStageFigure({ inputs, words }: FigureProps) {
  const { t, plural, locale } = words;
  const times = useStageTimes(inputs);
  const label = t("deal.standing.figure.daysInStage");
  const days = inputs.deal.stage_id
    ? times?.get(inputs.deal.stage_id)?.days
    : undefined;
  if (days === undefined) {
    return <StatCard label={label} value={t("record.notShown")} />;
  }
  const stage = inputs.stages.find((each) => each.id === inputs.deal.stage_id);
  return (
    <StatCard
      label={label}
      value={plural("standing.days", days, {
        count: formatNumber(days, locale),
      })}
      detail={stage?.name}
    />
  );
}

function NextStepFigure({ inputs, words }: FigureProps) {
  const { t, locale, zone } = words;
  const step = nextStepOf(inputs.activities, inputs.asOf, zone);
  const label = t("deal.standing.figure.next");
  if (!step) {
    return <StatCard label={label} value={t("standing.noneYet")} />;
  }
  const late = Date.parse(step.at) < Date.parse(inputs.asOf);
  let tone: "success" | "danger" | undefined;
  if (step.kind === "meeting") {
    tone = "success";
  } else if (late) {
    tone = "danger";
  }
  return (
    <StatCard
      label={label}
      value={activityDay(step.at, step.precision, locale, zone, formatDayMonth)}
      detail={step.subject ?? t(KIND_LABEL[step.kind])}
      tone={tone}
    />
  );
}

export function DealTimeline({
  inputs,
  onOpenHistory,
}: Readonly<{
  inputs: DealStandingInputs;
  // Where a logged item on the line leads: the History tab.
  onOpenHistory?: () => void;
}>) {
  const words = useWords();
  return (
    <StandingTimeline
      label={words.t("standing.timeline.label", {
        name: inputs.deal.name,
      })}
      asOf={inputs.asOf}
      record={words.zone}
      gap={timelineGap(inputs, words)}
      events={timelineEvents(inputs, words)}
      marks={activityMarks(inputs.activities, {
        ...words,
        record: words.zone,
        asOf: inputs.asOf,
        onOpen: onOpenHistory,
      })}
    />
  );
}

function timelineGap(
  { card, activities, asOf }: DealStandingInputs,
  { t, plural, locale, zone }: Words,
): TimelineGap | undefined {
  const waiting = card ? waitingSince(card, activities) : null;
  if (!waiting) {
    return undefined;
  }
  const days = daysWaiting(waiting, asOf, zone);
  return {
    from: waiting.at,
    title: plural("standing.days", days, {
      count: formatNumber(days, locale),
    }),
    sub: t("standing.noReply"),
  };
}

/**
 * What the axis marks: the calls and meetings held, the buyer's last word,
 * what is booked or due, and the expected close. Read off the loaded page of
 * the timeline; an offer is not drawn, because no field dates when one went
 * out.
 */
export function timelineEvents(
  { deal, activities, asOf }: DealStandingInputs,
  { t, locale, zone }: Words,
): TimelineEvent[] {
  const now = Date.parse(asOf);
  const day = (iso: string) => formatDayMonth(iso, locale, zone);
  const dayOf = (row: Activity) =>
    activityDay(
      row.occurred_at,
      row.occurred_precision,
      locale,
      zone,
      formatDayMonth,
    );
  const titleOf = (row: Activity) => row.subject || t(KIND_LABEL[row.kind]);
  const held = activities
    .filter((row) => isConversation(row) && activityHasHappened(row, now, zone))
    .sort((a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at))
    .slice(0, DONE_EVENTS)
    .map(
      (row): TimelineEvent => ({
        key: row.id,
        at: row.occurred_at,
        title: titleOf(row),
        sub: `${dayOf(row)} · ${t(KIND_LABEL[row.kind])}`,
        kind: "done",
        activityIds: [row.id],
      }),
    );
  const ahead = activities
    .filter(
      (row) =>
        row.kind === "meeting" &&
        row.meeting_status === "booked" &&
        activityIsAhead(row.occurred_at, row.occurred_precision, now, zone),
    )
    .map(
      (row): TimelineEvent => ({
        key: row.id,
        at: row.occurred_at,
        title: titleOf(row),
        sub: `${dayOf(row)} · ${t("deal.standing.timeline.booked")}`,
        kind: "ahead",
      }),
    );
  const tasks = activities.flatMap((row): TimelineEvent[] =>
    row.kind === "task" && !row.is_done && row.due_at
      ? [
          {
            key: row.id,
            at: row.due_at,
            title: titleOf(row),
            sub: day(row.due_at),
            kind: Date.parse(row.due_at) < now ? "late" : "ahead",
          },
        ]
      : [],
  );
  return [
    ...held,
    ...lastWord(activities, dayOf, t),
    ...ahead,
    ...tasks,
    ...closeEvent(deal, day, t),
  ];
}

function isConversation(row: Activity): boolean {
  if (row.kind === "call") {
    return true;
  }
  return (
    row.kind === "meeting" &&
    row.meeting_status !== "booked" &&
    row.meeting_status !== "canceled" &&
    row.meeting_status !== "no_show"
  );
}

function lastWord(
  activities: readonly Activity[],
  dayOf: (row: Activity) => string,
  t: Translator,
): TimelineEvent[] {
  const row = lastInbound(activities);
  if (!row) {
    return [];
  }
  return [
    {
      key: `last-${row.id}`,
      at: row.occurred_at,
      title: t("deal.standing.timeline.theyWrote"),
      sub: `${dayOf(row)} · ${t("deal.standing.timeline.lastContact")}`,
      kind: "last",
      activityIds: [row.id],
    },
  ];
}

// The close is the day the deal closed once it is won or lost, and the horizon
// ahead while it is open.
function closeEvent(
  deal: DealStandingInputs["deal"],
  day: (iso: string) => string,
  t: Translator,
): TimelineEvent[] {
  const closeDay = closeDayOf(deal);
  if (!closeDay) {
    return [];
  }
  return [
    {
      key: "close",
      at: closeDay,
      title: t("deals.close"),
      sub: day(closeDay),
      kind: "end",
    },
  ];
}
