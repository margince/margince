// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./progress.css";
import { useState } from "react";
import { Badge, Button, Field } from "../../design-system/atoms";
import { Avatar } from "../../design-system/avatar";
import { Callout } from "../../design-system/callout";
import { DataTable } from "../../design-system/datatable";
import { FilterBar } from "../../design-system/filterbar";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import {
  formatAnalyticsMoney,
  formatDateTime,
  identifierNumber,
} from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor, valueFor } from "../../lib/labelfor";
import { navigate } from "../../lib/router";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { useViewerZone } from "../../lib/viewerzone";
import { GoalBars, GoalMonths } from "./goals.bars";
import { GoalEditor } from "./goals.editor";
import {
  type SalesGoalPlan,
  type SalesGoalRoster,
  useSalesGoalPlan,
  useSalesGoalRoster,
} from "./progress.queries";

type PlanState = SalesGoalPlan["state"];

const STATE_TONE: Readonly<
  Record<PlanState, "default" | "success" | "warning" | "info">
> = {
  no_goal: "default",
  draft: "info",
  published: "success",
  published_with_draft: "info",
  withdrawn: "default",
  changed_outside_plan: "warning",
};

const STATE_LABEL: Readonly<Record<PlanState, MessageKey>> = {
  no_goal: "goals.state.no_goal",
  draft: "goals.state.draft",
  published: "goals.state.published",
  published_with_draft: "goals.state.published_with_draft",
  withdrawn: "goals.state.withdrawn",
  changed_outside_plan: "goals.state.changed_outside_plan",
};

export function PlanStateBadge({ state }: Readonly<{ state: PlanState }>) {
  const t = useT();
  return (
    <Badge tone={valueFor(STATE_TONE, state, "default")}>
      {t(labelFor(STATE_LABEL, state))}
    </Badge>
  );
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
            onClick={() => {
              const dials = new Map([
                ["period", "year"],
                ["year", identifierNumber(year)],
              ]);
              if (seller) dials.set("scope", `owner:${seller.owner_id}`);
              navigate({ screen: "progress" }, dials);
            }}
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
                label: identifierNumber(each),
              }))}
              value={identifierNumber(year)}
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
          <div className={answer.data.length > 1 ? "goals-board" : undefined}>
            {answer.data.length > 1 && <Roster roster={answer} />}
            <div className="goals-board-plan">
              {seller && <SellerPlan owner={seller.owner_id} year={year} />}
            </div>
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
              year: identifierNumber(answer.year),
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
  // Before anything is published the seller reads the draft; the state badge says so.
  const months =
    !plan.published && plan.draft ? plan.draft.months : plan.live_months;
  return (
    <div className="goal-plan">
      <Callout tone="info" title={t("goals.readOnly")} />
      <GoalBars months={months} />
      <GoalMonths months={months} currency={plan.currency} />
    </div>
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
          rows={roster.data}
          rowKey={(seller) => seller.owner_id}
          onRowClick={(seller) => replaceDial("seller", seller.owner_id)}
          columns={[
            {
              key: "name",
              header: t("goals.seller"),
              grow: true,
              render: (seller) => (
                <span className="goals-seller">
                  <Avatar name={seller.name} identity={seller.owner_id} />
                  <span className="goals-seller-name">
                    {seller.name}
                    <span className="t-caption t-num">
                      {seller.total_minor == null
                        ? t("goals.notSet")
                        : formatAnalyticsMoney(
                            seller.total_minor,
                            roster.currency,
                            locale,
                          )}
                    </span>
                  </span>
                </span>
              ),
            },
            {
              key: "state",
              header: t("goals.status"),
              align: "end",
              render: (seller) => <PlanStateBadge state={seller.state} />,
            },
          ]}
        />
      </PanelBody>
    </Panel>
  );
}
