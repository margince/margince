// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./progress.css";
import { useState } from "react";
import { Badge, Button, Field } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { DataTable } from "../../design-system/datatable";
import { FilterBar } from "../../design-system/filterbar";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { formatAnalyticsMoney, formatDateTime } from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import { navigate } from "../../lib/router";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { useViewerZone } from "../../lib/viewerzone";
import { GoalEditor } from "./goals.editor";
import {
  monthName,
  type SalesGoalPlan,
  type SalesGoalRoster,
  useSalesGoalPlan,
  useSalesGoalRoster,
} from "./progress.queries";

type PlanState = SalesGoalPlan["state"];

const STATE_TONE: Record<
  PlanState,
  "default" | "success" | "warning" | "info"
> = {
  no_goal: "default",
  draft: "info",
  published: "success",
  published_with_draft: "info",
  withdrawn: "default",
  changed_outside_plan: "warning",
};

export function PlanStateBadge({ state }: Readonly<{ state: PlanState }>) {
  const t = useT();
  return <Badge tone={STATE_TONE[state]}>{t(`goals.state.${state}`)}</Badge>;
}

/**
 * Sales goals: one seller's twelve monthly goals for a calendar year. A rep
 * reads their own; a manager drafts, publishes and withdraws them.
 */
export function SalesGoalsScreen() {
  const t = useT();
  const [params] = useUrlParams();
  const [now] = useState(() => new Date());
  const year = Number(params.get("year")) || now.getFullYear();
  const roster = useSalesGoalRoster(year);
  const sellers = roster.data?.data ?? [];
  const seller =
    sellers.find((each) => each.owner_id === params.get("seller")) ??
    sellers[0];
  const years = Array.from(
    { length: 5 },
    (_, index) => now.getFullYear() + 1 - index,
  );
  return (
    <div className="wrap progress-page">
      <FilterBar
        label={t("goals.controls")}
        actions={
          <Button
            variant="ghost"
            onClick={() =>
              navigate(
                { screen: "progress" },
                new Map([
                  ["period", "year"],
                  ["year", String(year)],
                ]),
              )
            }
          >
            {t("goals.viewProgress")}
          </Button>
        }
      >
        <Field label={t("progress.year")}>
          {(field) => (
            <Select
              {...field}
              options={years.map((each) => ({
                value: String(each),
                label: String(each),
              }))}
              value={String(year)}
              onChange={(next) => replaceDial("year", next)}
            />
          )}
        </Field>
        {sellers.length > 1 && seller && (
          <Field label={t("goals.seller")}>
            {(field) => (
              <Select
                {...field}
                options={sellers.map((each) => ({
                  value: each.owner_id,
                  label: each.name,
                }))}
                value={seller.owner_id}
                onChange={(next) => replaceDial("seller", next)}
              />
            )}
          </Field>
        )}
      </FilterBar>
      <QueryGate query={roster} pendingLabel={t("goals.reading")}>
        {(answer) => (
          <div className="progress-body">
            {seller && <SellerPlan owner={seller.owner_id} year={year} />}
            {answer.data.length > 1 && <Roster roster={answer} />}
          </div>
        )}
      </QueryGate>
    </div>
  );
}

function SellerPlan({
  owner,
  year,
}: Readonly<{ owner: string; year: number }>) {
  const t = useT();
  const plan = useSalesGoalPlan(owner, year);
  return (
    <QueryGate query={plan} pendingLabel={t("goals.reading")}>
      {(answer) => (
        <>
          <Panel
            title={t("goals.planTitle", {
              name: answer.owner_name,
              year: String(answer.year),
            })}
            titleAction={<PlanStateBadge state={answer.state} />}
          >
            <PanelBody>
              {answer.capabilities.edit ? (
                <GoalEditor
                  key={`${answer.owner_id}:${answer.version}`}
                  plan={answer}
                />
              ) : (
                <ReadOnlyPlan plan={answer} />
              )}
            </PanelBody>
          </Panel>
          {answer.history.length > 0 && <PlanHistory plan={answer} />}
        </>
      )}
    </QueryGate>
  );
}

function ReadOnlyPlan({ plan }: Readonly<{ plan: SalesGoalPlan }>) {
  const t = useT();
  return (
    <div className="goal-plan">
      <Callout tone="info" title={t("goals.readOnly")} />
      <MonthsTable months={plan.live_months} currency={plan.currency} />
    </div>
  );
}

/** The twelve months as published, and their sum. */
export function MonthsTable({
  months,
  currency,
}: Readonly<{ months: readonly (number | null)[]; currency: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const total = months.every((month) => month !== null)
    ? months.reduce<number>((sum, month) => sum + (month ?? 0), 0)
    : null;
  const rows = [
    ...months.map((amount, index) => ({
      key: String(index + 1),
      label: monthName(index + 1, locale),
      amount,
    })),
    { key: "total", label: t("goals.yearTotal"), amount: total },
  ];
  return (
    <DataTable
      label={t("goals.months")}
      rows={rows}
      rowKey={(row) => row.key}
      columns={[
        {
          key: "month",
          header: t("progress.month"),
          grow: true,
          render: (row) => row.label,
        },
        {
          key: "goal",
          header: t("progress.goal"),
          align: "end",
          render: (row) =>
            row.amount === null
              ? t("goals.notSet")
              : formatAnalyticsMoney(row.amount, currency, locale),
        },
      ]}
    />
  );
}

function eventLine(
  event: SalesGoalPlan["history"][number],
  t: Translator,
): string {
  return t(`goals.event.${event.kind}`, { name: event.author_name });
}

function PlanHistory({ plan }: Readonly<{ plan: SalesGoalPlan }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  return (
    <Panel title={t("goals.history")}>
      <PanelBody>
        <DataTable
          label={t("goals.history")}
          fold
          rows={[...plan.history].reverse()}
          rowKey={(event) => String(event.revision)}
          columns={[
            {
              key: "what",
              header: t("goals.history.what"),
              fold: "title",
              render: (event) => eventLine(event, t),
            },
            {
              key: "reason",
              header: t("goals.history.reason"),
              grow: true,
              render: (event) => event.reason,
            },
            {
              key: "total",
              header: t("goals.yearTotal"),
              align: "end",
              render: (event) =>
                event.total_minor == null
                  ? ""
                  : formatAnalyticsMoney(
                      event.total_minor,
                      plan.currency,
                      locale,
                    ),
            },
            {
              key: "when",
              header: t("goals.history.when"),
              render: (event) => formatDateTime(event.created_at, locale, zone),
            },
          ]}
        />
      </PanelBody>
    </Panel>
  );
}

function Roster({ roster }: Readonly<{ roster: SalesGoalRoster }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <Panel title={t("goals.roster")}>
      <PanelBody>
        <DataTable
          label={t("goals.roster")}
          fold
          rows={roster.data}
          rowKey={(seller) => seller.owner_id}
          onRowClick={(seller) => replaceDial("seller", seller.owner_id)}
          columns={[
            {
              key: "name",
              header: t("goals.seller"),
              grow: true,
              fold: "title",
              render: (seller) => seller.name,
            },
            {
              key: "state",
              header: t("goals.status"),
              render: (seller) => <PlanStateBadge state={seller.state} />,
            },
            {
              key: "total",
              header: t("goals.yearTotal"),
              align: "end",
              render: (seller) =>
                seller.total_minor == null
                  ? t("goals.notSet")
                  : formatAnalyticsMoney(
                      seller.total_minor,
                      roster.currency,
                      locale,
                    ),
            },
          ]}
        />
      </PanelBody>
    </Panel>
  );
}
