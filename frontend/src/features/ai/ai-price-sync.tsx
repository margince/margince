// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, EmptyState } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { formatElapsed, useNow } from "../../format/now";
import { lockReasonOr } from "../../format/settinglocks";
import { useLocale, useT } from "../../i18n";
import { providerName } from "../../lib/ai-provider-names";
import { useCan, useCanUpsert, useCanWrite } from "../../lib/capability";
import { throwProblem, unwrap } from "../../lib/problem";
import {
  ProviderRefreshLine,
  RefreshModelPricesButton,
  RefreshSummary,
  useRefreshModelPrices,
} from "../settings/rate-catalogue-refresh";
import { ModelPriceDialog } from "../settings/rate-manual";
import { ModelPriceSheet } from "../settings/rates";
import "./ai-settings.css";

type PriceSyncRun = components["schemas"]["AiPriceSyncRun"];
type ProviderRefresh = components["schemas"]["AiModelRateProviderRefresh"];

// A vendor whose line says nothing a reader can act on stays off the card.
const QUIET = new Set<ProviderRefresh["outcome"]>([
  "not_available",
  "not_bound",
  "not_configured",
]);

export function usePriceSync(enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: ["ai-price-sync"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/ai/price-sync");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

function useSetAutoSync() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { autoSync: boolean }) => {
      return unwrap(
        await api.PUT("/ai/price-sync", {
          body: { auto_sync: vars.autoSync },
        }),
      );
    },
    onSuccess: (data) => queryClient.setQueryData(["ai-price-sync"], data),
  });
}

/**
 * Model prices: whether they sync daily, what the last sync did, and the
 * prices in force per provider.
 */
export function ModelPricesCard() {
  const t = useT();
  const canRead = useCan("ai_model_rate", "read");
  // The switch is an update of the sheet's setting; a run also creates the rows
  // it adds, so Refresh asks for both.
  const canManage = useCanWrite("ai_model_rate", "update");
  const canCreate = useCanWrite("ai_model_rate", "create");
  const canRefresh = canManage && canCreate;
  // One endpoint, insert or replace, the verb resolved inside the transaction.
  const canSetPrice = useCanUpsert("ai_model_rate");
  const [adding, setAdding] = useState(false);
  const query = usePriceSync(canRead);
  const refresh = useRefreshModelPrices();
  const setAutoSync = useSetAutoSync();
  if (!canRead) {
    return (
      <Panel title={t("aiPriceSync.title")}>
        <PanelBody>
          <EmptyState>{t("aiPriceSync.withheld")}</EmptyState>
        </PanelBody>
      </Panel>
    );
  }
  return (
    <Panel
      title={t("aiPriceSync.title")}
      actions={
        query.data !== undefined && (canRefresh || canSetPrice) ? (
          <>
            {canRefresh ? <RefreshModelPricesButton refresh={refresh} /> : null}
            {canSetPrice ? (
              <Button onClick={() => setAdding(true)}>
                {t("settings.rates.modelAdd")}
              </Button>
            ) : null}
          </>
        ) : undefined
      }
    >
      <QueryGate query={query} pendingLabel={t("aiPriceSync.title")}>
        {(state) => (
          <>
            <SettingList bleed="settings">
              <SettingRow
                label={t("aiPriceSync.autoSync.label")}
                description={t("aiPriceSync.autoSync.help")}
                control={(props) => (
                  <Switch
                    describedBy={props["aria-describedby"]}
                    testId="ai-price-sync-auto"
                    label={t("aiPriceSync.autoSync.label")}
                    labelHidden
                    reason={lockReasonOr(
                      state.locks,
                      "ai.price_sync",
                      t,
                      canManage ? undefined : t("aiPriceSync.adminOnly"),
                    )}
                    checked={state.auto_sync}
                    pending={setAutoSync.isPending}
                    onChange={(next) => setAutoSync.mutate({ autoSync: next })}
                  />
                )}
              />
              <SettingRow
                label={t("aiPriceSync.lastSyncedLabel")}
                description={t("aiPriceSync.sources")}
                value={<LastSynced run={state.last_run} />}
                control={null}
              />
            </SettingList>
            <RefreshSummary refresh={refresh} />
            {setAutoSync.error ? (
              <PanelBody>
                <ErrorLine error={setAutoSync.error} />
              </PanelBody>
            ) : null}
            {state.last_run?.report.providers
              .filter((p) => !QUIET.has(p.outcome))
              .map((p) => (
                <PanelRow record key={p.provider}>
                  <div className="ai-price-sync-line">
                    <span>{providerName(p.provider, t)}</span>
                    <ProviderRefreshLine line={p} />
                  </div>
                </PanelRow>
              ))}
          </>
        )}
      </QueryGate>
      <ModelPriceSheet />
      {adding ? <ModelPriceDialog onClose={() => setAdding(false)} /> : null}
    </Panel>
  );
}

function LastSynced({ run }: Readonly<{ run: PriceSyncRun | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  const now = useNow(60_000);
  if (!run) {
    return t("aiPriceSync.never");
  }
  return formatElapsed(now - Date.parse(run.ran_at), t, locale);
}
