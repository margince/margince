// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import "../design-system/keyedname.css";
import { formatDateTime, formatNumber } from "../format/format";
import { useNow } from "../format/now";
import { formatRelativeTime } from "../format/relativetime";
import { viewerZone } from "../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import {
  callCodeName,
  callCodeTone,
  DECIDE_RUNG,
  isKnownCallCode,
  tierLabel,
} from "./ai-decision-labels";
import { providerName } from "./ai-provider-names";
import { costLabel, ladderAttempts, taskName } from "./aicalls-vocab";
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
      <CellStack>
        {badge}
        <code className="keyed-name-key">{code}</code>
      </CellStack>
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
  onOpen: (call: CallSummary) => void,
): DataTableColumn<CallSummary>[] {
  const zone = viewerZone();
  return [
    {
      key: "when",
      header: t("aicalls.col.when"),
      render: (call) => {
        const at = formatDateTime(call.occurred_at, locale, zone);
        return (
          <CellStack>
            <span className="aicalls-time">{at}</span>
            <time dateTime={call.occurred_at} title={at} className="t-caption">
              {formatRelativeTime(call.occurred_at, locale, new Date(now))}
            </time>
          </CellStack>
        );
      },
    },
    {
      key: "task",
      header: t("aicalls.col.task"),
      fold: "title",
      // The row answers a pointer too, but a `<tr>` takes no focus and
      // announces nothing, so the task's name is the button a keyboard opens by.
      render: (call) => (
        <CellStack>
          <Button
            variant="link"
            className="aicalls-feature"
            aria-label={t("aicalls.open", {
              feature: taskName(call),
              when: formatDateTime(call.occurred_at, locale, zone),
            })}
            onClick={(event) => {
              event.stopPropagation();
              onOpen(call);
            }}
          >
            {taskName(call)}
          </Button>
          {call.task_display_name && (
            <code className="keyed-name-key">{call.task}</code>
          )}
        </CellStack>
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
      key: "cost",
      header: t("aicalls.col.cost"),
      align: "end",
      // An unpriced call says so in words, with why on hover: a blank reads as free.
      render: (call) => (
        <span
          className="aicalls-figure"
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
