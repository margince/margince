// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import type { components } from "../api/schema";
import { Callout } from "../design-system/callout";
import { NumberSettingRow } from "../design-system/numbersetting";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { SettingList } from "../design-system/settingrow";
import { formatDateTime, formatNumber } from "../format/format";
import { formatInterval } from "../format/interval";
import { UTC_ZONE } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { useCan, useCanWrite } from "../lib/capability";
import { problemMessageOf } from "../lib/problem";
import { useInstallationSettings } from "../lib/uploadlimit";
import { useResetLine } from "./agentusage";
import { useUpdateInstallationSettings } from "./installation-settings";

type Allowances = components["schemas"]["AgentAllowanceSettings"];
type Editable =
  | "agent_reads_per_window"
  | "agent_writes_per_window"
  | "agent_egress_per_window"
  | "agent_calls_per_window"
  | "agent_window_hours";

// The bounds the API refuses past, mirrored so a value out of range is refused
// in the box. Keyed by the wire property, which is how
// backend/gates/settingbounds_test.go holds each to the contract.
const AGENT_ALLOWANCE_BOUNDS = {
  agent_reads_per_window: { min: 100, max: 20_000 },
  agent_writes_per_window: { min: 10, max: 5_000 },
  agent_egress_per_window: { min: 1, max: 200 },
  agent_calls_per_window: { min: 50, max: 20_000 },
  agent_window_hours: { min: 1, max: 168 },
} as const satisfies Record<
  Editable & keyof Allowances,
  { min: number; max: number }
>;

const ROWS = [
  { property: "agent_reads_per_window", copy: "reads" },
  { property: "agent_writes_per_window", copy: "writes" },
  { property: "agent_egress_per_window", copy: "egress" },
  { property: "agent_calls_per_window", copy: "calls" },
  { property: "agent_window_hours", copy: "window" },
] as const satisfies readonly { property: Editable; copy: string }[];

// What one passport may do in each window. Every value commits on its own, as
// the other operating values do; a count binds within a minute, a window
// length at the next reset.
export function AgentAllowancesCard() {
  const t = useT();
  const { locale } = useLocale();
  const canRead = useCan("installation_settings", "read");
  const canManage = useCanWrite("installation_settings", "update");
  const query = useInstallationSettings(canRead);
  const queryClient = useQueryClient();
  // A new allowance is the limit every usage bar is drawn against.
  const update = useUpdateInstallationSettings(() => {
    for (const key of [["passports"], ["agent-usage"]]) {
      void queryClient.invalidateQueries({ queryKey: key });
    }
  });
  const resetLine = useResetLine();
  if (!canRead) {
    return null;
  }
  const title = t("agentAllowances.title");
  return (
    <Panel title={title}>
      <PanelBody className="form-stack">
        <PanelIntro>{t("agentAllowances.sub")}</PanelIntro>
        {!canManage && <PanelIntro>{t("operations.adminOnly")}</PanelIntro>}
      </PanelBody>
      <QueryGate query={query} pendingLabel={title}>
        {(settings) => {
          const allowances = settings.agent_allowances;
          // A server older than this screen answers without them.
          if (allowances == null) {
            return null;
          }
          return (
            <>
              <SettingList bleed="settings">
                {ROWS.map((row) => (
                  <NumberSettingRow
                    key={row.property}
                    label={t(`agentAllowances.${row.copy}.label`)}
                    description={t(`agentAllowances.${row.copy}.help`)}
                    testId={`allowance-${row.property}`}
                    value={allowances[row.property]}
                    {...AGENT_ALLOWANCE_BOUNDS[row.property]}
                    refusal={t("operations.refusal")}
                    reading={
                      row.property === "agent_window_hours"
                        ? (hours) =>
                            t("operations.inWords", {
                              interval: formatInterval(hours * 3_600, locale),
                            })
                        : undefined
                    }
                    disabled={!canManage || update.isPending}
                    onCommit={(next) => update.mutate({ [row.property]: next })}
                  />
                ))}
              </SettingList>
              <PanelBody className="form-stack">
                <p className="t-caption">
                  {resetLine(allowances.agent_window_resets_at)}
                </p>
                <PendingWindow allowances={allowances} />
                <p className="t-caption">
                  {t("agentAllowances.burst", {
                    calls: formatNumber(
                      allowances.mcp_calls_per_minute,
                      locale,
                    ),
                    streams: formatNumber(
                      allowances.mcp_stream_opens_per_minute,
                      locale,
                    ),
                  })}
                </p>
              </PanelBody>
            </>
          );
        }}
      </QueryGate>
      {update.isError && (
        <PanelBody>
          <Callout
            tone="danger"
            kind="outcome"
            title={t("operations.updateFailed")}
          >
            {problemMessageOf(update.error, t)}
          </Callout>
        </PanelBody>
      )}
    </Panel>
  );
}

// A saved window length that has not taken over yet. Said out loud, because
// the field still shows the length in force and a reader would think the save
// did nothing.
function PendingWindow({ allowances }: Readonly<{ allowances: Allowances }>) {
  const t = useT();
  const { locale } = useLocale();
  const hours = allowances.agent_window_next_hours;
  const from = allowances.agent_window_next_starts_at;
  if (hours == null || from == null) {
    return null;
  }
  return (
    <Callout tone="info" title={t("agentAllowances.pending.title")}>
      {t("agentAllowances.pending.body", {
        interval: formatInterval(hours * 3_600, locale),
        date: formatDateTime(from, locale, UTC_ZONE),
      })}
    </Callout>
  );
}
