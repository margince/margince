// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Badge, Skeleton } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { isoWeekOf } from "../../format/calendarday";
import {
  formatDateTime,
  formatDayLong,
  formatDayRange,
  formatNumber,
  formatTimeOfDay,
  hourInZone,
} from "../../format/format";
import { startOfDayInZone, viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useRecordZone } from "../../lib/recordzone";
import type { Worklist } from "../worklist/worklist.queries";
import type { WeeklyReview } from "./brief.queries";
import { type BriefSentence, briefSentence } from "./brief.sentence";
import type { BriefView } from "./brief.view";
import { weekSentence } from "./brief.weeksentence";

import "./brief.css";

// Four greetings, and the boundaries are the reader's day rather than the
// clock's quarters: work starts before noon, the afternoon runs to the end of
// the working day, the evening to the end of the waking one, and what is left
// over is somebody working at an hour nobody should have to.
function greetingKey(hour: number): MessageKey {
  if (hour >= 5 && hour < 12) {
    return "brief.glance.morning";
  }
  if (hour >= 12 && hour < 18) {
    return "brief.glance.afternoon";
  }
  if (hour >= 18 && hour < 22) {
    return "brief.glance.evening";
  }
  return "brief.glance.night";
}

function anonGreetingKey(hour: number): MessageKey {
  if (hour >= 5 && hour < 12) {
    return "brief.glance.morningAnon";
  }
  if (hour >= 12 && hour < 18) {
    return "brief.glance.afternoonAnon";
  }
  if (hour >= 18 && hour < 22) {
    return "brief.glance.eveningAnon";
  }
  return "brief.glance.nightAnon";
}

/** The greeting for the reader at this hour, by name where one is known. */
export function greetingText(
  firstName: string | null,
  hour: number,
  t: (key: MessageKey, values?: Record<string, string>) => string,
): string {
  return firstName
    ? t(greetingKey(hour), { name: firstName })
    : t(anonGreetingKey(hour));
}

/**
 * The greeting on two lines, cut at the reader's name, with the name in the
 * accent ink. The name is found in the translated line rather than spliced in,
 * so each language keeps its own word order around it. With no name in the
 * line, the whole line stands on the second.
 */
export function Greeting({
  text,
  firstName,
}: Readonly<{ text: string; firstName: string | null }>) {
  const at = firstName ? text.lastIndexOf(firstName) : -1;
  if (!firstName || at < 0)
    return (
      <>
        <span className="glance-greeting-lead" />
        <span className="glance-greeting-name">{text}</span>
      </>
    );
  return (
    <>
      <span className="glance-greeting-lead">{text.slice(0, at)}</span>
      <span className="glance-greeting-name">
        <span className="greeting-name">{firstName}</span>
        {text.slice(at + firstName.length)}
      </span>
    </>
  );
}

export type GlanceFacts = Readonly<{
  /** The reader's own name, or null while `/me` is still in flight. */
  firstName: string | null;
  now: Date;
  /** The ranked queue this page is showing, for the opening sentence. Undefined
   *  while it is in flight or after it failed — the sentence is then absent
   *  rather than guessed at. */
  day: Worklist | undefined;
  /** The week that closed, for the weekly's opening sentence. Null when no week
   *  has been written yet; undefined while the read is in flight or after it
   *  failed — a quiet week and an unread one are different sentences. */
  week: WeeklyReview | null | undefined;
  /** Which Brief this is. Each view composes its own sentence: the morning's
   *  from the ranked queue, the weekly's from the counts the week was frozen
   *  with. Neither can describe the other. */
  view: BriefView;
  /** The ranked queue is still being read, so the opening sentence's line is
   *  held rather than pushing the page down when it lands. */
  pending?: boolean;
  /** When the agent wrote the morning brief, for the provenance badge. Only
   *  the reader's own morning carries one. */
  briefedAt?: string;
  /** The page's controls, at the far end of the facts' line. */
  controls?: ReactNode;
  /** What stands at the end of the title's line: the weekly's week stepper. */
  titleAside?: ReactNode;
}>;

export type GlanceProps = GlanceFacts;

/**
 * The first fact on the line under the title. The morning names its day
 * written out; the weekly says when the week was frozen, which only a review
 * that exists can say.
 */
function GlanceWhen({
  view,
  now,
  week,
}: Readonly<{ view: BriefView; now: Date; week: GlanceFacts["week"] }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  if (view === "morning") {
    const date = now.toISOString();
    return (
      <p className="glance-date">
        <time dateTime={date}>{formatDayLong(date, locale, viewerZone())}</time>
      </p>
    );
  }
  if (!week?.generated_at) return null;
  return (
    <p className="glance-frozen">
      {t("brief.report.frozenAt", {
        at: formatDateTime(week.generated_at, locale, recordZone),
      })}
    </p>
  );
}

/**
 * The weekly's title: the report and its calendar week over the days it
 * covers. Before any week has closed it names the report itself.
 */
function WeekTitle({ weekStart }: Readonly<{ weekStart: string | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <>
      <span className="glance-greeting-lead">
        {weekStart && (
          <time dateTime={weekStart}>
            {t("brief.report.kicker", {
              week: formatNumber(isoWeekOf(weekStart), locale),
            })}
          </time>
        )}
        {weekStart && <span className="sr-only">,</span>}
      </span>
      <span className="glance-greeting-name">
        {weekStart
          ? formatDayRange(
              weekStart,
              startOfDayInZone(weekStart, recordZone, 6),
              locale,
              recordZone,
            )
          : t("brief.panel.weekly")}
      </span>
    </>
  );
}

export function BriefGlance({
  firstName,
  now,
  day,
  week,
  view,
  pending,
  briefedAt,
  controls,
  titleAside,
}: GlanceProps) {
  const t = useT();
  const hour = hourInZone(now, viewerZone());
  const { locale } = useLocale();
  const sentence = glanceSentence({ view, day, week }, t, locale);
  return (
    <header className="glance arrive" data-testid="brief-glance">
      <div className="glance-title">
        <Heading size="large" as="h1" className="glance-greeting">
          {view === "weekly" ? (
            <WeekTitle weekStart={week?.local_week_start} />
          ) : (
            <Greeting
              text={greetingText(firstName, hour, t)}
              firstName={firstName}
            />
          )}
        </Heading>
        {titleAside}
      </div>
      <div className="glance-row">
        <div className="glance-facts">
          <GlanceWhen view={view} now={now} week={week} />
          {!narratedWeek(view, week) && (
            <GlanceOpening view={view} sentence={sentence} pending={pending} />
          )}
          {briefedAt && (
            <p>
              <Badge tone="ai">
                {t("brief.briefedAt", {
                  time: formatTimeOfDay(briefedAt, locale, viewerZone()),
                })}
              </Badge>
            </p>
          )}
        </div>
        {controls}
      </div>
    </header>
  );
}

/** A week Margince narrated opens on that narration, under the head; the
 *  counted sentence stands in only where nobody wrote one. */
function narratedWeek(view: BriefView, week: GlanceFacts["week"]): boolean {
  return view === "weekly" && Boolean(week?.narrated_at && week.narrative);
}

function GlanceOpening({
  view,
  sentence,
  pending = false,
}: Readonly<{
  view: BriefView;
  sentence: BriefSentence | null;
  pending?: boolean;
}>) {
  const t = useT();
  if (sentence)
    return (
      <p className="glance-sentence" data-testid="glance-sentence">
        {t(sentence.key, sentence.values)}
      </p>
    );
  if (view === "morning")
    return pending ? (
      <div
        className="glance-sentence glance-sentence-pending"
        aria-hidden="true"
      >
        <Skeleton width="6ch" />
      </div>
    ) : null;
  return <p className="glance-sentence">{t("brief.glance.introWeekly")}</p>;
}

function glanceSentence(
  facts: Pick<GlanceFacts, "view" | "day" | "week">,
  t: ReturnType<typeof useT>,
  locale: ReturnType<typeof useLocale>["locale"],
) {
  if (facts.view === "morning") {
    return briefSentence(facts.day, t, locale);
  }
  return weekSentence(facts.week, t, locale);
}
