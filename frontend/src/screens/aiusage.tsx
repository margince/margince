import { useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useMemo, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import { Button, EmptyState } from "../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatMoney, formatNumber, monthAndYear } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useRouting } from "./ai-routing-query";
import { DecisionSummary } from "./aiusage-decisions";
import { aggregate, CallsByDay, SpendByTask, spendRows } from "./aiusage-spend";
import { QueryGate, QueryStates, unwrap, useMe } from "./common";
import "./aiusage.css";
import { calendarMonth } from "../format/calendarday";
import { viewerZone } from "../format/timezone";

type AiUsage = components["schemas"]["AiUsage"];
export type Month = { from: string; to: string };

export function bandTone(band: string): "warning" | "danger" | undefined {
  if (band === "normal") return undefined;
  if (band === "degraded") return "warning";
  return "danger";
}

// monthAround is the first and last day of the month `offset` months from the
// one `seed` names.
//
// The boundaries are computed in UTC on purpose: once a month is NAMED, its
// first and last day are arithmetic on that name, and re-reading them through a
// zone would shift the window off the month that was asked for. The zone
// question is only about which month "now" falls in, and only currentMonth
// asks it.
function monthAround(seed: string, offset: number): Month {
  const named = new Date(`${seed}T00:00:00Z`);
  const first = new Date(
    Date.UTC(named.getUTCFullYear(), named.getUTCMonth() + offset, 1),
  );
  const last = new Date(
    Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0),
  );
  return {
    from: first.toISOString().slice(0, 10),
    to: last.toISOString().slice(0, 10),
  };
}

// The month the reader is in, as an explicit window.
//
// Explicit, rather than letting the server pick: an unbounded query returns the
// server's UTC month (ai.Meter.UsageWindow), and a stepper counting from the
// reader's month while the view came from UTC's disagrees with itself. In the
// first hours of a month east of UTC that made Previous a no-op — reader-Sept
// minus one is August, which is the month already on screen — and on the last
// evening west of UTC it skipped a month entirely. One reading of "this month",
// used for the first window and every step from it.
export function currentMonth(): Month {
  return monthAround(`${calendarMonth(new Date(), viewerZone())}-01`, 0);
}

function adjacentMonth(month: Month, offset: number): Month {
  return monthAround(month.from, offset);
}

// "This month" is the reader's month, not UTC's. In the first hours of a month
// east of UTC the two disagree — the reader has turned the page and UTC has not
// — and the page then refuses the Next arrow for a month they have already
// left.
function isCurrentMonth(month: Month): boolean {
  return month.from.slice(0, 7) >= calendarMonth(new Date(), viewerZone());
}

// The body is its own component so the per-task rollup can be a useMemo. Inside
// QueryGate's render prop it was an O(days × tasks) fold re-run on every render
// of the card — including the ones a sibling's 60-second clock causes.
function AiUsageBody({
  data,
  month,
  onMonth,
  decisionsBound,
}: Readonly<{
  data: AiUsage;
  month: Month;
  onMonth: (next: Month) => void;
  decisionsBound: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const lines = useMemo(() => aggregate(data.days), [data.days]);
  const rows = useMemo(() => spendRows(lines), [lines]);
  const showCost = lines.some((line) => line.cost_est_minor !== undefined);
  const currency = data.budget.currency ?? "USD";
  const totalCost = lines.reduce(
    (sum, line) => sum + (line.cost_est_minor ?? 0),
    0,
  );
  // Calls that carried usage and no rate. Their spend is in the token columns
  // and not in the total beside them, so the total is SHORT — and a money
  // number that is short without saying so is the one a reader acts on: it is
  // always the smaller figure, and nobody investigates a bill that looks
  // cheaper than expected until it does not.
  const unpricedCalls = lines.reduce(
    (sum, line) => sum + (line.unpriced_calls ?? 0),
    0,
  );
  const taskNames = useMemo(
    () => new Map(lines.map((line) => [line.task, line.task_display_name])),
    [lines],
  );
  const taskSummaries = useMemo(
    () => new Map(lines.map((line) => [line.task, line.task_summary])),
    [lines],
  );
  // Absent when the server priced nothing: a total of zero would state a figure
  // this window has none of.
  const costNote = showCost
    ? [
        `${t("aiusage.costNote")} ${formatMoney(totalCost, currency, locale)}`,
        unpricedCalls > 0
          ? t("aiusage.costPartial", {
              calls: formatNumber(unpricedCalls, locale),
            })
          : "",
      ]
        .filter(Boolean)
        .join(" ")
    : undefined;

  return (
    <>
      <SettingList bleed="settings">
        <SettingRow
          label={t("aiusage.monthLabel")}
          value={monthAndYear(new Date(`${month.from}T00:00`), locale)}
          control={
            // The two arrows keep their own names. An icon announces as nothing,
            // and the row's label names the decision, not which way each moves.
            <>
              <Button
                iconOnly
                aria-label={t("aiusage.prevMonth")}
                onClick={() => onMonth(adjacentMonth(month, -1))}
              >
                <ChevronLeft aria-hidden="true" />
              </Button>
              <Button
                iconOnly
                aria-label={t("aiusage.nextMonth")}
                disabled={isCurrentMonth(month)}
                onClick={() => onMonth(adjacentMonth(month, 1))}
              >
                <ChevronRight aria-hidden="true" />
              </Button>
            </>
          }
        />
      </SettingList>
      <SpendByTask
        rows={rows}
        showCost={showCost}
        currency={currency}
        note={costNote}
      />
      {decisionsBound && (
        <DecisionSummary
          decisions={data.decisions ?? []}
          taskName={(task) => taskNames.get(task) ?? task}
          taskSummary={(task) => taskSummaries.get(task)}
        />
      )}
      {data.days.length > 0 && <CallsByDay days={data.days} />}
    </>
  );
}

// The month's meter, as one query two surfaces share: this card, which lets a
// reader step through months, and the page header, which is fixed on the
// current one. Keyed on the window, so the two are the same fetch exactly when
// they are asking the same question.
export function useAiUsage(month: Month, enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: ["ai-usage", month],
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/ai/usage", {
          params: { query: month },
        }),
      );
      if (!data?.budget || !Array.isArray(data.days)) {
        throw new Error("malformed AI usage response");
      }
      return data;
    },
  });
}

export function AiUsageCard() {
  const t = useT();
  const me = useMe();
  // `ai_diagnostics:read`, which is what the server asks for (ai/usage.go).
  //
  // It asked for `automation:update` once — a write verb guarding a GET,
  // because the runtime's spend was treated as operator information. That is
  // why the seat ceiling stays out of this (capability.ts): a read seat may
  // still read. The object is its own now, so management sees what it spends
  // without holding the automation editor.
  const canSee = useCan("ai_diagnostics", "read");
  // Read once, at mount: the reader's month is what this card is about, and
  // recomputing it per render would churn the query key on the one day of the
  // month it could change.
  const [month, setMonth] = useState<Month>(currentMonth);
  const query = useAiUsage(month, canSee);
  // The decision rates are shown only while a decision model is bound: rows
  // written under a binding outlive it, and a rate for a model nobody runs
  // answers a question nobody is asking. A reader who may not see the routing
  // cannot be told whether one is bound, so the rates stay hidden for them too.
  const canRoute = useCan("ai_routing", "read");
  const routing = useRouting(canSee && canRoute);
  const decisionsBound = routing.data?.routing.decisions !== undefined;

  if (!canSee) {
    // Withheld, not absent. An absent spend card on the AI page is a claim about
    // the DATA — that nothing has been spent, or that this installation does not
    // meter it — where the truth is only that these figures are not this reader's
    // to see. Gated on the /me probe so the notice waits for the grants rather
    // than flashing while they are in flight, and the query above asks the server
    // for nothing because the answer is already known.
    return (
      <Panel title={t("aiusage.title")}>
        <PanelBody>
          <PanelIntro>{t("aiusage.sub")}</PanelIntro>
          <QueryGate query={me} pendingLabel={t("aiusage.title")}>
            {() => <EmptyState>{t("aiusage.withheld")}</EmptyState>}
          </QueryGate>
        </PanelBody>
      </Panel>
    );
  }

  // No bottom margin of its own: `.settings-stack` owns the gap between cards.
  return (
    <Panel title={t("aiusage.title")}>
      <PanelBody>
        <PanelIntro>{t("aiusage.sub")}</PanelIntro>
        {query.data === undefined && (
          <QueryStates query={query} pendingLabel={t("aiusage.title")}>
            {null}
          </QueryStates>
        )}
      </PanelBody>
      {query.data !== undefined && (
        <AiUsageBody
          data={query.data}
          month={month}
          onMonth={setMonth}
          decisionsBound={decisionsBound}
        />
      )}
    </Panel>
  );
}
