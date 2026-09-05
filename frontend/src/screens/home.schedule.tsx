// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge } from "../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatTimeOfDay } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import { isUnprepared, itemTitle, rowHref } from "./worklist.copy";
import type { Worklist, WorklistItem } from "./worklist.queries";

// The two rail panels the morning is read alongside: what the day is booked
// with, and what this rep owes.
//
// Both are cuts of the ONE worklist answer the work column is drawn from, not
// reads of their own. The rail is context for the work beside it, and a rail
// that fetched separately could show a meeting the queue had already dropped.
//
// NEITHER SORTS. The order is the server's, the same order the queue prints,
// so the rail and the work column cannot disagree about what comes first.

const MEETING = "meeting";
const TASK = "task";

/**
 * The day's schedule, in the order the server ranked it.
 *
 * A meeting with nothing prepared for it carries the badge here as well as in
 * the queue — the same `isUnprepared` the readings strip counts, so the figure
 * in the strip, the badge in the queue and the badge here are one answer.
 */
export function SchedulePanel({
  day,
  state,
}: Readonly<{ day: Worklist | undefined; state: SectionState }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const meetings = rowsFrom(day, MEETING);
  return (
    <section id="home-schedule" aria-label={t("home.panel.schedule")}>
      <Panel title={t("home.panel.schedule")} className="rail-panel">
        {/* The rows are `PanelRow`s and carry the panel's own gutter, so they
            sit in the Panel directly — inside a `PanelBody` they would be
            padded twice and read as an indented block against every other
            panel in the rail. `SurfaceState` draws its sentence either way. */}
        <SurfaceState
          loadingLabel={t("home.panel.schedule")}
          state={state === "ready" && meetings.length === 0 ? "empty" : state}
          emptyLabel={t("home.schedule.clear")}
        >
          {meetings.map((item) => (
            <PanelRow key={item.id} className="rail-schedule-row">
              <span className="t-caption rail-schedule-when">
                {whenOf(item, locale, zone)}
              </span>
              <span className="rail-schedule-what">
                <Title item={item} />
                {isUnprepared(item) && (
                  <Badge tone="warn">{t("worklist.needsPrep")}</Badge>
                )}
              </span>
            </PanelRow>
          ))}
        </SurfaceState>
      </Panel>
    </section>
  );
}

/**
 * What this rep owes: their open tasks, and an honest word about promises.
 *
 * The commitments lane is not wired, so a promise made in a conversation
 * reaches nothing. The panel says so rather than listing tasks under a heading
 * that claims both — a rep who reads "Promises & tasks" and sees only tasks
 * would take the absence of a promise for its absence in the world.
 */
export function PromisesPanel({
  day,
  state,
}: Readonly<{ day: Worklist | undefined; state: SectionState }>) {
  const t = useT();
  const tasks = rowsFrom(day, TASK);
  return (
    <section id="home-promises" aria-label={t("home.panel.promises")}>
      <Panel title={t("home.panel.promises")} className="rail-panel">
        <SurfaceState
          loadingLabel={t("home.panel.promises")}
          state={state === "ready" && tasks.length === 0 ? "empty" : state}
          emptyLabel={t("home.promises.clear")}
        >
          {tasks.map((item) => (
            <PanelRow key={item.id} className="rail-promise-row">
              <Title item={item} />
            </PanelRow>
          ))}
        </SurfaceState>
        {/* Under the list on every reading, including the empty one. It is the
            state of the PRODUCT rather than of this morning, and a reader who
            saw it only on a busy day would read an empty panel as "no promises
            outstanding" — which is exactly the claim nothing here can make. */}
        <PanelBody>
          <p className="t-caption rail-promise-note">
            {t("home.promises.untracked")}
          </p>
        </PanelBody>
      </Panel>
    </section>
  );
}

/** The row's own words, linked where the row names a record. */
function Title({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  const title = itemTitle(item, t, locale);
  const href = rowHref(item);
  return href ? (
    <a className="entity-link t-body" href={href}>
      {title}
    </a>
  ) : (
    <span className="t-body">{title}</span>
  );
}

/**
 * When a meeting starts, or nothing.
 *
 * The start is on `due_at`. The meeting lane puts it there deliberately — it is
 * the deadline a reader is racing (attention/meeting.go) — and sets no
 * `occurred_at` at all, because a meeting on today's schedule has not occurred
 * yet. Reading the other field drew every row with a blank time.
 *
 * A row the server sent without one is still drawn without a time rather than
 * with an invented one.
 */
function whenOf(item: WorklistItem, locale: Locale, zone: string): string {
  return item.due_at ? formatTimeOfDay(item.due_at, locale, zone) : "";
}

/** One source's rows, in the server's order. */
function rowsFrom(day: Worklist | undefined, source: string): WorklistItem[] {
  return (day?.queue ?? []).filter((item) => item.source === source);
}
