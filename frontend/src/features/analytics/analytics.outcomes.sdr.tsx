// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Badge, Button, StatCard } from "../../design-system/atoms";
import {
  Panel,
  PanelBody,
  PanelIntro,
  PanelRow,
} from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { StatStrip } from "../../design-system/statstrip";
import { SurfaceState } from "../../design-system/surfacestate";
import {
  formatDateAbbrev,
  formatDuration,
  formatNumber,
} from "../../format/format";
import { type Locale, useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { DoNext } from "./analytics.overview.donext";
import {
  type ReportingEvaluation,
  type ReportingSelection,
  reportingCompactAmount,
  reportingQuery,
} from "./reporting.model";
import { useReportingCatalog } from "./reporting.queries";

type SdrSummary = components["schemas"]["SdrSummary"];
type WaitingHandoff = components["schemas"]["SdrWaitingHandoff"];

const LEAD_READINGS = [
  "leads_promoted",
  "lead_first_response",
  "leads_awaiting_response",
] as const;

/**
 * An SDR's own work beside their results: their leads, the handoffs that wait
 * on somebody else, and what waits on them. Every figure is the server's; this
 * file only lays them out.
 */
export function MySdrWork({
  self,
  locale,
}: Readonly<{ self: string; locale: Locale }>) {
  const t = useT();
  const readsLeads = useCan("lead", "read");
  const summary = useSdrSummary(self, readsLeads);
  return (
    <>
      {readsLeads && (
        <MyLeads self={self} locale={locale} summary={summary.data} />
      )}
      {/* Which workflow applies is the summary's answer, so a failed read
          says so instead of reading as a workspace with no handoffs. */}
      {summary.isError && (
        <Panel title={t("analytics.sdrWaiting")}>
          <PanelBody>
            <SurfaceState
              state="failed"
              emptyLabel={t("common.empty")}
              loadingLabel={t("analytics.sdrWaiting")}
              detail={{ onRetry: () => void summary.refetch() }}
            >
              {null}
            </SurfaceState>
          </PanelBody>
        </Panel>
      )}
      {summary.data?.handoffs && (
        <WaitingOnOthers
          self={self}
          locale={locale}
          summary={summary.data}
          totals={summary.data.handoffs}
        />
      )}
      <DoNext />
    </>
  );
}

/** The seat's own handoff and meeting counts, under the key both views share. */
export function useSdrSummary(self: string, enabled: boolean) {
  return useQuery({
    queryKey: ["sdr-summary", self],
    enabled,
    queryFn: async ({ signal }): Promise<SdrSummary> =>
      unwrap(
        await api.GET("/analytics/sdr-summary", {
          params: { query: { scope_kind: "owner", scope_id: self } },
          signal,
        }),
      ),
  });
}

function MyLeads({
  self,
  locale,
  summary,
}: Readonly<{ self: string; locale: Locale; summary?: SdrSummary }>) {
  const t = useT();
  const readsReports = useCan("report_definition", "read");
  const catalog = useReportingCatalog(readsReports);
  const evaluation = useQuery({
    queryKey: ["reporting-evaluation", "my-leads", self],
    enabled: readsReports,
    queryFn: async ({ signal }): Promise<ReportingEvaluation> => {
      const selection: ReportingSelection = {
        scope: { kind: "owner", id: self },
        period: "this_month",
        target_basis: "month",
        close_window: "all_open",
        metrics: [...LEAD_READINGS],
        blocks: [],
      };
      return unwrap(
        await api.GET("/analytics/evaluate", {
          params: { query: reportingQuery(selection) },
          signal,
        }),
      );
    },
  });
  if (!readsReports) return null;
  // The catalog's own words for "Leads qualified", so the screen never
  // restates a definition the server may change.
  const qualifiedMeans = catalog.data?.metrics.find(
    (metric) => metric.id === "leads_promoted",
  )?.definition;
  const upcoming =
    summary?.workflow === "qualification"
      ? summary.upcoming_meetings
      : undefined;
  return (
    <Panel title={t("analytics.sdrLeads")}>
      <PanelBody>
        {qualifiedMeans && <PanelIntro>{qualifiedMeans}</PanelIntro>}
        <QueryGate query={evaluation} pendingLabel={t("analytics.sdrLeads")}>
          {(result) => (
            <StatStrip>
              {LEAD_READINGS.map((id) => (
                <StatCard
                  key={id}
                  narrow="row"
                  label={t(`analytics.sdrReading.${id}`)}
                  value={leadReading(result, id, t, locale)}
                />
              ))}
              {upcoming != null && summary?.upcoming_window && (
                <StatCard
                  narrow="row"
                  label={t("analytics.prospect.upcoming")}
                  value={formatNumber(upcoming, locale)}
                  detail={upcomingThrough(summary.upcoming_window, t, locale)}
                />
              )}
            </StatStrip>
          )}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

/**
 * The window a booked-meetings count covers, in the words every view that
 * shows the count uses.
 */
export function upcomingThrough(
  window: NonNullable<SdrSummary["upcoming_window"]>,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  return t("analytics.prospect.upcomingThrough", {
    date: formatDateAbbrev(
      new Date(Date.parse(window.end_at) - 1).toISOString(),
      locale,
      window.timezone,
    ),
  });
}

function leadReading(
  result: ReportingEvaluation,
  id: (typeof LEAD_READINGS)[number],
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  const metric = result.metrics.find((each) => each.id === id);
  if (!metric) return t("analytics.readingUnavailable");
  if (metric.value == null)
    return t(`reporting.status.${metric.coverage.status}`);
  if (id === "leads_promoted" && metric.denominator != null) {
    return t("analytics.sdrQualifiedOf", {
      qualified: formatNumber(metric.value, locale),
      arrived: formatNumber(metric.denominator, locale),
    });
  }
  return reportingCompactAmount(
    metric.value,
    metric.unit,
    result.context.currency,
    locale,
  );
}

export function WaitingOnOthers({
  self,
  locale,
  summary,
  totals,
}: Readonly<{
  self: string;
  locale: Locale;
  summary: SdrSummary;
  totals: NonNullable<SdrSummary["handoffs"]>;
}>) {
  const t = useT();
  const asOf = Date.parse(summary.as_of);
  const rows = useHandoffPages(
    { kind: "owner", id: self },
    "waiting",
    totals.waiting > 0,
  );
  const loaded = rows.data?.pages.flatMap((page) => page.rows) ?? [];
  return (
    <Panel title={t("analytics.sdrWaiting")}>
      <PanelBody>
        <PanelIntro>{t("analytics.sdrWaitingIntro")}</PanelIntro>
        <StatStrip>
          <StatCard
            narrow="row"
            label={t("analytics.sdrWaitingCount")}
            value={formatNumber(totals.waiting, locale)}
            detail={
              totals.oldest_waiting_since
                ? t("analytics.sdrLongestWait", {
                    age: formatDuration(
                      asOf - Date.parse(totals.oldest_waiting_since),
                      locale,
                    ),
                  })
                : undefined
            }
          />
          <StatCard
            narrow="row"
            label={t("analytics.sdrBlocked")}
            value={formatNumber(totals.blocked, locale)}
            tone={totals.blocked > 0 ? "warning" : undefined}
            detail={
              totals.blocked > 0 ? t("analytics.sdrBlockedDetail") : undefined
            }
          />
          <StatCard
            narrow="row"
            label={t("analytics.sdrQueued")}
            value={formatNumber(totals.unassigned, locale)}
          />
          <StatCard
            narrow="row"
            label={t("analytics.sdrReturned")}
            value={formatNumber(totals.returned, locale)}
            detail={
              totals.returned > 0 ? t("analytics.sdrReturnedDetail") : undefined
            }
          />
        </StatStrip>
      </PanelBody>
      {totals.waiting > 0 && (
        <QueryGate query={rows} pendingLabel={t("analytics.sdrWaiting")}>
          {() => (
            <>
              {loaded.map((row) => (
                <WaitingRow
                  key={row.id}
                  row={row}
                  asOf={asOf}
                  locale={locale}
                />
              ))}
              <MoreRows rows={rows} />
            </>
          )}
        </QueryGate>
      )}
      {totals.returned > 0 && <ReturnedHandoffs self={self} locale={locale} />}
    </Panel>
  );
}

export function WaitingRow({
  row,
  asOf,
  locale,
}: Readonly<{ row: WaitingHandoff; asOf: number; locale: Locale }>) {
  const t = useT();
  const age = formatDuration(asOf - Date.parse(row.waiting_since), locale);
  const waitingOn = row.recipient
    ? row.recipient.active
      ? row.recipient.label
      : t("analytics.sdrRecipientInactive", { name: row.recipient.label })
    : t("analytics.sdrSharedQueue");
  const facts = [
    t(
      row.resubmitted
        ? "analytics.sdrWaitingSinceResubmit"
        : "analytics.sdrWaitingFor",
      {
        age,
      },
    ),
    waitingOn,
  ];
  return (
    <HandoffRow row={row} facts={facts}>
      {row.recipient && !row.recipient.active && (
        <Badge tone="warning">{t("analytics.prospect.blocked")}</Badge>
      )}
      {!row.recipient && <Badge>{t("analytics.prospect.unassigned")}</Badge>}
    </HandoffRow>
  );
}

// A handoff sent back: it waits on the submitter, aged from its return, with
// the reason the recipient gave. The prospect opens to be worked again.
function ReturnedRow({
  row,
  asOf,
  locale,
}: Readonly<{ row: WaitingHandoff; asOf: number; locale: Locale }>) {
  const t = useT();
  const facts = [
    t("analytics.prospect.returnedAgo", {
      age: formatDuration(asOf - Date.parse(row.waiting_since), locale),
    }),
    row.reason ?? t("analytics.prospect.noReason"),
  ];
  return (
    <HandoffRow row={row} facts={facts}>
      <Badge>{t("analytics.prospect.returned")}</Badge>
    </HandoffRow>
  );
}

function HandoffRow({
  row,
  facts,
  children,
}: Readonly<{ row: WaitingHandoff; facts: string[]; children: ReactNode }>) {
  const t = useT();
  return (
    <PanelRow className="analytics-attention-row">
      <div className="analytics-attention-text">
        <p className="analytics-attention-title">
          {row.subject ? (
            <EntityRef
              kind={row.subject.type}
              id={row.subject.id}
              name={row.subject.label}
            />
          ) : (
            t("analytics.sdrRestrictedProspect")
          )}{" "}
          {children}
        </p>
        <p className="t-caption">{facts.join(" · ")}</p>
      </div>
    </PanelRow>
  );
}

/** The handoffs sent back to the reader, one page at a time. */
function ReturnedHandoffs({
  self,
  locale,
}: Readonly<{ self: string; locale: Locale }>) {
  const t = useT();
  const rows = useHandoffPages({ kind: "owner", id: self }, "returned", true);
  const loaded = rows.data?.pages.flatMap((page) => page.rows) ?? [];
  const asOf = Date.parse(rows.data?.pages[0]?.as_of ?? "");
  return (
    <QueryGate query={rows} pendingLabel={t("analytics.sdrReturned")}>
      {() => (
        <>
          {loaded.map((row) => (
            <ReturnedRow key={row.id} row={row} asOf={asOf} locale={locale} />
          ))}
          <MoreRows rows={rows} />
        </>
      )}
    </QueryGate>
  );
}

/**
 * Whose open handoffs to list: one seat, a team, or the whole company. No
 * kind is the seat's own default population, which My teams is.
 */
export type HandoffScope = Readonly<{
  kind?: "owner" | "team" | "workspace";
  id?: string;
}>;

/** The open handoffs of a scope in one state, one page at a time. */
export function useHandoffPages(
  scope: HandoffScope,
  state: "waiting" | "returned",
  enabled: boolean,
) {
  return useInfiniteQuery({
    queryKey: [
      "sdr-summary",
      scope.id ?? scope.kind ?? "default",
      "handoffs",
      state,
    ],
    enabled,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      unwrap(
        await api.GET("/analytics/sdr-summary/handoffs", {
          params: {
            query: {
              ...(scope.kind ? { scope_kind: scope.kind } : {}),
              ...(scope.id ? { scope_id: scope.id } : {}),
              state,
              ...(pageParam ? { cursor: pageParam } : {}),
            },
          },
          signal,
        }),
      ),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

export function MoreRows({
  rows,
}: Readonly<{ rows: ReturnType<typeof useHandoffPages> }>) {
  const t = useT();
  if (!rows.hasNextPage) return null;
  return (
    <PanelRow>
      <Button
        onClick={() => void rows.fetchNextPage()}
        disabled={rows.isFetchingNextPage}
      >
        {t("analytics.sdrShowMore")}
      </Button>
    </PanelRow>
  );
}
