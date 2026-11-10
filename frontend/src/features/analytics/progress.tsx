// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./progress.css";
import { useState } from "react";
import { Button, Field, SegmentedControl } from "../../design-system/atoms";
import { FilterBar } from "../../design-system/filterbar";
import { QueryGate } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { navigate } from "../../lib/router";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import {
  type AnalyticsScope,
  scopeKey,
  useAnalyticsContext,
} from "./analytics.context";
import {
  monthName,
  type ProgressAsk,
  periodLabel,
  type SalesProgress,
  useSalesProgress,
} from "./progress.queries";
import { ProgressReadings, ProgressResult } from "./progress.result";
import { ProgressMembers, ProgressMonths } from "./progress.tables";
import { ProgressWork } from "./progress.work";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import type { ReportingEvidenceRef } from "./reporting.model";

type Period = ProgressAsk["period"];
const PERIODS: readonly Period[] = ["month", "year"];

// The populations Progress reads: one seller, or one team. A workspace or a
// manager's whole set has no single goal to compare against.
function progressScopes(scopes: readonly AnalyticsScope[]): AnalyticsScope[] {
  return scopes.filter(
    (scope) => scope.kind === "owner" || scope.kind === "team",
  );
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
  const named = scopes.find((scope) => scopeKey(scope) === params.get("scope"));
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
        caption={progress.data && <ProgressCaption progress={progress.data} />}
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
                value={String(dials.month)}
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
                label: String(year),
              }))}
              value={String(dials.year)}
              onChange={(next) => replaceDial("year", next)}
            />
          )}
        </Field>
        {scopes.length > 1 && (
          <Field label={t("progress.scope")}>
            {(field) => (
              <Select
                {...field}
                options={scopes.map((each) => ({
                  value: scopeKey(each),
                  label: each.label,
                }))}
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

// Sales goals opens on the same year, and on the seller being read.
function goalDials(ask: ProgressAsk): Map<string, string> {
  const dials = new Map([["year", String(ask.year)]]);
  if (ask.scope?.kind === "owner") dials.set("seller", ask.scope.id);
  return dials;
}

function ProgressCaption({ progress }: Readonly<{ progress: SalesProgress }>) {
  const t = useT();
  const { locale } = useLocale();
  const whose =
    progress.scope.kind === "team"
      ? t("progress.teamTitle")
      : t("progress.myTitle");
  return (
    <span>
      {t("progress.heading", {
        whose,
        scope: progress.scope.label ?? "",
        period: periodLabel(progress.period, locale),
        cutoff: formatDateTime(progress.cutoff, locale, progress.timezone),
      })}
    </span>
  );
}

function ProgressBody({ progress }: Readonly<{ progress: SalesProgress }>) {
  const t = useT();
  const [evidence, setEvidence] = useState<ReportingEvidenceRef | null>(null);
  const open = progress.evaluation ? setEvidence : undefined;
  const team = progress.scope.kind === "team";
  return (
    <div className="progress-body">
      <ProgressResult progress={progress} onEvidence={open} />
      <ProgressReadings progress={progress} onEvidence={open} />
      {team && <ProgressMembers progress={progress} />}
      <ProgressWork progress={progress} />
      <ProgressMonths progress={progress} />
      <p className="t-caption progress-definition">
        {t("progress.definition")}
      </p>
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
