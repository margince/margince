import { useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useMemo, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  EmptyState,
  SegmentedControl,
  StatCard,
} from "../../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate, QueryStates } from "../../design-system/querystates";
import { StatStrip } from "../../design-system/statstrip";
import { calendarMonth } from "../../format/calendarday";
import { formatMoney, formatNumber, monthAndYear } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { useRouting } from "../../lib/ai-routing-query";
import { aiTaskLabel } from "../../lib/ai-task-label";
import { useCan } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { routeHash } from "../../lib/router";
import { useMe } from "../../lib/session";
import { settingsHref } from "../../lib/settingsrouting";
import { DecisionSummary } from "./aiusage-decisions";
import {
  aggregate,
  monthTotals,
  SPEND_VIEWS,
  SpendByDay,
  SpendByTask,
  type SpendView,
  spendRows,
} from "./aiusage-spend";
import { formatTokens } from "./tokens";
import "./aiusage.css";

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

// The arrows keep their own names: an icon announces as nothing.
function MonthSwitch({
  month,
  onMonth,
}: Readonly<{ month: Month; onMonth: (next: Month) => void }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <span className="aiusage-month">
      <Button
        iconOnly
        aria-label={t("aiusage.prevMonth")}
        onClick={() => onMonth(adjacentMonth(month, -1))}
      >
        <ChevronLeft aria-hidden="true" />
      </Button>
      <span className="aiusage-month-name" aria-live="polite">
        {monthAndYear(new Date(`${month.from}T00:00`), locale)}
      </span>
      <Button
        iconOnly
        aria-label={t("aiusage.nextMonth")}
        disabled={isCurrentMonth(month)}
        onClick={() => onMonth(adjacentMonth(month, 1))}
      >
        <ChevronRight aria-hidden="true" />
      </Button>
    </span>
  );
}

function MonthTotals({
  lines,
  currency,
}: Readonly<{ lines: ReturnType<typeof aggregate>; currency: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const totals = monthTotals(lines);
  const number = (value: number) => formatNumber(value, locale);
  return (
    <>
      <StatStrip
        label={t("aiusage.totals")}
        testId="aiusage-totals"
        density="compact"
      >
        <StatCard
          narrow="row"
          label={t("aiusage.col.cost")}
          value={
            totals.cost === undefined
              ? t("aiusage.notPriced")
              : formatMoney(totals.cost, currency, locale)
          }
        />
        <StatCard
          narrow="row"
          label={t("aiusage.col.calls")}
          value={number(totals.calls)}
        />
        <StatCard
          narrow="row"
          label={t("aiusage.col.tokens")}
          value={`${formatTokens(totals.tokensIn, locale)} / ${formatTokens(totals.tokensOut, locale)}`}
        />
        <StatCard
          narrow="row"
          label={t("aiusage.col.cached")}
          value={number(totals.cached)}
        />
      </StatStrip>
      {totals.cost !== undefined && (
        <p className="t-caption" data-testid="aiusage-cost-note">
          {t("aiusage.costNote")}
          {/* Calls with usage and no rate leave the total short, and a short
              money figure that does not say so is the one a reader acts on. */}
          {totals.unpricedCalls > 0
            ? ` ${plural("aiusage.unpriced", totals.unpricedCalls, {
                count: number(totals.unpricedCalls),
              })}`
            : null}
        </p>
      )}
    </>
  );
}

// Its own component so the per-task rollup can be a useMemo, out of reach of
// the re-renders a sibling's 60-second clock causes.
function AiUsageBody({
  data,
  decisionsBound,
  view,
  onView,
}: Readonly<{
  data: AiUsage;
  decisionsBound: boolean;
  view: SpendView;
  onView: (view: SpendView) => void;
}>) {
  const t = useT();
  const lines = useMemo(() => aggregate(data.days), [data.days]);
  const rows = useMemo(() => spendRows(lines), [lines]);
  const showCost = lines.some((line) => line.cost_est_minor !== undefined);
  const currency = data.budget.currency ?? "USD";
  const taskNames = useMemo(
    () => new Map(lines.map((line) => [line.task, line.task_display_name])),
    [lines],
  );
  const taskSummaries = useMemo(
    () => new Map(lines.map((line) => [line.task, line.task_summary])),
    [lines],
  );

  const decisions = decisionsBound && (
    <DecisionSummary
      decisions={data.decisions ?? []}
      taskName={(task) => aiTaskLabel(task, t, taskNames.get(task))}
      taskSummary={(task) => taskSummaries.get(task)}
    />
  );
  if (rows.length === 0) {
    return (
      <>
        <PanelBody>
          <PanelIntro>{t("aiusage.sub")}</PanelIntro>
          <EmptyState>{t("aiusage.empty")}</EmptyState>
        </PanelBody>
        {decisions}
      </>
    );
  }
  return (
    <>
      <PanelBody className="form-stack">
        <PanelIntro>{t("aiusage.sub")}</PanelIntro>
        <MonthTotals lines={lines} currency={currency} />
        <div className="aiusage-view">
          <SegmentedControl
            label={t("aiusage.view.label")}
            options={SPEND_VIEWS}
            value={view}
            onChange={onView}
            labels={{
              task: t("aiusage.view.task"),
              day: t("aiusage.view.day"),
            }}
          />
        </div>
      </PanelBody>
      {view === "task" ? (
        <SpendByTask rows={rows} showCost={showCost} currency={currency} />
      ) : (
        <SpendByDay days={data.days} showCost={showCost} currency={currency} />
      )}
      {decisions}
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
  // Held here, so the reader's view outlives the body a month step remounts.
  const [view, setView] = useState<SpendView>("task");
  const query = useAiUsage(month, canSee);
  const canReadPrices = useCan("ai_model_rate", "read");
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
    <Panel
      title={t("aiusage.title")}
      titleAction={<MonthSwitch month={month} onMonth={setMonth} />}
      footer={
        canReadPrices && (
          <a className="link-button" href={routeHash(settingsHref("models"))}>
            {t("aiusage.pricesLink")}
          </a>
        )
      }
    >
      {query.data === undefined ? (
        <PanelBody>
          <PanelIntro>{t("aiusage.sub")}</PanelIntro>
          <QueryStates query={query} pendingLabel={t("aiusage.title")}>
            {null}
          </QueryStates>
        </PanelBody>
      ) : (
        <AiUsageBody
          data={query.data}
          decisionsBound={decisionsBound}
          view={view}
          onView={setView}
        />
      )}
    </Panel>
  );
}
