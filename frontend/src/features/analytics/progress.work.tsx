// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type InfiniteData,
  type QueryKey,
  type UseInfiniteQueryResult,
  useInfiniteQuery,
  useQueries,
} from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import {
  Badge,
  Button,
  EmptyState,
  SegmentedControl,
} from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor } from "../../lib/labelfor";
import { useMemberNames } from "../../lib/membernames";
import { unwrap } from "../../lib/problem";
import { routeHash } from "../../lib/router";
import { useDealColumns, useWorkDeals } from "./progress.deals";
import type { SalesProgress } from "./progress.queries";

type Lead = components["schemas"]["Lead"];
type Tab = "leads" | "deals";
type OpenLeadStatus = "new" | "contacted" | "engaged";
const TABS: readonly Tab[] = ["leads", "deals"];
const LEAD_STATUSES: readonly OpenLeadStatus[] = [
  "new",
  "contacted",
  "engaged",
];
const PAGE = 25;
const LEAD_STATUS_LABEL = {
  new: "lead.status.new",
  contacted: "lead.status.contacted",
  engaged: "lead.status.engaged",
  promoted: "lead.statusPromoted",
  disqualified: "lead.statusDisqualified",
} as const satisfies Record<Lead["status"], MessageKey>;

type Paged<Row> = Readonly<{
  data: Row[];
  page: components["schemas"]["PageInfo"];
}>;

// The owner filter the lead list takes for the population being read.
function ownerFilter(progress: SalesProgress) {
  const id = progress.scope.id;
  return progress.scope.kind === "team"
    ? { owner_team_id: id }
    : { owner_id: id };
}

// How many leads sit in each open status. A list that does not count leaves
// its status unknown, never zero.
function useLeadCounts(progress: SalesProgress) {
  const reads = useQueries({
    queries: LEAD_STATUSES.map((status) => ({
      queryKey: ["progress-work", "lead-count", progress.scope, status],
      queryFn: async ({ signal }: { signal: AbortSignal }) =>
        unwrap(
          await api.GET("/leads", {
            params: { query: { ...ownerFilter(progress), status, limit: 1 } },
            signal,
          }),
        ).page.total,
    })),
  });
  const counts: Partial<Record<OpenLeadStatus, number>> = {};
  LEAD_STATUSES.forEach((status, index) => {
    const total = reads[index]?.data;
    if (total != null) counts[status] = total;
  });
  const known = LEAD_STATUSES.every((status) => counts[status] != null);
  const all = known
    ? LEAD_STATUSES.reduce((sum, status) => sum + (counts[status] ?? 0), 0)
    : undefined;
  return { counts, all };
}

/**
 * The open work behind the figures, as it stands now whatever period is read:
 * leads still being worked and deals still open.
 */
export function ProgressWork({
  progress,
}: Readonly<{ progress: SalesProgress }>) {
  const t = useT();
  const [tab, setTab] = useState<Tab>("leads");
  const leads = useLeadCounts(progress);
  const deals = useWorkDeals(progress);
  const dealPages = deals.data?.pages;
  const dealCount =
    dealPages && !deals.hasNextPage
      ? dealPages.reduce((sum, page) => sum + page.data.length, 0)
      : undefined;
  return (
    <Panel
      title={t("progress.work")}
      titleAction={
        <SegmentedControl
          label={t("progress.work")}
          options={TABS}
          value={tab}
          labels={{
            leads: t("progress.work.leads"),
            deals: t("progress.work.deals"),
          }}
          counts={{
            ...(leads.all != null ? { leads: leads.all } : {}),
            ...(dealCount != null ? { deals: dealCount } : {}),
          }}
          onChange={setTab}
        />
      }
    >
      <PanelBody>
        <p className="t-caption progress-work-note">
          {t("progress.work.note")}
        </p>
        {tab === "leads" ? (
          <OpenLeads progress={progress} counts={leads.counts} />
        ) : (
          <OpenDeals progress={progress} query={deals} />
        )}
      </PanelBody>
    </Panel>
  );
}

function OpenLeads({
  progress,
  counts,
}: Readonly<{
  progress: SalesProgress;
  counts: Partial<Record<OpenLeadStatus, number>>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [status, setStatus] = useState<OpenLeadStatus>("new");
  const query = useInfiniteQuery<
    Paged<Lead>,
    Error,
    InfiniteData<Paged<Lead>>,
    QueryKey,
    string | undefined
  >({
    queryKey: ["progress-work", "leads", progress.scope, status],
    initialPageParam: undefined,
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
    queryFn: async ({ pageParam, signal }) =>
      unwrap(
        await api.GET("/leads", {
          params: {
            query: {
              ...ownerFilter(progress),
              status,
              limit: PAGE,
              cursor: pageParam,
            },
          },
          signal,
        }),
      ),
  });
  const rows = query.data?.pages.flatMap((page) => page.data) ?? [];
  const owners = useMemberNames(
    progress.scope.kind === "team"
      ? rows.flatMap((lead) => (lead.owner_id ? [lead.owner_id] : []))
      : [],
  );
  const columns: DataTableColumn<Lead>[] = [
    {
      key: "lead",
      header: t("progress.work.lead"),
      grow: true,
      fold: "title",
      render: (lead) => (
        <span className="progress-work-stack">
          <AppLink href={routeHash({ screen: "leads", id: lead.id })}>
            {lead.full_name || lead.company_name || t("progress.work.unnamed")}
          </AppLink>
          {lead.full_name && lead.company_name && (
            <span className="t-caption">{lead.company_name}</span>
          )}
        </span>
      ),
    },
    {
      key: "status",
      header: t("progress.work.status"),
      render: (lead) => (
        <Badge>{t(labelFor(LEAD_STATUS_LABEL, lead.status))}</Badge>
      ),
    },
    {
      key: "next",
      header: t("progress.work.nextTask"),
      render: (lead) =>
        lead.next_task_subject ? (
          <span className="progress-work-stack">
            <span>{lead.next_task_subject}</span>
            {lead.next_task_due_at && (
              <span className="t-caption">
                {formatDate(lead.next_task_due_at, locale, progress.timezone)}
              </span>
            )}
          </span>
        ) : (
          <span className="progress-work-none">
            {t("progress.work.noNextTask")}
          </span>
        ),
    },
  ];
  if (progress.scope.kind === "team")
    columns.push({
      key: "owner",
      header: t("progress.work.owner"),
      render: (lead) =>
        lead.owner_id ? (owners.get(lead.owner_id) ?? "") : "",
    });
  return (
    <div className="progress-work">
      <SegmentedControl
        label={t("progress.work.status")}
        options={LEAD_STATUSES}
        value={status}
        labels={{
          new: t(LEAD_STATUS_LABEL.new),
          contacted: t(LEAD_STATUS_LABEL.contacted),
          engaged: t(LEAD_STATUS_LABEL.engaged),
        }}
        counts={counts}
        onChange={setStatus}
      />
      <WorkPages
        query={query}
        columns={columns}
        label={t("progress.work.leads")}
        rowKey={(lead) => lead.id}
      />
    </div>
  );
}

function OpenDeals({
  progress,
  query,
}: Readonly<{
  progress: SalesProgress;
  query: ReturnType<typeof useWorkDeals>;
}>) {
  const t = useT();
  const rows = query.data?.pages.flatMap((page) => page.data) ?? [];
  const columns = useDealColumns(progress, rows);
  return (
    <WorkPages
      query={query}
      columns={columns}
      label={t("progress.work.deals")}
      rowKey={(row) => row.deal.id}
    />
  );
}

function WorkPages<Row>({
  query,
  columns,
  label,
  rowKey,
}: Readonly<{
  query: UseInfiniteQueryResult<InfiniteData<Paged<Row>>>;
  columns: DataTableColumn<Row>[];
  label: string;
  rowKey: (row: Row) => string;
}>) {
  const t = useT();
  return (
    <QueryGate query={query} pendingLabel={label}>
      {(answer) => {
        const rows = answer.pages.flatMap((page) => page.data);
        if (rows.length === 0)
          return <EmptyState>{t("progress.work.empty")}</EmptyState>;
        return (
          <>
            <DataTable<Row>
              label={label}
              fold
              rows={rows}
              rowKey={rowKey}
              columns={columns}
            />
            {query.hasNextPage && (
              <div className="card-actions">
                <Button
                  variant="ghost"
                  pending={query.isFetchingNextPage}
                  onClick={() => void query.fetchNextPage()}
                >
                  {t("progress.work.more")}
                </Button>
              </div>
            )}
          </>
        );
      }}
    </QueryGate>
  );
}
