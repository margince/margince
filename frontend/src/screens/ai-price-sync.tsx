// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import { EmptyState } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { formatElapsed, useNow } from "../format/now";
import { useLocale, useT } from "../i18n";
import { providerName } from "./ai-provider-names";
import { QueryGate, throwProblem } from "./common";
import {
  ProviderRefreshLine,
  RefreshModelPricesButton,
  useRefreshModelPrices,
} from "./rate-catalogue-refresh";
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
      const { data, error } = await api.PUT("/ai/price-sync", {
        body: { auto_sync: vars.autoSync },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data) => queryClient.setQueryData(["ai-price-sync"], data),
  });
}

/** Where model prices come from, whether they sync daily, and what the last sync did. */
export function ModelPricesCard() {
  const t = useT();
  const canRead = useCan("ai_model_rate", "read");
  // The switch is an update of the sheet's setting; a run also creates the rows
  // it adds, so Refresh now asks for both.
  const canManage = useCanWrite("ai_model_rate", "update");
  const canCreate = useCanWrite("ai_model_rate", "create");
  const canRefresh = canManage && canCreate;
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
      titleAction={
        canRefresh ? <RefreshModelPricesButton refresh={refresh} /> : undefined
      }
    >
      <QueryGate query={query} pendingLabel={t("aiPriceSync.title")}>
        {(state) => (
          <>
            <PanelBody className="form-stack">
              <SettingList>
                <SettingRow
                  label={t("aiPriceSync.autoSync.label")}
                  description={t("aiPriceSync.autoSync.help")}
                  control={(props) => (
                    <Switch
                      describedBy={props["aria-describedby"]}
                      testId="ai-price-sync-auto"
                      label={t("aiPriceSync.autoSync.label")}
                      labelHidden
                      reason={
                        canManage ? undefined : t("aiPriceSync.adminOnly")
                      }
                      checked={state.auto_sync}
                      pending={setAutoSync.isPending}
                      onChange={(next) =>
                        setAutoSync.mutate({ autoSync: next })
                      }
                    />
                  )}
                />
              </SettingList>
              <p className="t-caption">{t("aiPriceSync.sources")}</p>
              <LastSynced run={state.last_run} />
              {refresh.error ? <ErrorLine error={refresh.error} /> : null}
              {setAutoSync.error ? (
                <ErrorLine error={setAutoSync.error} />
              ) : null}
            </PanelBody>
            {state.last_run?.report.providers
              .filter((p) => !QUIET.has(p.outcome))
              .map((p) => (
                <PanelRow key={p.provider}>
                  <div className="ai-price-sync-line">
                    <span>{providerName(p.provider, t)}</span>
                    <ProviderRefreshLine line={p} />
                  </div>
                </PanelRow>
              ))}
          </>
        )}
      </QueryGate>
    </Panel>
  );
}

function LastSynced({ run }: Readonly<{ run: PriceSyncRun | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  const now = useNow(60_000);
  if (!run) {
    return <p className="t-caption">{t("aiPriceSync.never")}</p>;
  }
  const ago = formatElapsed(now - Date.parse(run.ran_at), t, locale);
  return <p className="t-caption">{t("aiPriceSync.lastSynced", { ago })}</p>;
}
