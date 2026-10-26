import { ArrowDownLeft, ArrowUpRight } from "lucide-react";
import {
  type CSSProperties,
  type RefObject,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import {
  activityHasBegun,
  type OccurredPrecision,
} from "../format/activitywhen";
import { formatNumber } from "../format/format";
import { formatDayMonth, monthName } from "../format/format.dates";
import { UTC_ZONE } from "../format/timezone";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import { type PeekContent, type PeekItem, usePeek } from "./peek";
import { useScrollRegion } from "./scrollregion";
import {
  axisOf,
  clusterEvents,
  groupMarks,
  instant,
  isWeekly,
  LABEL_MAX_PX,
  type LabelPlace,
  type MarkGroup,
  pct,
  placeLabels,
  RIGHT_ALIGN_AFTER,
  type TimelineKind,
  type TimelineRange,
  tickLabelsShown,
  ticksOf,
} from "./standingtimeline.layout";
import { TIMELINE_ICON } from "./timelineentry";
import "./standingtimeline.css";

/**
 * What a dot says. `done` happened, `late` was owed and slipped, and `last` is
 * the newest contact. `ahead` is dated and has not happened. `end` closes the
 * record's horizon (an expected close, a target end).
 */
export type TimelineEventKind = TimelineKind;

export type TimelineEvent = Readonly<{
  key: string;
  // An ISO date or instant; it places the dot.
  at: string;
  title: string;
  // Formatted by the caller ("3 Jun · meeting").
  sub?: string;
  kind: TimelineEventKind;
  // The logged items this event already stands for, so no mark is drawn a
  // second time beside its label.
  activityIds?: readonly string[];
}>;

/**
 * One logged item or message on the line: what it was, who it was with and
 * when. Drawn as a small glyph chip above the spine, never labelled; marks
 * close together share one chip that opens a card of all of them.
 */
export type TimelineMark = Readonly<{
  // The activity's id, which `TimelineEvent.activityIds` names.
  key: string;
  at: string;
  precision: OccurredPrecision;
  kind: keyof typeof TIMELINE_ICON;
  // Which way a message or call went; drawn as an arrow on the glyph.
  direction?: "inbound" | "outbound";
  // The kind in words ("Email"), the card's head for a lone mark.
  what: string;
  // The subject, or the kind when it has none.
  title: string;
  // Who it was with and which way ("From Dana Reyes"); absent when the row
  // names nobody.
  detail?: string;
  // The date, formatted by the caller in the record's zone.
  when: string;
  // Where pressing the title goes (the History tab).
  onOpen?: () => void;
}>;

/** The silence before today: drawn from `from` to today as a band over the
 *  spine with its length in words. */
export type TimelineGap = Readonly<{
  from: string;
  title: string;
  sub?: string;
}>;

type Pos = CSSProperties & Record<`--${string}`, string>;

/**
 * The record's time on one dated axis. It draws month lines and what happened
 * as solid spine. It adds the silence since the last contact as a band, today
 * as a pill, and what is dated ahead as dotted spine. The canvas is drawn for
 * the eye.
 * A screen reader gets the same events as a list.
 */
export function StandingTimeline({
  events,
  asOf,
  record,
  gap,
  label,
  marks = [],
}: Readonly<{
  events: readonly TimelineEvent[];
  // Logged items and messages to draw as chips on the line. Only those inside
  // the span the events already set are drawn, so a mark never stretches it.
  marks?: readonly TimelineMark[];
  // The instant the record was read at: where today stands. Never the clock.
  asOf: string;
  // The record zone, which says what date today is for a date-only mark.
  record: string;
  gap?: TimelineGap;
  // What the axis is a history of, for a screen reader.
  label: string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const scroller = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLDivElement>(null);
  const canvasPx = useWidth(canvas);
  const region = useScrollRegion(scroller, label);
  if (events.length === 0) {
    return (
      <p className="standing-timeline-empty t-caption">
        {t("standing.timeline.empty")}
      </p>
    );
  }
  const today = instant(asOf);
  const sorted = [...events].sort((a, b) => instant(a.at) - instant(b.at));
  const range = axisOf(
    today,
    sorted.map((event) => instant(event.at)),
  );
  const nowPct = pct(today, range);
  const gapPct = gap ? Math.min(pct(instant(gap.from), range), nowPct) : nowPct;
  const items = sorted.map((event) => ({
    event,
    pct: pct(instant(event.at), range),
    kind: event.kind,
  }));
  const labels = clusterEvents(items, canvasPx).map(({ lead, members }) => {
    const { event, pct: at } = items[lead] ?? { event: sorted[0], pct: 0 };
    const more =
      members.length > 1
        ? plural("standing.timeline.more", members.length - 1, {
            count: formatNumber(members.length - 1, locale),
          })
        : undefined;
    const sub = [event?.sub, more].filter(Boolean).join(" · ");
    return { event, at, sub };
  });
  const places = placeLabels(
    labels.map(({ event, at, sub }) => ({
      pct: at,
      kind: event?.kind ?? "done",
      titleChars: event?.title.length ?? 0,
      subChars: sub.length,
    })),
    canvasPx,
  );
  const rows = Math.max(
    1,
    ...places.map((place) => (place ? place.row + 1 : 0)),
  );
  const canvasStyle: Pos = {
    "--standing-timeline-rows": String(rows),
    "--standing-timeline-label-max": `${LABEL_MAX_PX}px`,
  };
  const gapStyle: Pos = {
    right: `${100 - nowPct}%`,
    "--standing-timeline-room": `${nowPct}%`,
  };
  const drawn = markedEvents(sorted);
  const lane = marks
    .filter(
      (mark) =>
        !drawn.has(mark.key) &&
        instant(mark.at) >= range.start &&
        activityHasBegun(mark.at, mark.precision, today, record),
    )
    .sort((a, b) => instant(a.at) - instant(b.at));
  return (
    <div className="standing-timeline">
      <div ref={scroller} className="standing-timeline-scroll" {...region}>
        <div className="standing-timeline-stage" style={canvasStyle}>
          <div
            ref={canvas}
            className="standing-timeline-canvas"
            aria-hidden="true"
          >
            <Ticks range={range} locale={locale} canvasPx={canvasPx} />
            {gap && (
              <div className="standing-timeline-gap" style={gapStyle}>
                <span className="standing-timeline-gap-title">{gap.title}</span>
                {gap.sub && (
                  <span className="standing-timeline-gap-sub">{gap.sub}</span>
                )}
              </div>
            )}
            <Spine gapPct={gapPct} nowPct={nowPct} />
            {items.map(({ event, pct: at }) => (
              <span
                key={event.key}
                className={`standing-timeline-dot is-${event.kind}`}
                style={{ left: `${at}%` }}
              />
            ))}
            {labels.map(({ event, at, sub }, index) =>
              event ? (
                <EventLabel
                  key={event.key}
                  event={event}
                  sub={sub}
                  at={at}
                  place={places[index] ?? null}
                />
              ) : null,
            )}
            <span
              className="standing-timeline-today"
              style={{ left: `${nowPct}%` }}
            >
              {t("co.spine.today")}
            </span>
          </div>
          <MarkLane
            marks={lane}
            groups={groupMarks(
              lane.map((mark) => pct(instant(mark.at), range)),
              canvasPx,
              nowPct,
            )}
          />
        </div>
      </div>
      <ol className="sr-only" aria-label={label}>
        {sorted.map((event) => (
          <li key={event.key}>
            {event.sub ? `${event.title}, ${event.sub}` : event.title}
          </li>
        ))}
      </ol>
    </div>
  );
}

// The activities the events already draw, so their marks are left out.
function markedEvents(events: readonly TimelineEvent[]): Set<string> {
  return new Set(events.flatMap((event) => event.activityIds ?? []));
}

// The marks' own layer, beside the canvas rather than inside it. The canvas is
// drawn for the eye and hidden from assistive technology. A chip is a control
// a reader must be able to reach.
function MarkLane({
  marks,
  groups,
}: Readonly<{ marks: readonly TimelineMark[]; groups: readonly MarkGroup[] }>) {
  return (
    <div className="standing-timeline-marks">
      {groups.map((group) => (
        <MarkChip
          key={marks[group.members[0] ?? 0]?.key}
          px={group.px}
          members={group.members.flatMap((index) => marks[index] ?? [])}
        />
      ))}
    </div>
  );
}

// The most rows one card lists; the rest are named and left to History.
const CARD_ROWS = 6;

function MarkChip({
  px,
  members,
}: Readonly<{ px: number; members: readonly TimelineMark[] }>) {
  const plural = usePlural();
  const t = useT();
  const { locale } = useLocale();
  const newest = members.at(-1);
  const oldest = members[0];
  const content: PeekContent | undefined =
    newest && oldest
      ? {
          head:
            members.length === 1
              ? newest.what
              : plural("standing.mark.group", members.length, {
                  count: formatNumber(members.length, locale),
                  range:
                    oldest.when === newest.when
                      ? newest.when
                      : t("standing.mark.range", {
                          from: oldest.when,
                          to: newest.when,
                        }),
                }),
          items: [...members].reverse().slice(0, CARD_ROWS).map(markRow),
          foot:
            members.length > CARD_ROWS
              ? plural("standing.mark.more", members.length - CARD_ROWS, {
                  count: formatNumber(members.length - CARD_ROWS, locale),
                })
              : undefined,
        }
      : undefined;
  const {
    ref: peekRef,
    props: peekProps,
    panel: peekPanel,
    open: peekOpen,
  } = usePeek<HTMLButtonElement>({ content, pressOpens: true });
  if (!newest || !content) {
    return null;
  }
  const Icon = TIMELINE_ICON[newest.kind];
  return (
    <>
      <button
        ref={peekRef}
        type="button"
        className="standing-timeline-mark"
        style={{ left: `${px}px` }}
        aria-label={content.head}
        aria-haspopup="true"
        aria-expanded={peekOpen}
        {...peekProps}
      >
        <Icon aria-hidden="true" />
        {members.length === 1 && newest.direction && (
          <span className="standing-timeline-mark-way">
            {newest.direction === "inbound" ? (
              <ArrowDownLeft aria-hidden="true" />
            ) : (
              <ArrowUpRight aria-hidden="true" />
            )}
          </span>
        )}
        {members.length > 1 && (
          <span className="standing-timeline-mark-count">
            {formatNumber(members.length, locale)}
          </span>
        )}
      </button>
      {peekPanel}
    </>
  );
}

function markRow(mark: TimelineMark): PeekItem {
  return {
    key: mark.key,
    title: mark.title,
    sub: [mark.detail, mark.when].filter(Boolean).join(" · "),
    onOpen: mark.onOpen,
  };
}

// The canvas's drawn width, so labels are measured against the room they
// really have; 760px, the design's canvas, until the first layout.
function useWidth(target: RefObject<HTMLDivElement | null>): number {
  const [width, setWidth] = useState(760);
  useLayoutEffect(() => {
    const node = target.current;
    if (!node || typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(([entry]) => {
      if (entry && entry.contentRect.width > 0) {
        setWidth(entry.contentRect.width);
      }
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [target]);
  return width;
}

function Ticks({
  range,
  locale,
  canvasPx,
}: Readonly<{ range: TimelineRange; locale: Locale; canvasPx: number }>) {
  const weekly = isWeekly(range);
  const ticks = ticksOf(range).map((tick) => ({
    ...tick,
    text: weekly
      ? formatDayMonth(new Date(tick.at).toISOString(), locale, UTC_ZONE)
      : monthName(new Date(tick.at).getUTCMonth() + 1, locale),
  }));
  const shown = tickLabelsShown(
    ticks.map((tick) => ({ pct: tick.pct, chars: tick.text.length })),
    canvasPx,
  );
  return ticks.map((tick, index) => (
    <span
      key={tick.at}
      className={
        tick.pct > RIGHT_ALIGN_AFTER
          ? "standing-timeline-tick hangs-left"
          : "standing-timeline-tick"
      }
      style={{ left: `${tick.pct}%` }}
    >
      {shown[index] && (
        <span className="standing-timeline-tick-label">{tick.text}</span>
      )}
    </span>
  ));
}

function Spine({
  gapPct,
  nowPct,
}: Readonly<{ gapPct: number; nowPct: number }>) {
  return (
    <>
      <span
        className="standing-timeline-spine is-past"
        style={{ width: `${gapPct}%` }}
      />
      {nowPct > gapPct && (
        <span
          className="standing-timeline-spine is-waiting"
          style={{ left: `${gapPct}%`, width: `${nowPct - gapPct}%` }}
        />
      )}
      <span
        className="standing-timeline-spine is-ahead"
        style={{ left: `${nowPct}%` }}
      />
      <span className="standing-timeline-now" style={{ left: `${nowPct}%` }} />
    </>
  );
}

function EventLabel({
  event,
  sub,
  at,
  place,
}: Readonly<{
  event: TimelineEvent;
  sub: string;
  at: number;
  place: LabelPlace;
}>) {
  if (!place) {
    return null;
  }
  const onRow: Pos = {
    left: `${at}%`,
    "--standing-timeline-row": String(place.row),
  };
  // A reading label starts a little left of its dot, never past the canvas edge.
  const style: Pos = place.hangsLeft
    ? { right: `${100 - at}%`, "--standing-timeline-row": String(place.row) }
    : {
        left: `max(0px, calc(${at}% - var(--space-2)))`,
        "--standing-timeline-row": String(place.row),
      };
  return (
    <>
      {place.row > 0 && (
        <span className="standing-timeline-lead" style={onRow} />
      )}
      <span
        className={`standing-timeline-label is-${event.kind}${place.hangsLeft ? " hangs-left" : ""}`}
        style={style}
      >
        <span className="standing-timeline-title" title={event.title}>
          {event.title}
        </span>
        {sub && (
          <span className="standing-timeline-sub" title={sub}>
            {sub}
          </span>
        )}
      </span>
    </>
  );
}
