// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { KeyedName } from "../design-system/keyedname";
import { formatDateTime, formatNumber } from "../format/format";
import { useNow } from "../format/now";
import { formatRelativeTime } from "../format/relativetime";
import { viewerZone } from "../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import { DECIDE_RUNG, tierLabel } from "./ai-decision-labels";
import { providerName } from "./ai-provider-names";
import { CallDetailPanel } from "./aicalls-detail";
import { sentinelLabel } from "./aicalls-sentinel";
import "./aicalls.css";

type CallSummary = components["schemas"]["AiCallSummary"];

const RELATIVE_TICK_MS = 60_000;

// The attempts the ladder itself made. A decision model that fell back is the
// first of `calls_attempted`; the next rung is a second model, not a retry.
function ladderAttempts(call: CallSummary): number {
  return call.decision_attempted
    ? call.calls_attempted - 1
    : call.calls_attempted;
}

/** One badge, and only for a call that did not go as asked; a normal call carries none. */
export function CallOutcome({ call }: Readonly<{ call: CallSummary }>) {
  const t = useT();
  if (call.error_sentinel) {
    return <Badge tone="danger">{sentinelLabel(call.error_sentinel, t)}</Badge>;
  }
  if (call.degraded) {
    return <Badge tone="warning">{t("aicalls.badge.degraded")}</Badge>;
  }
  if (ladderAttempts(call) > 1) {
    return <Badge tone="warning">{t("aicalls.outcome.retried")}</Badge>;
  }
  return null;
}

// The rung the call ended on and its vendor. A tier is empty on a call that
// failed before routing; a decision model asked first is named before the rung.
function rungCaption(call: CallSummary, t: Translator): string {
  const tier = tierLabel(call.tier, t);
  const rung =
    call.decision_attempted && call.tier !== DECIDE_RUNG
      ? t("aicalls.model.afterDecision", { tier })
      : tier;
  return [rung, providerName(call.provider, t)].filter(Boolean).join(" · ");
}

function callColumns(
  t: Translator,
  locale: Locale,
  now: number,
): DataTableColumn<CallSummary>[] {
  const zone = viewerZone();
  return [
    {
      key: "when",
      header: t("aicalls.col.when"),
      render: (call) => {
        const at = formatDateTime(call.occurred_at, locale, zone);
        return (
          <time dateTime={call.occurred_at} title={at}>
            <CellStack>
              <span className="aicalls-time">{at}</span>
              <span className="t-caption">
                {formatRelativeTime(call.occurred_at, locale, new Date(now))}
              </span>
            </CellStack>
          </time>
        );
      },
    },
    {
      key: "task",
      header: t("aicalls.col.task"),
      fold: "title",
      render: (call) => (
        <KeyedName
          name={call.task_display_name ?? call.task}
          code={call.task}
        />
      ),
    },
    {
      key: "model",
      header: t("aicalls.col.model"),
      render: (call) => (
        <span className="aicalls-model-line">
          <CellStack>
            <code className="aicalls-model">{call.served_model}</code>
            <span className="t-caption">{rungCaption(call, t)}</span>
          </CellStack>
        </span>
      ),
    },
    {
      key: "tokens",
      header: t("aicalls.col.tokens"),
      align: "end",
      render: (call) => (
        <span className="aicalls-figure">
          <CellStack>
            <span>
              {formatNumber(call.tokens_in, locale)} /{" "}
              {formatNumber(call.tokens_out, locale)}
            </span>
            {call.cache_hit && (
              <span className="t-caption">{t("aicalls.badge.cacheHit")}</span>
            )}
          </CellStack>
        </span>
      ),
    },
    {
      key: "latency",
      header: t("aicalls.col.latency"),
      align: "end",
      render: (call) => (
        <span className="aicalls-figure">
          {t("aicalls.ms", { value: formatNumber(call.latency_ms, locale) })}
        </span>
      ),
    },
    {
      key: "outcome",
      header: t("aicalls.col.outcome"),
      fold: "end",
      render: (call) => <CallOutcome call={call} />,
    },
  ];
}

export function CallTable({
  calls,
  captureEnabled,
}: Readonly<{ calls: readonly CallSummary[]; captureEnabled: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const now = useNow(RELATIVE_TICK_MS);
  const zone = viewerZone();
  return (
    <DataTable
      bleed
      fold
      label={t("aicalls.callsLabel")}
      columns={callColumns(t, locale, now)}
      rows={[...calls]}
      rowKey={(call) => call.id}
      rowTestId={(call) => `call-${call.id}`}
      detail={{
        header: t("aicalls.col.detail"),
        toggleLabel: (call) =>
          t("aicalls.expandCall", {
            task: call.task_display_name ?? call.task,
            when: formatDateTime(call.occurred_at, locale, zone),
          }),
        render: (call) => (
          <CallDetailPanel id={call.id} captureEnabled={captureEnabled} />
        ),
      }}
    />
  );
}
