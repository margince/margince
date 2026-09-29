// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { throwProblem } from "./common";
import "./rates.css";

type ProviderRefresh = components["schemas"]["AiModelRateProviderRefresh"];
type Outcome = ProviderRefresh["outcome"];

// The outcome is a wire string the server closes over five members; the tone
// says how the run went for THIS provider, not how good the news is. A vendor
// that publishes no price list is `info`, a fact rather than a fault.
const OUTCOME_TONE = {
  updated: "success",
  unchanged: "default",
  not_available: "info",
  unreachable: "danger",
  not_bound: "default",
} as const satisfies Record<Outcome, "success" | "default" | "info" | "danger">;

/**
 * RefreshModelPrices re-prices the models this installation calls from the
 * provider's own catalogue and lists what happened per provider.
 *
 * It runs inline, so unlike the currency sheet's refresh the answer is here
 * rather than in the approvals inbox: a price the catalogue states is written
 * to the sheet, and the report says which providers had nothing to say.
 */
export function RefreshModelPrices() {
  const t = useT();
  const queryClient = useQueryClient();
  const refresh = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST("/ai-model-rates/refresh");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    // The lanes read their prices from the same sheet this just wrote.
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["ai-model-rates"] }),
  });
  return (
    <span className="rates-refresh">
      <Button
        variant="ghost"
        onClick={() => refresh.mutate()}
        // `pending`, not `disabled`: the reader who just pressed it keeps their
        // focus, and the repeat press is still blocked.
        pending={refresh.isPending}
      >
        {t("aiRates.refresh.button")}
      </Button>
      {refresh.data ? (
        <ul
          className="rates-refresh-report"
          aria-label={t("aiRates.refresh.report")}
        >
          {refresh.data.providers.map((p) => (
            <li key={p.provider}>
              <Badge tone={OUTCOME_TONE[p.outcome]}>
                {t(`aiRates.refresh.outcome.${p.outcome}` as const)}
              </Badge>
              <span>{p.provider}</span>
              <ProviderCounts provider={p} />
            </li>
          ))}
        </ul>
      ) : null}
      <ErrorLine error={refresh.error} inline />
    </span>
  );
}

// The counts a run produced, spoken only where there are some: a provider
// reporting `not_available` has nothing to count and says so with its badge.
function ProviderCounts({ provider }: Readonly<{ provider: ProviderRefresh }>) {
  const { locale } = useLocale();
  const plural = usePlural();
  return (
    <>
      {provider.updated > 0 ? (
        <span className="t-caption">
          {plural("aiRates.refresh.updatedCount", provider.updated, {
            count: formatNumber(provider.updated, locale),
          })}
        </span>
      ) : null}
      {provider.unchanged > 0 ? (
        <span className="t-caption">
          {plural("aiRates.refresh.unchangedCount", provider.unchanged, {
            count: formatNumber(provider.unchanged, locale),
          })}
        </span>
      ) : null}
    </>
  );
}
