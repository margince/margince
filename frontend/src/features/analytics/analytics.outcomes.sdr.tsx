// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Button, StatCard } from "../../design-system/atoms";
import {
  Panel,
  PanelBody,
  PanelIntro,
  PanelRow,
} from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { StatStrip } from "../../design-system/statstrip";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatDuration, formatNumber } from "../../format/format";
import { isMessageKey, type Locale, useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { isEntityKind } from "../../lib/entity";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { listReadState } from "../../lib/worklist.listread";
import { worklistDayOptions } from "../../lib/worklistday";
import {
  type ReportingEvaluation,
  type ReportingSelection,
  reportingCompactAmount,
  reportingQuery,
} from "./reporting.model";
import { useReportingCatalog } from "./reporting.queries";

type SdrSummary = components["schemas"]["SdrSummary"];
type WaitingHandoff = components["schemas"]["SdrWaitingHandoff"];
type WorklistItem = components["schemas"]["WorklistItem"];

// How many ranked items the work panel names, in the Worklist's own order.
const NAMED_WORK = 5;

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
  const summary = useQuery({
    queryKey: ["sdr-summary", self],
    enabled: readsLeads,
    queryFn: async ({ signal }): Promise<SdrSummary> =>
      unwrap(
        await api.GET("/analytics/sdr-summary", {
          params: { query: { scope_kind: "owner", scope_id: self } },
          signal,
        }),
      ),
  });
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
      <NeedsYourAction locale={locale} />
    </>
  );
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
              {upcoming != null && (
                <StatCard
                  narrow="row"
                  label={t("analytics.sdrUpcomingMeetings")}
                  value={formatNumber(upcoming, locale)}
                  detail={t("analytics.sdrUpcomingMeetingsDetail")}
                />
              )}
            </StatStrip>
          )}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
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

function WaitingOnOthers({
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
  const rows = useInfiniteQuery({
    queryKey: ["sdr-summary", self, "handoffs"],
    enabled: totals.waiting > 0,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      unwrap(
        await api.GET("/analytics/sdr-summary/handoffs", {
          params: {
            query: {
              scope_kind: "owner",
              scope_id: self,
              ...(pageParam ? { cursor: pageParam } : {}),
            },
          },
          signal,
        }),
      ),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
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
              {rows.hasNextPage && (
                <PanelRow>
                  <Button
                    onClick={() => void rows.fetchNextPage()}
                    disabled={rows.isFetchingNextPage}
                  >
                    {t("analytics.sdrShowMore")}
                  </Button>
                </PanelRow>
              )}
            </>
          )}
        </QueryGate>
      )}
    </Panel>
  );
}

function WaitingRow({
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
    <PanelRow className="analytics-attention-row">
      <div className="analytics-attention-text">
        {row.subject ? (
          <EntityRef
            kind={row.subject.type}
            id={row.subject.id}
            name={row.subject.label}
          />
        ) : (
          <p>{t("analytics.sdrRestrictedProspect")}</p>
        )}
        <p className="t-caption">{facts.join(" · ")}</p>
      </div>
    </PanelRow>
  );
}

// The first page of the reader's own day, under the key Home reads it by.
// The two screens then share one request and one answer.
function NeedsYourAction({ locale }: Readonly<{ locale: Locale }>) {
  const t = useT();
  const day = useInfiniteQuery(worklistDayOptions("mine", "all"));
  const page = day.data?.pages[0];
  const items = page?.queue?.slice(0, NAMED_WORK) ?? [];
  // A source that failed or was withheld means the list may be short, so an
  // empty answer is never read as a clear day.
  const incomplete = (page?.sources_unavailable?.length ?? 0) > 0;
  const read = listReadState(day, page?.queue);
  const state = read === "empty" && incomplete ? "ready" : read;
  return (
    <Panel
      title={t("analytics.sdrNeedsYou")}
      actions={
        <Button onClick={() => navigate({ screen: "worklist" })}>
          {t("analytics.sdrOpenWorklist")}
        </Button>
      }
    >
      <PanelBody>
        <SurfaceState
          state={state}
          emptyLabel={t("analytics.sdrNothingNeedsYou")}
          loadingLabel={t("analytics.sdrNeedsYou")}
          detail={{ onRetry: () => void day.refetch() }}
        >
          {page?.summary && (
            <PanelIntro>
              {t("analytics.sdrWorkCount", {
                total: formatNumber(page.summary.total, locale),
                urgent: formatNumber(page.summary.urgent, locale),
              })}
            </PanelIntro>
          )}
          {incomplete && (
            <p className="t-caption">{t("analytics.sdrWorkIncomplete")}</p>
          )}
        </SurfaceState>
      </PanelBody>
      {state === "ready" &&
        items.map((item) => (
          <WorkRow key={`${item.source}-${item.id}`} item={item} />
        ))}
    </Panel>
  );
}

function WorkRow({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const category = `worklist.category.${item.category}`;
  const title =
    item.title ||
    item.subject?.label ||
    (isMessageKey(category) ? t(category) : t("worklist.untitled.generic"));
  return (
    <PanelRow className="analytics-attention-row">
      <div className="analytics-attention-text">
        <p className="analytics-attention-title">{title}</p>
        {item.subject && isEntityKind(item.subject.type) && (
          <EntityRef
            kind={item.subject.type}
            id={item.subject.id}
            name={item.subject.label}
          />
        )}
      </div>
    </PanelRow>
  );
}
