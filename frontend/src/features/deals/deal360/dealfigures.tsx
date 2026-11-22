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
import { ACTIVITY_KIND_LABEL } from "../../../design-system/timelineentry";
import {
  activityDay,
  activityHasHappened,
  activityIsAhead,
} from "../../../format/activitywhen";
import { taskDueOnDay, taskDueZone } from "../../../format/calendarday";
import {
  calendarDaysBetween,
  formatDayMonth,
  formatDueCompact,
  formatMoneyCompact,
  formatNumber,
  formatPercent,
  identifierNumber,
} from "../../../format/format";
import { offerStatusLabel } from "../../../format/offerstatus";
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
import { useCompanyZone } from "../../../lib/companyzone";
import { labelFor } from "../../../lib/labelfor";
import { recordsAnotherMeeting } from "../../../lib/meetingrecord";
import { useViewerId } from "../../../lib/session";
import { activityMarks } from "../../../lib/standingmarks";
import { useViewerZone } from "../../../lib/viewerzone";
import { probabilityWords } from "../dealprobability";
import { useStageTimes } from "./dealstagehistory";
import type { DealStandingInputs } from "./dealstanding";
import { nextStepOf, standingOffer } from "./dealstanding";
import { daysWaiting, waitingSince } from "./dealwaiting";

type Activity = components["schemas"]["Activity"];
type Deal = components["schemas"]["Deal"];
type Offer = components["schemas"]["Offer"];

type Words = Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  zone: string;
  companyZone: string;
}>;

function useWords(): Words {
  const { locale } = useLocale();
  return {
    t: useT(),
    plural: usePlural(),
    locale,
    zone: useViewerZone(),
    companyZone: useCompanyZone(),
  };
}

// The calls and meetings that already happened, newest kept: the axis has room
// for a handful of labels, and the history tab holds every one.
const DONE_EVENTS = 5;
export function DealFigures({
  inputs,
}: Readonly<{ inputs: DealStandingInputs }>) {
  const words = useWords();
  return (
    <StatStrip density="plate">
      <ValueFigure inputs={inputs} words={words} />
      <ProbabilityFigure inputs={inputs} words={words} />
      {inputs.deal.status === "open" ? (
        <DaysInStageFigure inputs={inputs} words={words} />
      ) : (
        <DaysToCloseFigure inputs={inputs} words={words} />
      )}
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
      detail={offer ? offerLine(offer, t) : modelWrittenAmount(deal, t)}
    />
  );
}

// An amount Margince read from the customer's mail says so until a rep edits
// it. A currency the mail did not name is a guess, and says that instead.
function modelWrittenAmount(
  deal: Pick<Deal, "ai_written_fields" | "amount_minor">,
  t: Words["t"],
): string | undefined {
  if (deal.amount_minor == null) {
    return undefined;
  }
  if (deal.ai_written_fields?.includes("currency")) {
    return t("deal.standing.figure.currencyGuessed");
  }
  if (deal.ai_written_fields?.includes("amount_minor")) {
    return t("deal.standing.figure.amountByModel");
  }
  return undefined;
}

// An offer still in progress says so, so it does not read as the source of the value.
function offerLine(offer: Offer, t: Translator): string {
  const named = {
    number: offer.offer_number,
    revision: identifierNumber(offer.revision),
  };
  return offer.status === "accepted"
    ? t("deal.standing.figure.offer", named)
    : t("deal.standing.figure.offerInProgress", {
        ...named,
        status: offerStatusLabel(offer.status, t),
      });
}

// The deal's own probability where somebody set one, else its stage's. A closed
// deal's outcome is settled, so it is certain either way.
function ProbabilityFigure({ inputs, words }: FigureProps) {
  const { t, locale } = words;
  const viewerId = useViewerId();
  const { status } = inputs.deal;
  const label = t("deal.standing.figure.probability");
  if (status !== "open") {
    return (
      <StatCard
        label={label}
        value={formatPercent(status === "won" ? 1 : 0, locale)}
        detail={t("deal.standing.figure.final")}
      />
    );
  }
  const probability = probabilityWords(inputs.deal, t, locale, viewerId);
  if (!probability) {
    return <StatCard label={label} value={t("field.unset")} />;
  }
  const detail = [probability.source, probability.stage].filter(Boolean);
  return (
    <StatCard
      label={label}
      value={probability.value}
      detail={detail.join(" · ") || undefined}
    />
  );
}

// Whole days since the deal arrived in its current stage, from the stage
// history. A reader the history is refused to has no entry, which reads as not
// shown rather than as zero.
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

// How long the deal took, from the day it was opened to the day it closed. An
// imported close with no day has no span to count. Nor does a corrected close
// day that falls before the deal was opened.
function DaysToCloseFigure({ inputs, words }: FigureProps) {
  const { t, plural, locale, zone } = words;
  const { created_at: opened, closed_at: closed } = inputs.deal;
  const label = t("deal.standing.figure.daysToClose");
  if (!closed) {
    return <StatCard label={label} value={t("field.unset")} />;
  }
  const days = calendarDaysBetween(new Date(opened), new Date(closed), zone);
  if (days < 0) {
    return <StatCard label={label} value={t("field.unset")} />;
  }
  return (
    <StatCard
      label={label}
      value={plural("standing.days", days, {
        count: formatNumber(days, locale),
      })}
      detail={t("deal.standing.figure.openedOn", {
        date: formatDayMonth(opened, locale, zone),
      })}
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
      detail={step.subject ?? t(labelFor(ACTIVITY_KIND_LABEL, step.kind))}
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
      history={inputs.history}
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
  { card, asOf }: DealStandingInputs,
  { t, plural, locale, zone }: Words,
): TimelineGap | undefined {
  const waiting = card ? waitingSince(card) : null;
  if (!waiting) {
    return undefined;
  }
  const days = daysWaiting(waiting, asOf, zone);
  return {
    from: waiting.at,
    title: plural("standing.days", days, {
      count: formatNumber(days, locale),
    }),
    sub: t("deal.standing.timeline.currentRequest"),
  };
}

/**
 * What the axis marks: the calls and meetings held, the buyer's last word,
 * what is booked or due, and the expected close. Read off the loaded page of
 * the timeline; an offer is not drawn, because no field dates when one went
 * out.
 */
export function timelineEvents(
  { deal, activities, asOf, lastInbound }: DealStandingInputs,
  { t, locale, zone, companyZone }: Words,
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
  const titleOf = (row: Activity) =>
    row.subject || t(labelFor(ACTIVITY_KIND_LABEL, row.kind));
  const held = activities
    .filter((row) => isConversation(row) && activityHasHappened(row, now, zone))
    .sort((a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at))
    .slice(0, DONE_EVENTS)
    .map(
      (row): TimelineEvent => ({
        key: row.id,
        at: row.occurred_at,
        title: titleOf(row),
        sub: `${dayOf(row)} · ${t(labelFor(ACTIVITY_KIND_LABEL, row.kind))}`,
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
            sub: formatDueCompact(
              row.due_at,
              row.due_has_time,
              locale,
              zone,
              row.due_intent,
            ),
            kind:
              Date.parse(
                taskDueOnDay(
                  row.due_at,
                  row.due_has_time,
                  row.due_intent,
                  taskDueZone(row.due_has_time, zone, companyZone),
                ),
              ) < now
                ? "late"
                : "ahead",
          },
        ]
      : [],
  );
  return [
    ...held,
    ...lastWord(lastInbound, dayOf, t),
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
    row.meeting_status !== "no_show" &&
    !recordsAnotherMeeting(row)
  );
}

function lastWord(
  row: Activity | undefined,
  dayOf: (row: Activity) => string,
  t: Translator,
): TimelineEvent[] {
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
const CLOSE_TITLE: Record<DealStandingInputs["deal"]["status"], MessageKey> = {
  open: "deals.close",
  won: "deals.statusWon",
  lost: "deals.statusLost",
};

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
      title: t(labelFor(CLOSE_TITLE, deal.status)),
      sub: day(closeDay),
      kind: "end",
      outcome: deal.status === "open" ? undefined : deal.status,
    },
  ];
}
