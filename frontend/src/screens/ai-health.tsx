import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, DataTable, EmptyState } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { QueryGate, throwProblem } from "./common";

// Whether the model lanes are answering.
//
// The binding card above says which vendor serves a tier and the keys card says
// whether this installation can call it. Neither says whether it ANSWERED, and
// under the capture posture that gap is expensive: a thread stays held whether
// the classifier judged it confidential or never replied at all, so an outage
// and correct cautious behaviour look identical until somebody asks why a
// thread never opened.

type RungHealth = components["schemas"]["AiRungHealth"];

export function AiHealthCard() {
  const t = useT();
  const { locale } = useLocale();
  const query = useQuery({
    queryKey: ["ai-health"],
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/health");
      if (error) throwProblem(error);
      return data;
    },
    // The question is whether it is answering NOW, so a reader who leaves this
    // page open watches it rather than reading a snapshot from when they
    // arrived. One minute against a one-hour window: often enough to notice a
    // lane die, rare enough to cost nothing.
    refetchInterval: 60_000,
  });

  return (
    <Panel title={t("aiHealth.title")}>
      <PanelBody>
        <p className="settings-panel-sub">{t("aiHealth.sub")}</p>
        <QueryGate query={query}>
          {(health) =>
            health.rungs.length === 0 ? (
              // No rung called anything in the window. That is not an outage —
              // an installation nobody used this hour looks exactly the same —
              // so it says which it is rather than drawing an empty table a
              // reader would read as failure.
              <EmptyState>
                {t("aiHealth.noCalls", {
                  hours: formatNumber(health.window_hours, locale),
                })}
              </EmptyState>
            ) : (
              <RungTable rungs={health.rungs} hours={health.window_hours} />
            )
          }
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

function RungTable({
  rungs,
  hours,
}: Readonly<{ rungs: RungHealth[]; hours: number }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  return (
    <DataTable<RungHealth>
      label={t("aiHealth.title")}
      rows={rungs}
      rowKey={(row) => row.tier}
      columns={[
        {
          key: "tier",
          header: t("aiHealth.colTier"),
          render: (row) => row.tier,
        },
        {
          key: "state",
          header: t("aiHealth.colState"),
          render: (row) =>
            row.healthy ? (
              <Badge tone="success">{t("aiHealth.answering")}</Badge>
            ) : (
              <Badge tone="danger">{t("aiHealth.notAnswering")}</Badge>
            ),
        },
        {
          key: "calls",
          header: t("aiHealth.colCalls", {
            hours: formatNumber(hours, locale),
          }),
          render: (row) =>
            // Both numbers, because "12 calls" beside a red badge leaves a
            // reader working out how many of them failed.
            t("aiHealth.callCounts", {
              calls: formatNumber(row.calls, locale),
              failures: formatNumber(row.failures, locale),
            }),
        },
        {
          key: "latency",
          header: t("aiHealth.colLatency"),
          render: (row) =>
            // The median tells a slow lane from a dead one, which is the
            // distinction an operator acts on differently.
            t("aiHealth.ms", {
              ms: formatNumber(row.median_latency_ms, locale),
            }),
        },
        {
          key: "last",
          header: t("aiHealth.colLast"),
          render: (row) => <LastCell row={row} zone={zone} />,
        },
      ]}
    />
  );
}

// When this rung last answered, and what it said if it failed.
//
// The sentinel is the operator's first clue and the reason this column is not
// just a timestamp: a budget refusal and an unreachable model are both "not
// answering" and want completely different fixes.
function LastCell({ row, zone }: Readonly<{ row: RungHealth; zone: string }>) {
  const { locale } = useLocale();
  return (
    <span className="cell-stack">
      {row.last_call_at ? (
        <span>{formatDateTime(row.last_call_at, locale, zone)}</span>
      ) : (
        <span className="t-meta">—</span>
      )}
      {row.last_sentinel ? (
        <span className="t-meta t-mono">{row.last_sentinel}</span>
      ) : null}
    </span>
  );
}
