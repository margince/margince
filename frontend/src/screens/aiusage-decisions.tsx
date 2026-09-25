// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { EmptyState } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { SettingRow } from "../design-system/settingrow";
import { formatNumber, formatPercent } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { attemptReasonLabel } from "./ai-decision-labels";

type DecisionSummary = components["schemas"]["AiDecisionSummary"];

// A task's fallbacks as one line, largest first, each under the reason label
// the call trace uses — so an operator reads the same words on both screens.
function fallbackLine(
  fallbacks: DecisionSummary["fallbacks"],
  locale: Locale,
  t: ReturnType<typeof useT>,
): string {
  const entries = Object.entries(fallbacks).sort(
    ([a, left], [b, right]) => right - left || a.localeCompare(b),
  );
  if (entries.length === 0) return "—";
  return entries
    .map(
      ([reason, calls]) =>
        `${attemptReasonLabel(reason, t)}: ${formatNumber(calls, locale)}`,
    )
    .join(" · ");
}

function fallbackCount(fallbacks: DecisionSummary["fallbacks"]): number {
  return Object.values(fallbacks).reduce((sum, calls) => sum + calls, 0);
}

// A rate of nothing asked is no rate, and a 0% would state one.
function rate(part: number, whole: number, locale: Locale): string {
  return whole === 0 ? "—" : formatPercent(part / whole, locale);
}

// The decision model's pass and fallback rates per task. Each count is of
// logical calls, which the server reads from the call trace — the spend table's
// calls count attempts, and a decision that fell back is two of those.
export function DecisionSummaryRow({
  decisions,
  taskName,
}: Readonly<{
  decisions: DecisionSummary[];
  taskName: (task: string) => string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const columns = [
    {
      key: "task",
      header: t("aiusage.col.task"),
      render: (r: DecisionSummary) => taskName(r.task),
    },
    {
      key: "asked",
      header: t("aiusage.decisions.col.asked"),
      render: (r: DecisionSummary) => formatNumber(r.asked, locale),
    },
    {
      key: "pass",
      header: t("aiusage.decisions.col.passRate"),
      render: (r: DecisionSummary) => rate(r.decided, r.asked, locale),
    },
    {
      key: "fallback",
      header: t("aiusage.decisions.col.fallbackRate"),
      // From the fallbacks rather than asked minus decided: a consultation
      // that neither stood nor handed a reason on is neither, and folding it
      // into either rate would claim to know which it was.
      render: (r: DecisionSummary) =>
        rate(fallbackCount(r.fallbacks), r.asked, locale),
    },
    {
      key: "reasons",
      header: t("aiusage.decisions.col.reasons"),
      render: (r: DecisionSummary) => fallbackLine(r.fallbacks, locale, t),
    },
  ];
  return (
    <SettingRow
      layout="stack"
      label={t("aiTier.decide")}
      description={t("aiusage.decisions.note")}
      control={
        <div className="settingrow-measure">
          {decisions.length === 0 ? (
            <EmptyState>{t("aiusage.decisions.empty")}</EmptyState>
          ) : (
            <DataTable
              label={t("aiTier.decide")}
              columns={columns}
              rows={decisions}
              rowKey={(row) => row.task}
            />
          )}
        </div>
      }
    />
  );
}
