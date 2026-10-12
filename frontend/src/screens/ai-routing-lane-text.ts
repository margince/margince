// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatUsdPerMTok } from "../format/format";
import type { Locale, Translator } from "../i18n";
import {
  inputOnlyLane,
  type ModelCatalogue,
  type ModelLane,
  unreadablePrice,
} from "./ai-models";
import { isOpenRouter } from "./ai-provider-links";
import type { Lane } from "./ai-routing-lane";

// How a broker tier orders its hosts: its own sort, the shipped default, or the
// broker's own when it opted out. Nothing for a binding no broker serves.
export function servingSort(
  binding: Lane["binding"],
  t: Translator,
): string | undefined {
  if (
    binding?.provider !== "openai_compatible" ||
    !isOpenRouter(binding.base_url ?? "")
  ) {
    return undefined;
  }
  const routing = binding.routing;
  if (!routing) return "throughput";
  const sort = routing.provider?.sort;
  if (!sort) return t("aiFigures.line.brokerOwn");
  if (typeof sort === "string") return sort;
  return sort.partition === "none"
    ? t("aiFigures.line.sortAcross", { by: sort.by })
    : sort.by;
}

// Not a key built from the tier name: a composed key compiles as any string and
// ships a typo. A tier nobody glossed renders with no gloss.
export function laneGloss(name: string, t: Translator): string | null {
  switch (name) {
    case "local_small":
      return t("aiRouting.lane.local_small");
    case "cheap_cloud":
      return t("aiRouting.lane.cheap_cloud");
    case "premium":
      return t("aiRouting.lane.premium");
    case "frontier":
      return t("aiRouting.lane.frontier");
    case "local_large":
      return t("aiRouting.lane.local_large");
    case "embeddings":
      return t("aiRouting.lane.embeddings");
    case "decisions":
      return t("aiRouting.lane.decisions");
    default:
      return null;
  }
}

// Empty where the sheet cannot say: a printed zero would claim a price nobody
// set, and the editor's rate plate spells out a missing one.
export function priceLabel(
  catalogue: ModelCatalogue,
  provider: string,
  model: string,
  lane: ModelLane,
  locale: Locale,
  t: Translator,
): string {
  const rate = (catalogue ?? []).find(
    (r) => r.provider === provider && r.model_id === model && r.lane === lane,
  );
  if (!rate) {
    return "";
  }
  // Not the formatter: NaN in `Intl.NumberFormat` throws a RangeError during
  // render, and takes the whole settings page with it.
  if (unreadablePrice(rate.input_per_mtok)) {
    return "";
  }
  const input = formatUsdPerMTok(rate.input_per_mtok, locale);
  if (inputOnlyLane(lane)) {
    return t("aiAdmin.inputRate", { input });
  }
  if (unreadablePrice(rate.output_per_mtok)) {
    return "";
  }
  return t("aiAdmin.rates", {
    input,
    output: formatUsdPerMTok(rate.output_per_mtok, locale),
  });
}
