import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  EmptyState,
  Field,
  SegmentedControl,
} from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { isISODate } from "../../design-system/dateinput";
import { MonthInput, type YearMonth } from "../../design-system/monthinput";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { formatAnalyticsMoney, identifierNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { useCompanyZone } from "../../lib/companyzone";
import { usePipelines } from "../../lib/pipelines.queries";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import {
  useAnalyticsContext,
  useAnalyticsSelection,
} from "./analytics.context";
import { type SalesGoalRoster, useSalesGoalRoster } from "./progress.queries";
import { reportingAmount, reportingPeriodLabel } from "./reporting.model";
import { useReportingPages } from "./reporting.pagination";
import { useMetricNames } from "./reporting.queries";
import { ReportingTargetDialog } from "./reporting.targetdialog";

type Target = components["schemas"]["ReportingTarget"];
type GoalSeller = SalesGoalRoster["data"][number];
// A typed target, or one team member's published sales goals for the year.
// The twelve months a plan owns read as one row that opens the plan.
type Row =
  | Readonly<{ kind: "target"; target: Target }>
  | Readonly<{ kind: "goals"; seller: GoalSeller; roster: SalesGoalRoster }>;

function rowsOf(
  targets: readonly Target[],
  rosters: readonly (SalesGoalRoster | undefined)[],
): Row[] {
  const goals = rosters.flatMap((roster) =>
    roster
      ? roster.data
          .filter((seller) => seller.total_minor != null)
          .map((seller): Row => ({ kind: "goals", seller, roster }))
      : [],
  );
  return [
    ...targets.map((target): Row => ({ kind: "target", target })),
    ...goals,
  ];
}

export function ReportingTargets() {
  const t = useT();
  const metricName = useMetricNames();
  const { locale } = useLocale();
  const context = useAnalyticsContext();
  const { selection } = useAnalyticsSelection(context.data);
  const pipelines = usePipelines();
  const canCreate = useCanWrite("sales_target", "create");
  const canUpdate = useCanWrite("sales_target", "update");
  const { cursor, next: setCursor, back, canBack } = useReportingPages();
  const [status, setStatus] = useState("active");
  const [periodFilter, setPeriodFilter] = useState<YearMonth | "">("");
  const [editing, setEditing] = useState<Target | "new" | null>(null);
  const query = useQuery({
    queryKey: ["reporting-targets", cursor, status, periodFilter],
    queryFn: async () => {
      return unwrap(
        await api.GET("/analytics/targets", {
          params: {
            query: {
              cursor,
              limit: 50,
              retired: status === "all" ? undefined : status === "retired",
              // A plan's months show as one row per team member below.
              plan_owned: false,
              period_start: isISODate(`${periodFilter}-01`)
                ? `${periodFilter}-01`
                : undefined,
            },
          },
        }),
      );
    },
  });
  const [now] = useState(() => new Date());
  const goalYear = periodFilter
    ? Number(periodFilter.slice(0, 4))
    : now.getFullYear();
  // Published goals are live targets, listed once on the first page. A disabled
  // read keeps its cached rows, so the list takes them only while it is on.
  const rosterOn = status !== "retired" && !cursor;
  const roster = useSalesGoalRoster(goalYear, rosterOn);
  // With no month picked, next year's plans are live targets too.
  const nextRoster = useSalesGoalRoster(
    goalYear + 1,
    rosterOn && !periodFilter,
  );
  const reads = [
    ...(rosterOn ? [roster] : []),
    ...(rosterOn && !periodFilter ? [nextRoster] : []),
  ];
  const unanswered = reads.find((read) => !read.data);
  return (
    <Panel
      title={t("reporting.targets")}
      titleAction={
        canCreate ? (
          <Button onClick={() => setEditing("new")}>
            {t("reporting.newTarget")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <div className="reporting-toolbar">
          <SegmentedControl
            label={t("reporting.targetStatus")}
            value={status}
            options={["active", "retired", "all"]}
            labels={{
              active: t("reporting.activeTargets"),
              retired: t("reporting.retired"),
              all: t("reporting.allTargets"),
            }}
            onChange={(value) => {
              setStatus(value);
              setCursor(undefined);
            }}
          />
          <Field label={t("reporting.periodMonth")}>
            {(field) => (
              <MonthInput
                {...field}
                value={periodFilter}
                onChange={(month) => {
                  setPeriodFilter(month);
                  setCursor(undefined);
                }}
              />
            )}
          </Field>
        </div>
        <QueryGate query={query} pendingLabel={t("reporting.targets")}>
          {(result) => {
            const rows = rowsOf(
              result.data,
              reads.map((read) => read.data),
            );
            // An empty list is only empty once every goals read has answered.
            if (!rows.length && unanswered)
              return (
                <QueryGate
                  query={unanswered}
                  pendingLabel={t("reporting.targets")}
                >
                  {() => null}
                </QueryGate>
              );
            return rows.length ? (
              <>
                <DataTable
                  label={t("reporting.targets")}
                  rows={rows}
                  rowKey={(row) =>
                    row.kind === "target"
                      ? row.target.id
                      : `${row.seller.owner_id}:${row.roster.year}`
                  }
                  columns={[
                    {
                      key: "scope",
                      header: t("reporting.targetFor"),
                      render: (row) =>
                        row.kind === "goals" ? (
                          <>
                            <strong>{row.seller.name}</strong>
                            <div>{metricName("bookings_won")}</div>
                            <div className="t-caption">
                              {t("reporting.goalsRow", {
                                year: identifierNumber(row.roster.year),
                              })}
                            </div>
                            <div className="t-caption">
                              {t("reporting.allPipelines")}
                            </div>
                          </>
                        ) : (
                          <TargetSubject
                            target={row.target}
                            pipelines={pipelines}
                          />
                        ),
                    },
                    {
                      key: "amount",
                      header: t("reporting.target"),
                      render: (row) =>
                        row.kind === "goals" ? (
                          <strong>
                            {formatAnalyticsMoney(
                              row.seller.total_minor ?? 0,
                              row.roster.currency,
                              locale,
                            )}
                          </strong>
                        ) : (
                          <TargetAmount target={row.target} />
                        ),
                    },
                    {
                      key: "edit",
                      header: t("reporting.revise"),
                      headerHidden: true,
                      render: (row) => {
                        if (row.kind === "goals")
                          return (
                            <Button
                              variant="link"
                              onClick={() =>
                                navigate({ screen: "goals" }, goalsDials(row))
                              }
                            >
                              {t("reporting.openGoals")}
                            </Button>
                          );
                        return canUpdate ? (
                          <Button
                            variant="link"
                            onClick={() => setEditing(row.target)}
                          >
                            {t("reporting.revise")}
                          </Button>
                        ) : null;
                      },
                    },
                  ]}
                />
                {canBack && (
                  <Button variant="ghost" onClick={back}>
                    {t("reporting.back")}
                  </Button>
                )}
                {result.next_cursor && (
                  <Button
                    variant="ghost"
                    onClick={() => setCursor(result.next_cursor)}
                  >
                    {t("reporting.next")}
                  </Button>
                )}
              </>
            ) : (
              <EmptyState>
                {t(
                  // A seat that cannot set a target learns who does. A filter or
                  // a later page that matched nothing says only that.
                  !canCreate && !cursor && status === "active" && !periodFilter
                    ? "reporting.noTargetsYet"
                    : "common.empty",
                )}
              </EmptyState>
            );
          }}
        </QueryGate>
        {editing && context.data && (
          <ReportingTargetDialog
            target={editing === "new" ? undefined : editing}
            context={{
              ...context.data,
              default_scope: selection?.scope ?? context.data.default_scope,
            }}
            onClose={() => setEditing(null)}
          />
        )}
      </PanelBody>
    </Panel>
  );
}

// Sales goals on the year the row is about, with that team member's plan open.
function goalsDials(row: Extract<Row, { kind: "goals" }>) {
  return new Map([
    ["year", identifierNumber(row.roster.year)],
    ["seller", row.seller.owner_id],
  ]);
}

function TargetSubject({
  target,
  pipelines,
}: Readonly<{ target: Target; pipelines: ReturnType<typeof usePipelines> }>) {
  const t = useT();
  const metricName = useMetricNames();
  const { locale } = useLocale();
  const zone = useCompanyZone();
  const pipeline = target.definition.pipeline_id
    ? (pipelines.data?.find(({ id }) => id === target.definition.pipeline_id)
        ?.name ??
      t(pipelines.isPending ? "common.loading" : "reporting.restricted"))
    : t("reporting.allPipelines");
  return (
    <>
      <strong>
        {target.definition.scope.label ?? target.definition.scope.kind}
      </strong>
      <div>{metricName(target.definition.metric)}</div>
      <div className="t-caption">
        {reportingPeriodLabel(target.interval, zone, locale)}
      </div>
      <div className="t-caption">
        {pipeline}
        {target.definition.retired === true
          ? ` · ${t("reporting.retired")}`
          : ""}
      </div>
    </>
  );
}

// The target, and how much of it the members' own targets and sales goals
// already cover.
function TargetAmount({ target }: Readonly<{ target: Target }>) {
  const t = useT();
  const { locale } = useLocale();
  const amount = (value: number) =>
    reportingAmount(
      value,
      target.unit === "count" ? "count" : "money",
      target.unit,
      locale,
    );
  const difference = target.allocation_difference;
  const notes = [
    target.allocated_value != null && target.allocated_value > 0
      ? t("reporting.allocatedCover", {
          amount: amount(target.allocated_value),
        })
      : null,
    difference != null && difference !== 0
      ? `${t(difference < 0 ? "reporting.overallocated" : "reporting.unallocated")}: ${amount(Math.abs(difference))}`
      : null,
  ].filter((note) => note !== null);
  return (
    <>
      <strong>{amount(target.definition.value)}</strong>
      {notes.length > 0 && <div className="t-caption">{notes.join(" · ")}</div>}
    </>
  );
}
