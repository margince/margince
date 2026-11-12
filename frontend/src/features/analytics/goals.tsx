// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./progress.css";
import { type ReactNode, useState } from "react";
import { AppLink } from "../../design-system/applink";
import { Badge, Button, Field } from "../../design-system/atoms";
import { Avatar } from "../../design-system/avatar";
import { DataTable } from "../../design-system/datatable";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { Meter } from "../../design-system/readings";
import { Select } from "../../design-system/select";
import {
  formatAnalyticsMoney,
  formatDateTime,
  formatNumber,
  identifierNumber,
} from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor, valueFor } from "../../lib/labelfor";
import { navigate, routeHash } from "../../lib/router";
import { hashWithParams, replaceDial, useUrlParams } from "../../lib/urlstate";
import { useViewerZone } from "../../lib/viewerzone";
import { GoalBars, GoalMonths } from "./goals.bars";
import { GoalEditor, WithdrawConfirm } from "./goals.editor";
import {
  type SalesGoalPlan,
  type SalesGoalRoster,
  useSalesGoalPlan,
  useSalesGoalRoster,
  useSalesProgress,
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
 * Sales goals: one team member's twelve monthly goals for a calendar year. A
 * rep reads their own; a manager drafts, publishes and withdraws them.
 */
export function SalesGoalsScreen() {
  const t = useT();
  const [params] = useUrlParams();
  const [now] = useState(() => new Date());
  const year = Number(params.get("year")) || now.getFullYear();
  const roster = useSalesGoalRoster(year);
  return (
    <div className="wrap progress-page">
      <QueryGate query={roster} pendingLabel={t("goals.reading")}>
        {(answer) => {
          const seller =
            answer.data.find(
              (each) => each.owner_id === params.get("seller"),
            ) ?? answer.data[0];
          const listed = answer.data.length > 1;
          return (
            <div className={listed ? "goals-board" : undefined}>
              {listed && (
                <Roster
                  roster={answer}
                  selected={seller?.owner_id}
                  year={<YearSelect year={year} now={now} />}
                />
              )}
              <div className="goals-board-plan">
                {seller && (
                  <SellerPlan
                    owner={seller.owner_id}
                    year={year}
                    yearSelect={
                      listed ? undefined : <YearSelect year={year} now={now} />
                    }
                  />
                )}
              </div>
            </div>
          );
        }}
      </QueryGate>
    </div>
  );
}

// The year in hand, in the head of the list it picks for.
function YearSelect({ year, now }: Readonly<{ year: number; now: Date }>) {
  const t = useT();
  const years = Array.from(
    { length: 5 },
    (_, index) => now.getFullYear() + 1 - index,
  );
  return (
    <Field label={t("progress.year")} labelHidden>
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
  );
}

function openProgress(owner: string, year: number) {
  navigate(
    { screen: "progress" },
    new Map([
      ["period", "year"],
      ["year", identifierNumber(year)],
      ["scope", `owner:${owner}`],
    ]),
  );
}

function SellerPlan({
  owner,
  year,
  yearSelect,
}: Readonly<{ owner: string; year: number; yearSelect?: ReactNode }>) {
  const t = useT();
  const plan = useSalesGoalPlan(owner, year);
  // A published plan reads as a plan until the manager asks to change it.
  const [changing, setChanging] = useState<string | null>(null);
  // A pending draft opens in the editor; this is the plan the manager left it on.
  const [left, setLeft] = useState<string | null>(null);
  return (
    <QueryGate query={plan} pendingLabel={t("goals.reading")}>
      {(answer) => {
        const key = planKey(answer);
        const settled =
          answer.state === "published"
            ? changing !== key
            : answer.state === "published_with_draft" && left === key;
        const edits = answer.capabilities.edit && !settled;
        return (
          <>
            <Panel
              title={t("goals.planTitle", {
                name: answer.owner_name,
                year: identifierNumber(answer.year),
              })}
              titleAction={
                <span className="goal-head-end">
                  {yearSelect}
                  <PlanStateBadge state={answer.state} />
                </span>
              }
              footer={
                answer.published ? undefined : (
                  <Button
                    variant="link"
                    onClick={() => openProgress(answer.owner_id, answer.year)}
                  >
                    {t("goals.viewProgress")}
                  </Button>
                )
              }
            >
              <PanelBody>
                {edits ? (
                  <GoalEditor
                    key={`${answer.owner_id}:${answer.version}`}
                    plan={answer}
                    onCancel={
                      answer.published
                        ? () => {
                            setChanging(null);
                            setLeft(key);
                          }
                        : undefined
                    }
                  />
                ) : (
                  <ReadOnlyPlan
                    plan={answer}
                    onChange={
                      answer.capabilities.edit
                        ? () => {
                            setChanging(key);
                            setLeft(null);
                          }
                        : undefined
                    }
                  />
                )}
              </PanelBody>
            </Panel>
            {answer.history.length > 0 && <PlanHistory plan={answer} />}
          </>
        );
      }}
    </QueryGate>
  );
}

// One plan as it stands, so a save or a publication closes the editor again.
function planKey(plan: SalesGoalPlan): string {
  return `${plan.owner_id}:${plan.year}:${plan.version}`;
}

// The plan as text. A rep reads their own this way, and a manager a published
// one, with the way into the editor.
function ReadOnlyPlan({
  plan,
  onChange,
}: Readonly<{ plan: SalesGoalPlan; onChange?: () => void }>) {
  const t = useT();
  const [withdrawing, setWithdrawing] = useState(false);
  // Before anything is published the reader sees the draft; the state badge says so.
  const months =
    !plan.published && plan.draft ? plan.draft.months : plan.live_months;
  return (
    <div className="goal-plan">
      <p className="t-caption goal-note">
        {onChange
          ? t("goals.sellerReads", { name: plan.owner_name })
          : t("goals.readOnly")}
      </p>
      {plan.published && <YearSoFar plan={plan} />}
      <GoalBars months={months} />
      <GoalMonths months={months} currency={plan.currency} />
      {onChange && (
        <div className="goal-actions">
          <Button variant="ghost" onClick={() => setWithdrawing(true)}>
            {t("goals.withdraw")}
          </Button>
          <Button variant="ghost" onClick={onChange}>
            {t("goals.change")}
          </Button>
        </div>
      )}
      {withdrawing && (
        <WithdrawConfirm plan={plan} onClose={() => setWithdrawing(false)} />
      )}
    </div>
  );
}

// Where the published goals now show: the year's sales won against them, one
// door from Progress.
function YearSoFar({ plan }: Readonly<{ plan: SalesGoalPlan }>) {
  const t = useT();
  const { locale } = useLocale();
  const progress = useSalesProgress({
    period: "year",
    year: plan.year,
    month: 1,
    scope: { kind: "owner", id: plan.owner_id },
  });
  const closed = progress.data?.closed.amount_minor;
  const goal = progress.data?.goal.amount_minor;
  const money = (minor: number) =>
    formatAnalyticsMoney(minor, plan.currency, locale);
  return (
    <div className="goal-sofar">
      {closed != null && goal != null && goal > 0 && (
        <>
          <span className="goal-sofar-line t-num">
            {t("progress.yearLine", {
              year: identifierNumber(plan.year),
              closed: money(closed),
              goal: money(goal),
              percent: formatNumber(Math.round((closed / goal) * 100), locale),
            })}
          </span>
          <Meter
            dense
            flat
            value={Math.min(closed, goal)}
            max={goal}
            label={t("progress.ofGoal", { name: plan.owner_name })}
          />
        </>
      )}
      <Button
        variant="link"
        onClick={() => openProgress(plan.owner_id, plan.year)}
      >
        {t("goals.viewProgress")}
      </Button>
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

// What the list says under a name: the published year, else the draft's, so
// a drafted plan never reads as not set.
function sellerLine(
  seller: SalesGoalRoster["data"][number],
  money: (minor: number) => string,
  t: Translator,
): string {
  if (seller.total_minor != null) return money(seller.total_minor);
  if (seller.draft_total_minor != null)
    return t("goals.draftTotal", { amount: money(seller.draft_total_minor) });
  return t("goals.notSet");
}

// The list is the picker: a row opens that team member's plan and marks it.
function Roster({
  roster,
  selected,
  year,
}: Readonly<{
  roster: SalesGoalRoster;
  selected: string | undefined;
  year: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [params] = useUrlParams();
  const money = (minor: number) =>
    formatAnalyticsMoney(minor, roster.currency, locale);
  const sellerHref = (owner: string) =>
    hashWithParams(
      routeHash({ screen: "goals" }),
      new Map([...params, ["seller", owner]]),
    );
  return (
    <Panel title={t("goals.roster")} titleAction={year}>
      <PanelBody>
        <DataTable
          label={t("goals.roster")}
          rows={roster.data}
          rowKey={(seller) => seller.owner_id}
          selectedKey={selected}
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
                    {/* A row takes no focus, so the name is the keyboard's way in. */}
                    <AppLink
                      className="entity-link"
                      href={sellerHref(seller.owner_id)}
                    >
                      {seller.name}
                    </AppLink>
                    <span className="t-caption t-num">
                      {sellerLine(seller, money, t)}
                    </span>
                  </span>
                </span>
              ),
            },
            {
              key: "state",
              header: t("goals.status"),
              headerHidden: true,
              align: "end",
              render: (seller) => <PlanStateBadge state={seller.state} />,
            },
          ]}
        />
      </PanelBody>
    </Panel>
  );
}
