// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { PanelBody } from "../design-system/panel";
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
  not_listed: "warning",
  unreachable: "danger",
  not_bound: "default",
} as const satisfies Record<
  Outcome,
  "success" | "default" | "info" | "warning" | "danger"
>;

/**
 * The refresh mutation, held by the Providers card so its button sits in the
 * header and each vendor's answer reaches that vendor's sheet.
 *
 * It runs inline, so unlike the currency sheet's refresh the answer is here
 * rather than in the approvals inbox: a price the catalogue states is written
 * to the sheet, and the report says which providers had nothing to say.
 */
export function useRefreshModelPrices() {
  const queryClient = useQueryClient();
  return useMutation({
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
}

export type ModelPriceRefresh = ReturnType<typeof useRefreshModelPrices>;

export function RefreshModelPricesButton({
  refresh,
}: Readonly<{ refresh: ModelPriceRefresh }>) {
  const t = useT();
  return (
    <Button
      variant="ghost"
      onClick={() => refresh.mutate()}
      // `pending`, not `disabled`: the reader who just pressed it keeps their
      // focus, and the repeat press is still blocked.
      pending={refresh.isPending}
    >
      {t("aiRates.refresh.button")}
    </Button>
  );
}

/**
 * What the last refresh did overall, on the list where the button was pressed:
 * a refusal, or how many prices moved and which vendors could not be reached.
 * The per-vendor answer stays in that vendor's sheet.
 */
export function RefreshSummary({
  refresh,
}: Readonly<{ refresh: ModelPriceRefresh }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  if (refresh.error) {
    return (
      <PanelBody>
        <ErrorLine error={refresh.error} />
      </PanelBody>
    );
  }
  if (!refresh.data) {
    return null;
  }
  const updated = refresh.data.providers.reduce((n, p) => n + p.updated, 0);
  const unreachable = refresh.data.providers
    .filter((p) => p.outcome === "unreachable")
    .map((p) => p.provider);
  return (
    <PanelBody>
      <p className="t-caption" role="status">
        {plural("aiRates.refresh.updatedCount", updated, {
          count: formatNumber(updated, locale),
        })}
        {unreachable.length > 0
          ? ` · ${t("aiRates.refresh.outcome.unreachable")}: ${unreachable.join(", ")}`
          : ""}
      </p>
    </PanelBody>
  );
}

/** What the last refresh did for ONE vendor, or nothing before there was one. */
export function ProviderRefreshLine({
  refresh,
  provider,
}: Readonly<{ refresh: ModelPriceRefresh; provider: string }>) {
  const t = useT();
  const line = refresh.data?.providers.find((p) => p.provider === provider);
  if (!line) {
    return <ErrorLine error={refresh.error} inline />;
  }
  return (
    <span className="rates-refresh-detail" role="status">
      <Badge tone={OUTCOME_TONE[line.outcome]}>
        {t(`aiRates.refresh.outcome.${line.outcome}` as const)}
      </Badge>
      <ProviderCounts provider={line} />
      {line.unlisted.length > 0 ? (
        <span className="t-caption">
          {t("aiRates.refresh.unlisted", { ids: line.unlisted.join(", ") })}
        </span>
      ) : null}
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
