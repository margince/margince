// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./analytics.board.css";
import "./progress.css";
import { useState } from "react";
import { Button, Field, SegmentedControl } from "../../design-system/atoms";
import { FilterBar } from "../../design-system/filterbar";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { isoDay } from "../../format/calendarday";
import {
  formatDateTime,
  formatNumber,
  identifierNumber,
  monthName,
} from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { navigate } from "../../lib/router";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { isUuid } from "../../lib/uuid";
import {
  type AnalyticsScope,
  scopeKey,
  useAnalyticsContext,
} from "./analytics.context";
import {
  type ProgressAsk,
  periodLabel,
  type SalesProgress,
  useSalesProgress,
} from "./progress.queries";
import { goalText, ProgressReadings, ProgressResult } from "./progress.result";
import { ProgressMembers, ProgressMonths } from "./progress.tables";
import { ProgressWork } from "./progress.work";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import type { ReportingEvidenceRef } from "./reporting.model";

type Period = ProgressAsk["period"];
const PERIODS: readonly Period[] = ["month", "year"];

// The populations Progress reads: one team member, or one team inside the
// reader's own record access. A workspace or a manager's whole set has no
// single goal to compare against, and the server refuses any other team.
function progressScopes(scopes: readonly AnalyticsScope[]): AnalyticsScope[] {
  return scopes.filter(
    (scope) =>
      scope.kind === "owner" ||
      (scope.kind === "team" && scope.measurable !== false),
  );
}

// A team row opens one seller, whom the offer does not list. The server still
// decides whether the reader may see that seller.
function sellerFromKey(key: string | undefined): AnalyticsScope | undefined {
  const id = key?.startsWith("owner:") ? key.slice("owner:".length) : "";
  return isUuid(id) ? { kind: "owner", id, label: "" } : undefined;
}

function readDials(params: ReadonlyMap<string, string>, now: Date) {
  const period: Period = params.get("period") === "year" ? "year" : "month";
  const year = Number(params.get("year")) || now.getFullYear();
  const month = Number(params.get("month")) || now.getMonth() + 1;
  return { period, year, month: Math.min(Math.max(month, 1), 12) };
}

/**
 * My progress and Team progress: closed revenue against the sales goal for one
 * month or year. Every figure is Analytics' own reading, opened to its records.
 */
export function ProgressScreen() {
  const t = useT();
  const { locale } = useLocale();
  const [params] = useUrlParams();
  const [now] = useState(() => new Date());
  const context = useAnalyticsContext();
  const scopes = progressScopes(context.data?.allowed_scopes ?? []);
  const named =
    scopes.find((scope) => scopeKey(scope) === params.get("scope")) ??
    sellerFromKey(params.get("scope"));
  const defaultScope =
    context.data?.default_scope.kind === "owner"
      ? undefined
      : scopes.find((scope) => scope.kind === "team");
  const scope = named ?? defaultScope;
  const dials = readDials(params, now);
  const ask: ProgressAsk = {
    ...dials,
    scope:
      scope?.id && (scope.kind === "owner" || scope.kind === "team")
        ? { kind: scope.kind, id: scope.id }
        : undefined,
  };
  const progress = useSalesProgress(ask);
  const years = Array.from(
    { length: 5 },
    (_, index) => now.getFullYear() + 1 - index,
  );
  return (
    <div className="wrap progress-page">
      <FilterBar
        label={t("progress.controls")}
        actions={
          <Button
            variant="ghost"
            onClick={() => navigate({ screen: "goals" }, goalDials(ask))}
          >
            {t("progress.viewGoals")}
          </Button>
        }
        caption={
          progress.data &&
          t("progress.figuresUpTo", {
            cutoff: formatDateTime(
              progress.data.cutoff,
              locale,
              progress.data.timezone,
            ),
          })
        }
      >
        <SegmentedControl
          label={t("progress.period")}
          options={PERIODS}
          value={dials.period}
          labels={{ month: t("progress.monthly"), year: t("progress.yearly") }}
          onChange={(next) => replaceDial("period", next)}
        />
        {dials.period === "month" && (
          <Field label={t("progress.month")}>
            {(field) => (
              <Select
                {...field}
                options={Array.from({ length: 12 }, (_, index) => ({
                  value: String(index + 1),
                  label: monthName(index + 1, locale),
                }))}
                value={identifierNumber(dials.month)}
                onChange={(next) => replaceDial("month", next)}
              />
            )}
          </Field>
        )}
        <Field label={t("progress.year")}>
          {(field) => (
            <Select
              {...field}
              options={years.map((year) => ({
                value: String(year),
                label: identifierNumber(year),
              }))}
              value={identifierNumber(dials.year)}
              onChange={(next) => replaceDial("year", next)}
            />
          )}
        </Field>
        {scopes.length > 1 && (
          <Field label={t("progress.scope")}>
            {(field) => (
              <Select
                {...field}
                options={scopeOptions(scopes, named, progress.data)}
                value={
                  scope
                    ? scopeKey(scope)
                    : scopeKey(context.data?.default_scope ?? { kind: "owner" })
                }
                onChange={(next) => replaceDial("scope", next)}
              />
            )}
          </Field>
        )}
      </FilterBar>
      <QueryGate query={progress} pendingLabel={t("progress.reading")}>
        {(data) => <ProgressBody progress={data} />}
      </QueryGate>
    </div>
  );
}

// A seller opened from a team row joins the picker under the name the
// progress answer carries, so the control shows who is being read.
function scopeOptions(
  scopes: readonly AnalyticsScope[],
  named: AnalyticsScope | undefined,
  progress: SalesProgress | undefined,
) {
  const options = scopes.map((each) => ({
    value: scopeKey(each),
    label: each.label,
  }));
  if (named && !options.some((each) => each.value === scopeKey(named)))
    options.push({
      value: scopeKey(named),
      label: progress?.scope.label ?? "",
    });
  return options;
}

// Sales goals opens on the same year, and on the seller being read.
function goalDials(ask: ProgressAsk): Map<string, string> {
  const dials = new Map([["year", String(ask.year)]]);
  if (ask.scope?.kind === "owner") dials.set("seller", ask.scope.id);
  return dials;
}

function ProgressBody({ progress }: Readonly<{ progress: SalesProgress }>) {
  const t = useT();
  const [evidence, setEvidence] = useState<ReportingEvidenceRef | null>(null);
  const open = progress.evaluation ? setEvidence : undefined;
  const team = progress.scope.kind === "team";
  return (
    <div className="analytics-board">
      <div className="analytics-board-main">
        <div className="analytics-sheet">
          <ProgressResult
            progress={progress}
            onEvidence={open}
            onGoals={() =>
              navigate({ screen: "goals" }, goalDials(askOf(progress)))
            }
          />
          <ProgressReadings progress={progress} onEvidence={open} />
          {team && <ProgressMembers progress={progress} />}
          <ProgressWork progress={progress} />
          <ProgressMonths progress={progress} />
        </div>
      </div>
      <aside className="analytics-board-rail" aria-label={t("progress.rail")}>
        <ProgressGoalBasis progress={progress} />
      </aside>
      {evidence && progress.evaluation && (
        <ReportingEvidenceDrawer
          evaluation={progress.evaluation}
          reference={evidence}
          onClose={() => setEvidence(null)}
        />
      )}
    </div>
  );
}

// The question a reading answered, to open another screen on the same one.
function askOf(progress: SalesProgress): ProgressAsk {
  const { scope, period } = progress;
  return {
    period: period.kind === "year" ? "year" : "month",
    year: period.year,
    month: period.month ?? 1,
    scope:
      scope.id && (scope.kind === "owner" || scope.kind === "team")
        ? { kind: scope.kind, id: scope.id }
        : undefined,
  };
}

// Analytics Performance on the same team member or team and the same days.
function analyticsDials(ask: ProgressAsk): Map<string, string> {
  const month = ask.period === "year" ? 1 : ask.month;
  const months = ask.period === "year" ? 12 : 1;
  const dials = new Map<string, string>([
    ["period", "custom"],
    ["from", isoDay(new Date(ask.year, month - 1, 1))],
    ["through", isoDay(new Date(ask.year, month - 1 + months, 0))],
  ]);
  if (ask.scope) dials.set("scope", `${ask.scope.kind}:${ask.scope.id}`);
  return dials;
}

// The goal the reading stands against, what counts towards it, and the door
// to the Analytics view behind the figure.
function ProgressGoalBasis({
  progress,
}: Readonly<{ progress: SalesProgress }>) {
  const t = useT();
  const { locale } = useLocale();
  const { goal, currency } = progress;
  // Analytics opens only a scope it offers this reader. A team member opened
  // from a team row may not be one, so the door is left out for them.
  const offered = useAnalyticsContext().data?.allowed_scopes.some(
    (scope) => scopeKey(scope) === scopeKey(progress.scope),
  );
  return (
    <Panel
      title={t("progress.goal")}
      flatHead
      footer={
        offered && (
          <Button
            variant="link"
            onClick={() =>
              navigate(
                { screen: "analytics", id: "performance" },
                analyticsDials(askOf(progress)),
              )
            }
          >
            {t("progress.openInAnalytics")}
          </Button>
        )
      }
    >
      <PanelBody>
        <dl className="progress-basis">
          {goal.status !== "missing" && (
            <div>
              <dt>
                {t("progress.goalFor", {
                  period: periodLabel(progress.period, locale),
                })}
              </dt>
              <dd className="t-num">{goalText(goal, currency, locale, t)}</dd>
            </div>
          )}
          <div>
            <dt>{t("progress.goalMonths")}</dt>
            <dd className="t-num">
              {t("progress.goalMonthsSet", {
                set: formatNumber(goal.months_set, locale),
                needed: formatNumber(goal.months_needed, locale),
              })}
            </dd>
          </div>
        </dl>
        <p className="t-caption progress-definition">
          {t("progress.definition")}
        </p>
      </PanelBody>
    </Panel>
  );
}
