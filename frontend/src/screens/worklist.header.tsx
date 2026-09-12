// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The head of the Worklist: what the day holds, whose day it is, and which cut
// of it is in view.
//
// Its own file because it owns the LANE VOCABULARY as well as the markup —
// the cuts, the dial's name in the address, and the reading of an address that
// names a lane this build has not learnt. Those three and the strip that draws
// them are one thing: a lane the strip offers and the address cannot spell, or
// the reverse, is a link somebody pastes that opens the wrong day.

import { routeHash } from "../app/router";
import { hashWithParams } from "../app/urlstate";
import { SegmentedControl } from "../design-system/atoms";
import { FilterPills } from "../design-system/filterpills";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { completenessText, pillCount } from "./worklist.copy";
import { OwnerPicker } from "./worklist.manager";
import type {
  Worklist,
  WorklistFilter,
  WorklistScope,
} from "./worklist.queries";
import "./worklist.css";

// The cuts through the queue. `all` first because it is the default view.
//
// `as const` rather than a widened annotation, and it is load-bearing: each pill
// reads `worklist.filter.<value>`, so the literal type is what makes a pill
// without a label a compile error. Annotated as the whole filter vocabulary it
// would silently admit the link-only values below, which have no pill copy.
const FILTERS = [
  "all",
  "customer_waiting",
  "leads",
  "deals_at_risk",
  "meetings",
  "tasks",
  "decisions",
  "system",
] as const satisfies readonly WorklistFilter[];

// The narrowings that are reachable by ADDRESS but are not pills.
//
// Each is the destination of a count on another screen — Brief's urgent
// reading, its feed footer, its overnight notice — and each exists so that
// count and its link read ONE filter. None is a lane a reader picks:
// `urgent` is a level rather than a kind of work and already has a figure that
// sends them here, `except_decisions` is a complement that only means something
// on a screen already drawing its own decisions, and `changed_since_brief`
// describes a moment. Offered as pills they would be three cuts nobody asked
// for, so the row stays the seven kinds plus `all` and these arrive by link.
const LINKED_ONLY = [
  "urgent",
  "except_decisions",
  "changed_since_brief",
] as const satisfies readonly WorklistFilter[];

/** A narrowing that only a link can ask for. */
type LinkedOnlyFilter = (typeof LINKED_ONLY)[number];

/** Every filter this build can be addressed with — the pills and the links. */
const ADDRESSABLE: readonly WorklistFilter[] = [...FILTERS, ...LINKED_ONLY];

/** The dial's name in the address. One spelling, read and written. */
export const WORKLIST_FILTER_PARAM = "filter";

/**
 * The lane the address asks for, or the default.
 *
 * An unknown value reads as `all` rather than as an error: the vocabulary grows
 * on the server, and a pasted link naming a lane this build has not learnt
 * should show the day rather than an empty screen or a crash.
 *
 * Read against ADDRESSABLE, not the pill row. A link-only narrowing checked
 * against the pills would fall through to `all` — the door would open the whole
 * queue while the count that sent the reader named a part of it, which is the
 * exact defect these two values were added to fix.
 */
export function worklistFilterFrom(
  params: ReadonlyMap<string, string>,
): WorklistFilter {
  const asked = params.get(WORKLIST_FILTER_PARAM);
  return ADDRESSABLE.find((value) => value === asked) ?? "all";
}

/**
 * Whether this narrowing arrived by link rather than off the pill row.
 *
 * A type guard, so the caption below can spell `worklist.filter.linked.<value>`
 * and have the compiler check that both link-only values have copy — the same
 * protection the pill row gets from FILTERS being `as const`.
 */
export function isLinkedOnlyFilter(
  filter: WorklistFilter,
): filter is LinkedOnlyFilter {
  return (LINKED_ONLY as readonly WorklistFilter[]).includes(filter);
}

/**
 * The address that opens the queue on one narrowing.
 *
 * ONE spelling, because the surfaces that link here count a population first and
 * the count is only true if the door applies the same filter. Two hand-written
 * `#/worklist?filter=…` strings are two chances to typo a value that then falls
 * through to `all` — silently, since an unknown filter is deliberately not an
 * error — and the reader lands on the whole queue under a number describing part
 * of it. That is the defect this function exists to make unrepeatable.
 *
 * Built from the router's own two halves rather than by hand, so an address
 * written here and one the dial on the screen writes are byte-identical — which
 * is what lets the reader arrive by link, press a pill, and press Back.
 */
export function worklistLaneHref(filter: WorklistFilter): string {
  return hashWithParams(
    routeHash({ screen: "worklist" }),
    new Map([[WORKLIST_FILTER_PARAM, filter]]),
  );
}

/**
 * The day's own head: the sentence, the dials over it, the cuts under it.
 *
 * Three tiers, and the order is what each one is ABOUT: what the whole day
 * holds, then whose day and how wide, then which slice of it is on screen.
 */
export function WorklistHeader({
  day,
  loaded,
  scope,
  filter,
  onScope,
  onFilter,
  owner,
  onOwner,
}: Readonly<{
  day: Worklist;
  // How many rows are on screen, which grows as the reader pages.
  loaded: number;
  scope: WorklistScope;
  filter: WorklistFilter;
  owner: string;
  onScope: (next: WorklistScope) => void;
  onFilter: (next: WorklistFilter) => void;
  onOwner: (next: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const scopes = day.scope_options;
  const completeness = completenessText(day, filter, t, locale, loaded);
  return (
    <div className="worklist-header">
      {/* The facts line and the dials on one line: what the day holds, and
          whose day. A dial belongs beside the sentence it changes rather than
          under it, where it read as a control over the pills below. */}
      <div className="worklist-header-line">
        {/* Five figures, ONE scope: the whole assembled day. All five come off
          `summary`, which the server counts over every candidate it weighed
          rather than over the page it cut — so the sentence stays still as the
          reader pages, and a total the browser derived a second way cannot
          disagree with the bands beside it. */}
        <p className="t-h3 worklist-lead">
          {t(
            // `in_play` is optional, and a server that does not send it has not
            // said there is none — it has said nothing. Printing 0 for silence
            // is the under-reporting this line must never do, so the sentence
            // without the figure is drawn instead.
            day.summary.in_play === undefined
              ? "worklist.summary.noMiddle"
              : "worklist.summary",
            {
              urgent: formatNumber(day.summary.urgent, locale),
              due: formatNumber(day.summary.due, locale),
              inPlay: formatNumber(day.summary.in_play ?? 0, locale),
              lower: formatNumber(day.summary.lower_priority, locale),
              total: formatNumber(day.summary.total, locale),
            },
          )}
        </p>
        {/* BOTH scope controls at the trailing end of the header's own line, in
            one group so they share the sentence's line rather than each
            claiming a row of the page. The owner picker was full width under
            the sentence, where a control as wide as the page read as a switch
            over the pills below it rather than as a dial on the day above. */}
        <div className="worklist-header-dials">
          {/* Drawn only when there is a choice: a rep who can see only their
            own work is never offered a switch that would refuse when
            pressed. */}
          {scopes.length > 1 && owner === "" && (
            <SegmentedControl
              options={scopes}
              value={scope}
              onChange={onScope}
              label={t("worklist.scope.label")}
              labels={{
                mine: t("worklist.scope.mine"),
                unassigned: t("worklist.scope.unassigned"),
                team: t("worklist.scope.team"),
                all: t("worklist.scope.all"),
              }}
            />
          )}
          {/* Whose queue. Offered on the same tier the server admits a named
              owner on — `team` in the options means this reader reaches past
              themselves — so the control and the refusal cannot disagree. It
              replaces the scope switch rather than sitting beside it: a named
              owner outranks the scope word, and the pair is a 422. */}
          {scopes.includes("team") && (
            <OwnerPicker owner={owner} onOwner={onOwner} />
          )}
        </div>
      </div>
      {/* The cuts, and the sentence about what they left out, as one block: the
          caption qualifies the pills, so it sits a step under them rather than
          at the header's own interval, where it read as a third control in the
          strip. */}
      <div className="worklist-header-cuts">
        <FilterPills
          pills={FILTERS.map((value) => ({
            value,
            label: t(`worklist.filter.${value}` as const),
            count: pillCount(day, value),
          }))}
          value={filter}
          onChange={onFilter}
          label={t("worklist.filter.label")}
        />
        {/* A narrowing that came by LINK names itself, because no pill is
            pressed to name it. Without this the reader arrives from Brief at a
            short queue with the whole row unselected and nothing saying why —
            which reads as a broken page rather than a narrowed one. The way
            back out is the same control, so the sentence carries it. */}
        {isLinkedOnlyFilter(filter) && (
          <p className="t-caption worklist-completeness">
            {t(`worklist.filter.linked.${filter}` as const)}{" "}
            <button
              type="button"
              className="link-button"
              onClick={() => onFilter("all")}
            >
              {t("worklist.filter.linked.clear")}
            </button>
          </p>
        )}
        {/* What the page is NOT showing. Drawn only when there is a difference
            to report: on a day the queue carries whole, "12 of 12" is noise. */}
        {completeness !== null && (
          <p className="t-caption worklist-completeness">{completeness}</p>
        )}
      </div>
    </div>
  );
}
