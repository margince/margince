// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type InfiniteData,
  type QueryKey,
  type UseInfiniteQueryResult,
  useInfiniteQuery,
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
import { formatAnalyticsMoney, formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { usePipelines } from "../../lib/pipelines.queries";
import { unwrap } from "../../lib/problem";
import { routeHash } from "../../lib/router";
import type { SalesProgress } from "./progress.queries";

type Lead = components["schemas"]["Lead"];
type Deal = components["schemas"]["Deal"];
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

// The owner filter the list routes take for the population being read.
function ownerFilter(progress: SalesProgress) {
  const id = progress.scope.id;
  return progress.scope.kind === "team"
    ? { owner_team_id: id }
    : { owner_id: id };
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
          onChange={setTab}
        />
      }
    >
      <PanelBody>
        {tab === "leads" ? (
          <OpenLeads progress={progress} />
        ) : (
          <OpenDeals progress={progress} />
        )}
      </PanelBody>
    </Panel>
  );
}

function ownerNames(progress: SalesProgress): Map<string, string> {
  return new Map(
    progress.members.map((member) => [member.owner_id, member.name]),
  );
}

function OpenLeads({ progress }: Readonly<{ progress: SalesProgress }>) {
  const t = useT();
  const { locale } = useLocale();
  const [status, setStatus] = useState<OpenLeadStatus>("new");
  const owners = ownerNames(progress);
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
  const columns: DataTableColumn<Lead>[] = [
    {
      key: "lead",
      header: t("progress.work.lead"),
      grow: true,
      fold: "title",
      render: (lead) => (
        <AppLink href={routeHash({ screen: "leads", id: lead.id })}>
          {lead.full_name || lead.company_name || t("progress.work.unnamed")}
        </AppLink>
      ),
    },
    {
      key: "status",
      header: t("progress.work.status"),
      render: (lead) => <Badge>{t(LEAD_STATUS_LABEL[lead.status])}</Badge>,
    },
    {
      key: "next",
      header: t("progress.work.nextTask"),
      render: (lead) =>
        lead.next_task_subject
          ? `${lead.next_task_subject}${lead.next_task_due_at ? ` · ${formatDate(lead.next_task_due_at, locale, progress.timezone)}` : ""}`
          : t("progress.work.noNextTask"),
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
        onChange={setStatus}
      />
      <WorkPages
        query={query}
        columns={columns}
        label={t("progress.work.leads")}
      />
    </div>
  );
}

function OpenDeals({ progress }: Readonly<{ progress: SalesProgress }>) {
  const t = useT();
  const { locale } = useLocale();
  const owners = ownerNames(progress);
  const pipelines = usePipelines();
  const stages = new Map(
    (pipelines.data ?? []).flatMap((pipeline) =>
      (pipeline.stages ?? []).map((stage) => [stage.id, stage.name] as const),
    ),
  );
  const query = useInfiniteQuery<
    Paged<Deal>,
    Error,
    InfiniteData<Paged<Deal>>,
    QueryKey,
    string | undefined
  >({
    queryKey: ["progress-work", "deals", progress.scope],
    initialPageParam: undefined,
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
    queryFn: async ({ pageParam, signal }) =>
      unwrap(
        await api.GET("/deals", {
          params: {
            query: {
              ...ownerFilter(progress),
              status: "open",
              limit: PAGE,
              cursor: pageParam,
            },
          },
          signal,
        }),
      ),
  });
  const columns: DataTableColumn<Deal>[] = [
    {
      key: "deal",
      header: t("progress.work.deal"),
      grow: true,
      fold: "title",
      render: (deal) => (
        <AppLink href={routeHash({ screen: "deals", id: deal.id })}>
          {deal.name}
        </AppLink>
      ),
    },
    {
      key: "stage",
      header: t("progress.work.stage"),
      render: (deal) =>
        deal.stage_id ? (stages.get(deal.stage_id) ?? "") : "",
    },
    {
      key: "amount",
      header: t("progress.work.amount"),
      align: "end",
      render: (deal) =>
        deal.amount_minor != null && deal.currency
          ? formatAnalyticsMoney(deal.amount_minor, deal.currency, locale)
          : t("progress.work.noAmount"),
    },
    {
      key: "close",
      header: t("progress.work.expectedClose"),
      render: (deal) =>
        deal.expected_close_date
          ? formatDate(`${deal.expected_close_date}T12:00:00Z`, locale, "UTC")
          : t("progress.work.noClose"),
    },
  ];
  if (progress.scope.kind === "team")
    columns.push({
      key: "owner",
      header: t("progress.work.owner"),
      render: (deal) =>
        deal.owner_id ? (owners.get(deal.owner_id) ?? "") : "",
    });
  return (
    <WorkPages
      query={query}
      columns={columns}
      label={t("progress.work.deals")}
    />
  );
}

type Paged<Row> = Readonly<{
  data: Row[];
  page: components["schemas"]["PageInfo"];
}>;

function WorkPages<Row extends { id: string }>({
  query,
  columns,
  label,
}: Readonly<{
  query: UseInfiniteQueryResult<InfiniteData<Paged<Row>>>;
  columns: DataTableColumn<Row>[];
  label: string;
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
              rowKey={(row) => row.id}
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
