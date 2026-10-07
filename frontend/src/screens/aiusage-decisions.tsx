// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { EmptyState } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable } from "../design-system/datatable";
import { Popover } from "../design-system/popover";
import { SettingRow } from "../design-system/settingrow";
import { formatNumber, formatPercent } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { attemptReasonLabel } from "./ai-decision-labels";
import { TaskName } from "./ai-task-name";

type DecisionSummary = components["schemas"]["AiDecisionSummary"];

// A task's fallback rate, and where the number comes from: a button that opens
// the reasons one per line, largest first, each under the reason label the call
// trace uses — so an operator reads the same words on both screens. A task that
// never fell back has nothing to explain, so its rate is plain text.
function FallbackRate({
  row,
  locale,
}: Readonly<{ row: DecisionSummary; locale: Locale }>) {
  const t = useT();
  const shown = rate(fallbackCount(row.fallbacks), row.asked, locale);
  if (fallbackCount(row.fallbacks) === 0) return shown;
  return (
    <Popover
      className="evmark-trigger"
      label={
        <>
          <span aria-hidden>{shown}</span>
          <span className="sr-only">
            {t("aiusage.decisions.reasonsFor", { rate: shown })}
          </span>
        </>
      }
    >
      {fallbackLines(row.fallbacks, locale, t)}
    </Popover>
  );
}

function fallbackLines(
  fallbacks: DecisionSummary["fallbacks"],
  locale: Locale,
  t: ReturnType<typeof useT>,
): ReactNode {
  // Ties break on the reason KEY, a machine word, so no locale orders them.
  const entries = Object.entries(fallbacks).sort(
    ([a, left], [b, right]) => right - left || (a < b ? -1 : a > b ? 1 : 0),
  );
  return (
    <CellStack>
      {entries.map(([reason, calls]) => (
        <span key={reason}>
          {attemptReasonLabel(reason, t)}: {formatNumber(calls, locale)}
        </span>
      ))}
    </CellStack>
  );
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
  taskSummary,
}: Readonly<{
  decisions: DecisionSummary[];
  taskName: (task: string) => string;
  taskSummary: (task: string) => string | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const columns = [
    {
      key: "task",
      header: t("aiusage.col.task"),
      render: (r: DecisionSummary) => (
        <TaskName name={taskName(r.task)} summary={taskSummary(r.task)} />
      ),
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
      render: (r: DecisionSummary) => <FallbackRate row={r} locale={locale} />,
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
