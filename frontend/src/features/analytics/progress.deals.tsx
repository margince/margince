// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type InfiniteData,
  type QueryKey,
  useInfiniteQuery,
} from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useEntityName } from "../../components/entityref.queries";
import { AppLink } from "../../design-system/applink";
import { Badge } from "../../design-system/atoms";
import type { DataTableColumn } from "../../design-system/datatable";
import { calendarDay, todayIn } from "../../format/calendarday";
import {
  formatAnalyticsMoney,
  formatDate,
  formatDateTime,
} from "../../format/format";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { valueFor } from "../../lib/labelfor";
import { useMemberNames } from "../../lib/membernames";
import { usePipelines } from "../../lib/pipelines.queries";
import { unwrap } from "../../lib/problem";
import { routeHash } from "../../lib/router";
import type { SalesProgress } from "./progress.queries";

export type WorkDeal = components["schemas"]["SalesProgressDeal"];
type SaleType = NonNullable<WorkDeal["sale_type"]>;
type Paged<Row> = Readonly<{
  data: Row[];
  page: components["schemas"]["PageInfo"];
}>;
const PAGE = 25;

const SALE_TYPE: Record<
  SaleType,
  { label: MessageKey; tone: "success" | "info" | "accent" | "warning" }
> = {
  product: { label: "progress.work.saleProduct", tone: "success" },
  service: { label: "progress.work.saleService", tone: "info" },
  product_and_service: { label: "progress.work.saleBoth", tone: "accent" },
  partly_classified: { label: "progress.work.salePartly", tone: "warning" },
  unclassified: { label: "progress.work.saleNone", tone: "warning" },
};

/** The open deals behind the reading, with what each sells and its next task. */
export function useWorkDeals(progress: SalesProgress) {
  const { kind, id } = progress.scope;
  return useInfiniteQuery<
    Paged<WorkDeal>,
    Error,
    InfiniteData<Paged<WorkDeal>>,
    QueryKey,
    string | undefined
  >({
    queryKey: ["progress-work", "deals", kind, id],
    initialPageParam: undefined,
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
    queryFn: async ({ pageParam, signal }) =>
      unwrap(
        await api.GET("/analytics/progress/deals", {
          params: {
            query: {
              scope_kind: kind === "team" ? "team" : "owner",
              scope_id: id,
              limit: PAGE,
              cursor: pageParam,
            },
          },
          signal,
        }),
      ),
  });
}

type CloseFlag = "overdue" | "outside" | "none";

// What a deal's expected close says against today and the period being read.
// The horizon's end is the next period's first instant, so it is outside.
function closeFlag(
  date: string | null | undefined,
  today: string,
  progress: SalesProgress,
): CloseFlag | undefined {
  if (!date) return "none";
  if (date < today) return "overdue";
  const zone = progress.timezone;
  const start = calendarDay(new Date(progress.horizon.start_at), zone);
  const end = calendarDay(new Date(progress.horizon.end_at), zone);
  return date < start || date >= end ? "outside" : undefined;
}

const FLAG = {
  overdue: { label: "progress.work.overdue", tone: "danger" },
  outside: { label: "progress.work.outside", tone: "default" },
  none: { label: "progress.work.noClose", tone: "warning" },
} as const satisfies Record<CloseFlag, { label: MessageKey; tone: string }>;

function plannedText(deal: WorkDeal["deal"], t: Translator, locale: Locale) {
  if (deal.masked_fields?.includes("amount_minor"))
    return t("progress.withheld");
  if (deal.amount_minor == null || !deal.currency)
    return t("progress.work.noAmount");
  return formatAnalyticsMoney(deal.amount_minor, deal.currency, locale);
}

/** The columns of the open-deals list; the owner column only for a team. */
export function useDealColumns(
  progress: SalesProgress,
  rows: readonly WorkDeal[],
): DataTableColumn<WorkDeal>[] {
  const t = useT();
  const { locale } = useLocale();
  const [today] = useState(() => todayIn(progress.timezone));
  const pipelines = usePipelines();
  const stages = new Map(
    (pipelines.data ?? []).flatMap((pipeline) =>
      (pipeline.stages ?? []).map((stage) => [stage.id, stage.name] as const),
    ),
  );
  const names = useMemberNames(
    rows
      .flatMap((row) => [
        row.deal.owner_id ?? "",
        row.next_task?.assignee_id ?? "",
      ])
      .filter(Boolean),
  );
  const columns: DataTableColumn<WorkDeal>[] = [
    {
      key: "deal",
      header: t("progress.work.deal"),
      grow: true,
      fold: "title",
      render: (row) => (
        <DealCell
          deal={row.deal}
          stage={row.deal.stage_id ? stages.get(row.deal.stage_id) : undefined}
        />
      ),
    },
    {
      key: "planned",
      header: t("progress.work.amount"),
      align: "end",
      render: (row) => (
        <span className="progress-work-stack progress-work-end">
          <span className="t-num">{plannedText(row.deal, t, locale)}</span>
          <SaleTypeBadge saleType={row.sale_type} />
        </span>
      ),
    },
    {
      key: "close",
      header: t("progress.work.expectedClose"),
      render: (row) => {
        const date = row.deal.expected_close_date;
        const flag = closeFlag(date, today, progress);
        return (
          <span className="progress-work-stack">
            {date && (
              <span className="t-num">
                {formatDate(date, locale, progress.timezone)}
              </span>
            )}
            {flag && (
              <Badge tone={FLAG[flag].tone}>{t(FLAG[flag].label)}</Badge>
            )}
          </span>
        );
      },
    },
    {
      key: "next",
      header: t("progress.work.nextTask"),
      render: (row) => {
        const task = row.next_task;
        if (!task)
          return (
            <span className="progress-work-none">
              {t("progress.work.noNextTask")}
            </span>
          );
        const other =
          task.assignee_id && task.assignee_id !== row.deal.owner_id
            ? names.get(task.assignee_id)
            : undefined;
        const due = task.due_at
          ? formatDateTime(task.due_at, locale, progress.timezone)
          : undefined;
        return (
          <span className="progress-work-stack">
            <span>{task.subject}</span>
            {(due || other) && (
              <span className="t-caption">
                {[due, other].filter(Boolean).join(" · ")}
              </span>
            )}
          </span>
        );
      },
    },
  ];
  if (progress.scope.kind === "team")
    columns.push({
      key: "owner",
      header: t("progress.work.owner"),
      render: (row) =>
        row.deal.owner_id ? (names.get(row.deal.owner_id) ?? "") : "",
    });
  return columns;
}

// What the deal sells; a kind this build does not know shows nothing.
function SaleTypeBadge({ saleType }: Readonly<{ saleType?: SaleType }>) {
  const t = useT();
  const kind = saleType && valueFor(SALE_TYPE, saleType, undefined);
  return kind ? <Badge tone={kind.tone}>{t(kind.label)}</Badge> : null;
}

// The deal's name over where it stands: its company and its stage.
function DealCell({
  deal,
  stage,
}: Readonly<{ deal: WorkDeal["deal"]; stage: string | undefined }>) {
  const company = useEntityName("company", deal.company_id).name;
  const where = [company, stage].filter(Boolean).join(" · ");
  return (
    <span className="progress-work-stack">
      <AppLink href={routeHash({ screen: "deals", id: deal.id })}>
        {deal.name}
      </AppLink>
      {where && <span className="t-caption">{where}</span>}
    </span>
  );
}
