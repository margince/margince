// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { DateInput, isISODate } from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { SettingRow } from "../../design-system/settingrow";
import { formatDate } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";

type CaptureConnection = components["schemas"]["CaptureConnection"];

/** The date this mailbox's automatic commitment suggestions start from. */
export function HistoryBaselineRow({
  conn,
}: Readonly<{ conn: CaptureConnection }>) {
  const t = useT();
  const { locale } = useLocale();
  const save = useSetHistoryBaseline(conn.provider);
  const baseline = conn.history_baseline;
  const [draft, setDraft] = useState(baseline?.date ?? "");
  const changed = isISODate(draft) && draft !== baseline?.date;
  const help = t("connectors.historyBaseline.help");
  return (
    <SettingRow
      testId={`connector-${conn.provider}-history-baseline`}
      label={t("connectors.historyBaseline.label")}
      description={
        baseline
          ? `${t("connectors.historyBaseline.current", {
              date: formatDate(baseline.date, locale, baseline.timezone),
              zone: baseline.timezone,
            })} ${help}`
          : help
      }
      layout="stack"
      control={(controlProps) => (
        <div className="form-stack">
          <div className="connector-baseline">
            <DateInput
              {...controlProps}
              value={isISODate(draft) ? draft : ""}
              disabled={save.isPending}
              onChange={(event) => setDraft(event.target.value)}
            />
            <Button
              disabled={!changed}
              pending={save.isPending}
              onClick={() =>
                save.mutate({ date: draft, timezone: viewerZone() })
              }
            >
              {t("connectors.historyBaseline.save")}
            </Button>
            {baseline && (
              <Button
                variant="ghost"
                disabled={save.isPending}
                onClick={() => {
                  setDraft("");
                  save.mutate({ date: null });
                }}
              >
                {t("connectors.historyBaseline.clear")}
              </Button>
            )}
          </div>
          {save.error && (
            <ErrorLine inline>{problemMessageOf(save.error, t)}</ErrorLine>
          )}
        </div>
      )}
    />
  );
}

function useSetHistoryBaseline(provider: CaptureConnection["provider"]) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { date: string | null; timezone?: string }) => {
      unwrap(
        await api.PUT("/connectors/{provider}/history-baseline", {
          params: { path: { provider } },
          body: { date: vars.date, timezone: vars.timezone },
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["connectors"] });
    },
  });
}
