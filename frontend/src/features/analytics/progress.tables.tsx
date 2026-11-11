// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AppLink } from "../../design-system/applink";
import { Avatar } from "../../design-system/avatar";
import { Callout } from "../../design-system/callout";
import { DataTable } from "../../design-system/datatable";
import { Panel, PanelBody } from "../../design-system/panel";
import { Meter } from "../../design-system/readings";
import {
  formatAnalyticsMoney,
  formatNumber,
  identifierNumber,
  monthName,
} from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { routeHash } from "../../lib/router";
import { hashWithParams, replaceDial, useUrlParams } from "../../lib/urlstate";
import type {
  SalesProgress,
  SalesProgressMember,
  SalesProgressMonth,
} from "./progress.queries";
import { amountText, DeltaBadge, goalText } from "./progress.result";

/** Each team member's goal, closed and planned revenue, in name order. */
export function ProgressMembers({
  progress,
}: Readonly<{ progress: SalesProgress }>) {
  const t = useT();
  const { locale } = useLocale();
  const { currency } = progress;
  const [params] = useUrlParams();
  const sellerHref = (owner: string) =>
    hashWithParams(
      routeHash({ screen: "progress" }),
      new Map([...params, ["scope", `owner:${owner}`]]),
    );
  return (
    <Panel title={t("progress.team")}>
      <PanelBody>
        {progress.members_withheld && (
          <Callout tone="info" title={t("progress.membersWithheld")} />
        )}
        {progress.members.length > 0 && (
          <DataTable<SalesProgressMember>
            label={t("progress.team")}
            fold
            rows={progress.members}
            rowKey={(member) => member.owner_id}
            // The seller's own view, for the same period.
            onRowClick={(member) =>
              replaceDial("scope", `owner:${member.owner_id}`)
            }
            columns={[
              {
                key: "name",
                header: t("progress.member"),
                // A row takes no focus, so the name is the keyboard's way in.
                render: (member) => (
                  <span className="progress-member">
                    <Avatar name={member.name} identity={member.owner_id} />
                    <AppLink href={sellerHref(member.owner_id)}>
                      {member.name}
                    </AppLink>
                  </span>
                ),
                fold: "title",
                grow: true,
              },
              {
                key: "goal",
                header: t("progress.goal"),
                align: "end",
                render: (member) => goalText(member.goal, currency, locale, t),
              },
              {
                key: "closed",
                header: t("progress.closedRevenue"),
                align: "end",
                render: (member) => (
                  <span className="progress-closedcell">
                    {amountText(member.closed, currency, locale, t)}
                    {member.closed.amount_minor != null &&
                      member.goal.amount_minor != null &&
                      member.goal.amount_minor > 0 && (
                        <Meter
                          dense
                          value={Math.min(
                            member.closed.amount_minor,
                            member.goal.amount_minor,
                          )}
                          max={member.goal.amount_minor}
                          label={t("progress.ofGoal", { name: member.name })}
                        />
                      )}
                  </span>
                ),
              },
              {
                key: "delta",
                header: t("progress.delta"),
                align: "end",
                render: (member) => (
                  <DeltaBadge delta={member.delta_minor} currency={currency} />
                ),
              },
              {
                key: "planned",
                header: t("progress.planned"),
                align: "end",
                render: (member) =>
                  amountText(member.planned, currency, locale, t),
              },
            ]}
          />
        )}
      </PanelBody>
    </Panel>
  );
}

/** The twelve months of the year: goal, closed revenue and the difference. */
export function ProgressMonths({
  progress,
}: Readonly<{ progress: SalesProgress }>) {
  const t = useT();
  const { locale } = useLocale();
  const { currency } = progress;
  return (
    <Panel
      title={t("progress.months", {
        year: identifierNumber(progress.period.year),
      })}
      footer={<YearLine progress={progress} />}
    >
      <PanelBody>
        <MonthsChart progress={progress} />
        <DataTable<SalesProgressMonth>
          fold
          label={t("progress.months", {
            year: identifierNumber(progress.period.year),
          })}
          rows={progress.months}
          rowKey={(row) => String(row.month)}
          columns={[
            {
              key: "month",
              header: t("progress.month"),
              render: (row) => monthName(row.month, locale),
              fold: "title",
              grow: true,
            },
            {
              key: "goal",
              header: t("progress.goal"),
              align: "end",
              render: (row) => goalText(row.goal, currency, locale, t),
            },
            {
              key: "closed",
              header: t("progress.closedRevenue"),
              align: "end",
              render: (row) => amountText(row.closed, currency, locale, t),
            },
            {
              key: "delta",
              header: t("progress.delta"),
              align: "end",
              render: (row) =>
                row.state === "not_started" ? (
                  <span className="t-caption">{t("progress.notStarted")}</span>
                ) : (
                  <DeltaBadge delta={row.delta_minor} currency={currency} />
                ),
            },
          ]}
        />
      </PanelBody>
    </Panel>
  );
}

// Each month's goal as an outline and its closed revenue filled inside it. The
// table below carries the same figures for a screen reader.
function MonthsChart({ progress }: Readonly<{ progress: SalesProgress }>) {
  const { locale } = useLocale();
  const amounts = progress.months.flatMap((row) => [
    row.goal.amount_minor ?? 0,
    row.closed.amount_minor ?? 0,
  ]);
  const top = Math.max(...amounts, 1);
  const height = (amount: number | undefined) =>
    `${((amount ?? 0) / top) * 100}%`;
  return (
    <div className="progress-chart" aria-hidden="true">
      {progress.months.map((row) => {
        const goal = row.goal.amount_minor;
        const closed = row.closed.amount_minor;
        const met = goal != null && closed != null && closed >= goal;
        return (
          <div
            key={row.month}
            className="progress-chart-month"
            data-state={row.state}
            data-met={met || undefined}
          >
            <div className="progress-chart-plot">
              {goal != null && (
                <span
                  className="progress-chart-goal"
                  style={{ height: height(goal) }}
                />
              )}
              {closed != null && row.state !== "not_started" && (
                <span
                  className="progress-chart-closed"
                  style={{ height: height(closed) }}
                />
              )}
            </div>
            <span className="progress-chart-name">
              <span className="month-short">
                {monthName(row.month, locale, "short")}
              </span>
              <span className="month-narrow">
                {monthName(row.month, locale, "narrow")}
              </span>
            </span>
          </div>
        );
      })}
    </div>
  );
}

// The year so far against the whole year's goal, once every month has one.
function YearLine({ progress }: Readonly<{ progress: SalesProgress }>) {
  const t = useT();
  const { locale } = useLocale();
  // Only a year reading's goal says whether a changed plan awaits review, and
  // the year is compared only once every month has a whole goal and a reading.
  const whole =
    progress.period.kind === "year" &&
    progress.goal.status !== "changed_outside_plan" &&
    progress.months.every(
      (row) =>
        row.goal.amount_minor != null &&
        row.goal.status !== "incomplete" &&
        (row.closed.status === "ok" || row.closed.status === "not_started"),
    );
  if (!whole) return null;
  const goals = progress.months.map((row) => row.goal.amount_minor);
  const goal = goals.reduce<number>((sum, each) => sum + (each ?? 0), 0);
  const closed = progress.months.reduce(
    (sum, row) => sum + (row.closed.amount_minor ?? 0),
    0,
  );
  const money = (minor: number) =>
    formatAnalyticsMoney(minor, progress.currency, locale);
  return (
    <span className="t-caption t-num">
      {t("progress.yearLine", {
        year: identifierNumber(progress.period.year),
        closed: money(closed),
        goal: money(goal),
        percent:
          goal > 0
            ? formatNumber(Math.round((closed / goal) * 100), locale)
            : "–",
      })}
    </span>
  );
}
