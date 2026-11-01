// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, EmptyState, SegmentedControl } from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import {
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../../design-system/panel";
import {
  formatDayMonth,
  formatDayRange,
  formatNumber,
  formatPercent,
} from "../../format/format";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan } from "../../lib/capability";
import { useHealthScope, useHealthVisible } from "../../lib/healthscope";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { HealthCard } from "./healthcard";

// GET /admin/automatic-work-health: per colleague and kind, the items Margince
// created by itself and the outcome each holds now (compose/automaticwork.go).
// The share and its flag are the server's; this card only draws them.

type Health = components["schemas"]["AutomaticWorkHealth"];
type Colleague = components["schemas"]["AutomaticWorkColleague"];
type Tally = components["schemas"]["AutomaticWorkTally"];
type Counts = components["schemas"]["AutomaticWorkCounts"];
type Day = components["schemas"]["AutomaticWorkDay"];
type Row = Readonly<{ colleague: Colleague; tally: Tally }>;

const WINDOWS = ["7", "30"] as const;
type Window = (typeof WINDOWS)[number];

const KIND_LABEL: Record<Tally["kind"], MessageKey> = {
  promise_task: "automaticWork.kind.promise_task",
  reply_reminder: "automaticWork.kind.reply_reminder",
  approval_request: "automaticWork.kind.approval_request",
};

const OUTCOMES: ReadonlyArray<readonly [keyof Counts, MessageKey]> = [
  ["created", "automaticWork.col.created"],
  ["not_mine", "automaticWork.col.notMine"],
  ["rerouted", "automaticWork.col.rerouted"],
  ["dismissed", "automaticWork.col.dismissed"],
  ["rejected", "automaticWork.col.rejected"],
  ["completed", "automaticWork.col.completed"],
  ["expired", "automaticWork.col.expired"],
  ["open", "automaticWork.col.open"],
];

function countColumns<Item>(
  counts: (item: Item) => Counts,
  t: Translator,
  locale: Locale,
): DataTableColumn<Item>[] {
  return OUTCOMES.map(([key, header]) => ({
    key,
    header: t(header),
    align: "end",
    render: (item: Item) => formatNumber(counts(item)[key], locale),
  }));
}

function colleagueName(colleague: Colleague, t: Translator): string {
  return colleague.status === "active"
    ? colleague.display_name
    : t("automaticWork.inactive", { name: colleague.display_name });
}

function ShareCell({ tally }: Readonly<{ tally: Tally }>) {
  const t = useT();
  const { locale } = useLocale();
  if (tally.not_mine_share === null) {
    return <span>{t("automaticWork.noNotMine")}</span>;
  }
  const share = formatPercent(tally.not_mine_share, locale);
  if (tally.above_threshold) {
    return (
      <Badge tone="warning">{t("automaticWork.needsLook", { share })}</Badge>
    );
  }
  return (
    <span>
      {tally.small_sample ? t("automaticWork.fewItems", { share }) : share}
    </span>
  );
}

function DayTable({ row }: Readonly<{ row: Row }>) {
  const t = useT();
  const zone = useRecordZone();
  const { locale } = useLocale();
  const columns: DataTableColumn<Day>[] = [
    {
      key: "day",
      header: t("automaticWork.col.day"),
      render: (day) => formatDayMonth(day.date, locale, zone),
    },
    ...countColumns<Day>((day) => day.counts, t, locale),
  ];
  return (
    <DataTable
      label={t("automaticWork.days", {
        name: row.colleague.display_name,
        kind: t(KIND_LABEL[row.tally.kind]),
      })}
      columns={columns}
      rows={[...row.tally.by_day]}
      rowKey={(day) => day.date}
    />
  );
}

function workColumns(t: Translator, locale: Locale): DataTableColumn<Row>[] {
  return [
    {
      key: "colleague",
      header: t("automaticWork.col.colleague"),
      render: (row) => colleagueName(row.colleague, t),
    },
    {
      key: "kind",
      header: t("automaticWork.col.kind"),
      render: (row) => t(KIND_LABEL[row.tally.kind]),
    },
    ...countColumns<Row>((row) => row.tally.counts, t, locale),
    {
      key: "share",
      header: t("automaticWork.col.share"),
      align: "end",
      render: (row) => <ShareCell tally={row.tally} />,
    },
  ];
}

function AutomaticWorkBody({ health }: Readonly<{ health: Health }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const rows: Row[] = health.colleagues.flatMap((colleague) =>
    colleague.tallies
      .filter((tally) => tally.counts.created > 0)
      .map((tally) => ({ colleague, tally })),
  );
  const quiet = health.colleagues.filter((colleague) =>
    colleague.tallies.every((tally) => tally.counts.created === 0),
  ).length;
  const waiting = health.colleagues.filter(
    (colleague) => colleague.waiting_not_mine > 0,
  );
  const key = (row: Row) => `${row.colleague.user_id}:${row.tally.kind}`;
  return (
    <>
      {rows.length === 0 ? (
        <PanelBody>
          <EmptyState>{t("automaticWork.none")}</EmptyState>
        </PanelBody>
      ) : (
        <DataTable
          bleed
          stickyFirst
          label={t("settings.automaticWork")}
          columns={workColumns(t, locale)}
          rows={rows}
          rowKey={key}
          detail={{
            header: t("automaticWork.col.day"),
            toggleLabel: (row) =>
              t("automaticWork.days", {
                name: row.colleague.display_name,
                kind: t(KIND_LABEL[row.tally.kind]),
              }),
            render: (row) => <DayTable row={row} />,
          }}
        />
      )}
      <PanelBody>
        <PanelIntro>
          {plural("automaticWork.quiet", quiet, {
            count: formatNumber(quiet, locale),
          })}
        </PanelIntro>
        {health.unattributed > 0 && (
          <PanelIntro>
            {plural("automaticWork.unattributed", health.unattributed, {
              count: formatNumber(health.unattributed, locale),
            })}
          </PanelIntro>
        )}
      </PanelBody>
      {waiting.length > 0 && (
        <>
          <PanelGroupHead title={t("automaticWork.waiting")} level="h3" />
          <PanelBody>
            <PanelIntro>{t("automaticWork.waitingSub")}</PanelIntro>
          </PanelBody>
          <DataTable
            bleed
            label={t("automaticWork.waiting")}
            columns={[
              {
                key: "colleague",
                header: t("automaticWork.col.colleague"),
                render: (colleague) => colleagueName(colleague, t),
              },
              {
                key: "marked",
                header: t("automaticWork.col.waitingNotMine"),
                align: "end",
                render: (colleague) =>
                  formatNumber(colleague.waiting_not_mine, locale),
              },
            ]}
            rows={waiting}
            rowKey={(colleague) => colleague.user_id}
          />
        </>
      )}
    </>
  );
}

const isCount = (value: unknown): value is number =>
  typeof value === "number" && Number.isInteger(value) && value >= 0;

// A missing count would draw as zero, a reading this card must not invent. A
// malformed payload is the card's error state.
function isCounts(counts: Counts | undefined): boolean {
  return (
    typeof counts === "object" &&
    counts !== null &&
    OUTCOMES.every(([key]) => isCount(counts[key]))
  );
}

function isTally(tally: Tally): boolean {
  return (
    Object.hasOwn(KIND_LABEL, tally.kind) &&
    isCounts(tally.counts) &&
    (tally.not_mine_share === null ||
      typeof tally.not_mine_share === "number") &&
    Array.isArray(tally.by_day) &&
    tally.by_day.every(
      (day) => typeof day.date === "string" && isCounts(day.counts),
    )
  );
}

function isAutomaticWorkHealth(data: Health | undefined): data is Health {
  return (
    typeof data === "object" &&
    data !== null &&
    typeof data.generated_at === "string" &&
    typeof data.first_day === "string" &&
    typeof data.last_day === "string" &&
    isCount(data.unattributed) &&
    Array.isArray(data.colleagues) &&
    data.colleagues.every(
      (colleague) =>
        typeof colleague.display_name === "string" &&
        isCount(colleague.waiting_not_mine) &&
        Array.isArray(colleague.tallies) &&
        colleague.tallies.every(isTally),
    )
  );
}

// `job_health:read`, the grant every System health card shares.
export function useAutomaticWorkHealth(days: Window) {
  const canSee = useCan("job_health", "read");
  const scope = useHealthScope();
  const visible = useHealthVisible();
  const query = useQuery({
    queryKey: ["automatic-work-health", scope, days],
    enabled: canSee && visible && scope !== undefined,
    // The window control sits in the report; without the last answer held
    // while the next loads, the control would vanish under the pointer.
    placeholderData: keepPreviousData,
    queryFn: async ({ signal }) => {
      const data = unwrap(
        await api.GET("/admin/automatic-work-health", {
          signal,
          params: { query: { days: Number(days) } },
        }),
      );
      if (!isAutomaticWorkHealth(data)) {
        throw new Error("malformed automatic-work-health response");
      }
      return data;
    },
  });
  return { canSee, query };
}

export function AutomaticWorkHealthCard() {
  const t = useT();
  const zone = useRecordZone();
  const { locale } = useLocale();
  const [days, setDays] = useState<Window>("7");
  const { canSee, query } = useAutomaticWorkHealth(days);

  return (
    <HealthCard
      title={t("settings.automaticWork")}
      sub={t("settings.automaticWorkSub")}
      withheld={t("automaticWork.adminOnly")}
      canSee={canSee}
      query={query}
      lead={(health) => (
        <>
          <SegmentedControl
            label={t("automaticWork.window")}
            options={WINDOWS}
            value={days}
            onChange={setDays}
            labels={{
              "7": t("automaticWork.last7"),
              "30": t("automaticWork.last30"),
            }}
          />
          <PanelIntro>
            {t("automaticWork.howCounted", {
              range: formatDayRange(
                health.first_day,
                health.last_day,
                locale,
                zone,
              ),
              threshold: formatPercent(health.share_threshold, locale),
              sample: formatNumber(health.small_sample_below, locale),
            })}
          </PanelIntro>
        </>
      )}
    >
      {(health) => <AutomaticWorkBody health={health} />}
    </HealthCard>
  );
}
