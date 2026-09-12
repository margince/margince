// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRecordZone } from "../app/recordzone";
import { routeHash } from "../app/router";
import { DealCard } from "../design-system/composed";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { type Deal, useMorningDigest } from "./brief.queries";
import { overnightIsEmpty } from "./brief.rail.overnight";
import { scheduleIsEmpty, tasksIsEmpty } from "./brief.schedule";
import { toBoardDeal, useCompanyMarks } from "./deals";
import { rosterOwnerNaming, useRoster } from "./entityref";
import type { Worklist } from "./worklist.queries";

// Brief's context rail: what happened, and what has gone quiet. Every panel is
// READ — the work is in the main column beside them, and a rail that asks for a
// move is a second lead.
//
// It no longer carries what the pipeline is WORTH. A panel of open-pipeline
// money sat here beside the worklist and asked a question the morning cannot
// answer: one figure on a screen about today's work, on its own definition of
// which deals count, next to an Analytics screen answering the same question on
// a different one. Two pipeline summaries that disagree are worse than one
// somewhere else, so the money moved to Analytics, where a reader can see the
// scope and period it was computed over.
//
// Every panel is `Panel` rather than a card of its own, which is what makes the
// rail read as one column of the same shape: a header band at one height,
// full-bleed rows under it, and a footer only where a figure belongs to the
// whole panel.
//
// A PANEL EARNS ITS BOX BY HAVING SOMETHING IN IT. A clear morning used to
// stack four panels of chrome around four grey sentences, and the two panels
// that did have news were read last. Empty, a panel draws nothing and
// `RailQuiet` prints one line per silent source at the foot of the rail.
//
// The overnight digest panel lives in `brief.rail.overnight.tsx` — this file
// was at its line ceiling — and is re-exported here so the rail is still
// assembled from one import.
export { OvernightPanel } from "./brief.rail.overnight";

/**
 * Whether the quiet-deals panel has nothing to draw.
 *
 * `more` is why this is not `deals.length === 0`: past the end of Brief's one
 * page, an empty panel is not "nothing has gone quiet", it is "nothing on the
 * page we read" — a caveat, which is content, and the panel keeps its box to
 * say it.
 */
export function watchIsEmpty(
  deals: readonly Deal[],
  more: boolean,
  state: SectionState,
): boolean {
  return (
    (state === "ready" || state === "empty") && !more && deals.length === 0
  );
}

/**
 * The open deals nobody has heard from.
 *
 * The company on each card is resolved through the SAME naming the pipeline
 * board uses (`useCompanyMarks` + `toBoardDeal`), which is what gives a card its
 * four honest readings — named, withheld, unreadable, or genuinely no company.
 * Brief used to pass `company: ""` unconditionally, so every quiet deal on this page
 * claimed to belong to no company at all.
 */
export function WatchPanel({
  deals,
  more,
  state,
}: Readonly<{
  deals: readonly Deal[];
  /** Whether Brief's one page of deals ended short of the list. A quiet deal
   *  past that page is not on this panel, and `partial` is the state that says
   *  so — "nothing has gone quiet" would be a claim this read cannot make. */
  more: boolean;
  state: SectionState;
}>) {
  const t = useT();
  const recordZone = useRecordZone();
  // No page of companies to draw on here — this is a short list, and every
  // company it names is resolved by id and cached.
  const naming = useCompanyMarks([...deals], [], true);
  const roster = useRoster(
    "user",
    deals.some((deal) => Boolean(deal.owner_id)),
  );
  if (state === "loading" || watchIsEmpty(deals, more, state)) {
    return null;
  }
  // A settled read that ended the list has already collapsed above; what is
  // left is either rows, a page that stopped short — `partial`, the caveat
  // under the rows — or a read that did not answer, which keeps its own state.
  // A failure must never resolve to a sentence about the deals: that told a
  // reader their pipeline was healthy on the strength of a request nobody
  // answered.
  const resolved: SectionState = state === "ready" && more ? "partial" : state;
  return (
    <Panel title={t("brief.panel.watch")} className="rail-panel">
      <PanelBody className={deals.length > 0 ? "rail-watch-list" : undefined}>
        <SurfaceState
          state={resolved}
          emptyLabel={t("brief.rail.quietWatch")}
          loadingLabel={t("brief.panel.watch")}
        >
          {deals.map((deal) => (
            <DealCard
              key={deal.id}
              deal={toBoardDeal(deal, naming, rosterOwnerNaming(roster))}
              // A link, so no press handler: nothing here drags, and the
              // address is the whole behaviour.
              href={routeHash({ screen: "deals", id: deal.id })}
              zone={recordZone}
            />
          ))}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}

/**
 * The one panel a silent morning gets: a line per source that had nothing to
 * report.
 *
 * It is the counterweight to every panel above collapsing. Four absent panels
 * say nothing at all — a reader cannot tell a source that was quiet from one
 * the page forgot to draw — and four boxed sentences say it four times as
 * loudly as the news beside them. One line each, at meta size, at the foot of
 * the rail.
 *
 * It reads the SAME inputs the panels do and the same predicates they collapse
 * on, so the rail cannot print "nothing booked" over a schedule panel that
 * drew. The digest it reads itself: the query is the one the overnight panel
 * already holds, answered from cache, so asking again costs no request.
 */
export function RailQuiet({
  day,
  dayState,
  deals,
  more,
  dealsState,
}: Readonly<{
  day: Worklist | undefined;
  dayState: SectionState;
  deals: readonly Deal[];
  more: boolean;
  dealsState: SectionState;
}>) {
  const t = useT();
  const digestQuery = useMorningDigest();
  // In the rail's own order, so a reader who has learned where each panel sits
  // finds its absence in the same place.
  const silent: MessageKey[] = [];
  if (scheduleIsEmpty(day, dayState)) {
    silent.push("brief.rail.quietSchedule");
  }
  if (tasksIsEmpty(day, dayState)) {
    silent.push("brief.rail.quietTasks");
  }
  if (overnightIsEmpty(digestQuery.data)) {
    silent.push("brief.rail.quietOvernight");
  }
  if (watchIsEmpty(deals, more, dealsState)) {
    silent.push("brief.rail.quietWatch");
  }
  if (silent.length === 0) {
    return null;
  }
  return (
    <section id="brief-quiet">
      <Panel title={t("brief.panel.quiet")} className="rail-panel">
        {silent.map((key) => (
          <PanelRow key={key}>
            <span className="t-caption">{t(key)}</span>
          </PanelRow>
        ))}
      </Panel>
    </section>
  );
}
