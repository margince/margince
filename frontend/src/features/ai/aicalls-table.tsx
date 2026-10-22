// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { KeyedName } from "../../design-system/keyedname";
import { RowOpen } from "../../design-system/rowopen";
import { formatDateTime } from "../../format/format";
import { useNow } from "../../format/now";
import { formatRelativeTime } from "../../format/relativetime";
import { viewerZone } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { aiTaskLabel } from "../../lib/ai-task-label";
import {
  callCodeName,
  callCodeTone,
  isKnownCallCode,
} from "./ai-decision-labels";
import { costLabel, ladderAttempts } from "./aicalls-vocab";
import "./aicalls.css";

type CallSummary = components["schemas"]["AiCallSummary"];

const RELATIVE_TICK_MS = 60_000;

/** One badge, and only for a call that did not go as asked; a normal call carries none. */
export function CallOutcome({ call }: Readonly<{ call: CallSummary }>) {
  const t = useT();
  const code = call.error_sentinel;
  if (code) {
    const badge = (
      <Badge tone={callCodeTone(code)}>{callCodeName(code, t)}</Badge>
    );
    return isKnownCallCode(code) ? (
      badge
    ) : (
      <span className="aicalls-outcome">
        <CellStack>
          {badge}
          <code className="keyed-name-key">{code}</code>
        </CellStack>
      </span>
    );
  }
  if (call.degraded) {
    return <Badge tone="warning">{t("aicalls.badge.degraded")}</Badge>;
  }
  if (ladderAttempts(call) > 1) {
    return <Badge tone="warning">{t("aicalls.outcome.retried")}</Badge>;
  }
  return null;
}

// Five columns fit the settings column without a sideways scroll; the rung,
// tokens and latency are one press away in the drawer.
function callColumns(
  t: Translator,
  locale: Locale,
  now: number,
  onOpen: (call: CallSummary) => void,
): DataTableColumn<CallSummary>[] {
  const zone = viewerZone();
  return [
    {
      key: "when",
      header: t("aicalls.col.when"),
      render: (call) => (
        <time
          dateTime={call.occurred_at}
          title={formatDateTime(call.occurred_at, locale, zone)}
          className="aicalls-nowrap"
        >
          {formatRelativeTime(call.occurred_at, locale, new Date(now))}
        </time>
      ),
    },
    {
      key: "feature",
      header: t("aicalls.col.feature"),
      fold: "title",
      render: (call) => (
        <KeyedName
          name={aiTaskLabel(call.task, t, call.task_display_name)}
          code={call.task}
        />
      ),
    },
    {
      key: "model",
      header: t("aicalls.col.model"),
      render: (call) => (
        <code className="aicalls-model">{call.served_model}</code>
      ),
    },
    {
      key: "outcome",
      header: t("aicalls.col.outcome"),
      render: (call) => <CallOutcome call={call} />,
    },
    {
      key: "cost",
      header: t("aicalls.col.cost"),
      align: "end",
      // An unpriced call says so in words, with why on hover: a blank reads as free.
      render: (call) => (
        <span
          className="aicalls-nowrap"
          title={
            call.estimated_cost_microusd === undefined
              ? t("aicalls.cost.unpricedHint")
              : undefined
          }
        >
          {costLabel(call, locale, t)}
        </span>
      ),
    },
    {
      key: "open",
      header: t("table.actions"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (call) => (
        <RowOpen
          label={t("aicalls.open", {
            feature: aiTaskLabel(call.task, t, call.task_display_name),
            when: formatDateTime(call.occurred_at, locale, zone),
          })}
          onOpen={() => onOpen(call)}
        />
      ),
    },
  ];
}

/** The trace; a row opens its call in the drawer beside it. */
export function CallTable({
  calls,
  onOpen,
}: Readonly<{
  calls: readonly CallSummary[];
  onOpen: (call: CallSummary) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const now = useNow(RELATIVE_TICK_MS);
  return (
    <DataTable
      bleed
      fold
      label={t("aicalls.callsLabel")}
      columns={callColumns(t, locale, now, onOpen)}
      rows={[...calls]}
      rowKey={(call) => call.id}
      rowTestId={(call) => `call-${call.id}`}
      onRowClick={onOpen}
    />
  );
}
