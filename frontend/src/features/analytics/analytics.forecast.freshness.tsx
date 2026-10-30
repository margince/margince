// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";

type Assurance = components["schemas"]["ForecastAssurance"];
type Freshness = components["schemas"]["AssuranceFreshness"];

// Whether the records moved past this reading. A server from before freshness
// sends none, and has nothing to say about it, so its reading stays as it was.
export function outOfDate(run: Assurance): boolean {
  return run.freshness?.state === "out_of_date";
}

// Whether the verdict may keep its tone: not when the records moved past it,
// and not when nobody knows whether they did.
export function verdictStands(run: Assurance): boolean {
  const state = run.freshness?.state;
  return state !== "out_of_date" && state !== "unknown";
}

// A reading the records moved past. The server decides it; this only says so,
// in the warning tone, so an out-of-date verdict never reads as a healthy one.
export function OutOfDateBadge() {
  const t = useT();
  return <Badge tone="warning">{t("review.outOfDate")}</Badge>;
}

// What changed since the check, and what to do about it. Every kind of change
// the server counted is named, a workspace-wide one beside the deals. Nothing
// for a current reading: "nothing changed" is the reading itself.
export function FreshnessLine({
  freshness,
}: Readonly<{ freshness: Freshness | undefined }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (freshness?.state === "unknown") {
    return <p className="sub">{t("review.freshnessUnknown")}</p>;
  }
  if (freshness?.state !== "out_of_date") {
    return null;
  }
  const changed = [
    freshness.changed_deals > 0 &&
      plural("review.outOfDateDeals", freshness.changed_deals, {
        count: formatNumber(freshness.changed_deals, locale),
      }),
    freshness.changed_signals > 0 &&
      plural("review.outOfDateSignals", freshness.changed_signals, {
        count: formatNumber(freshness.changed_signals, locale),
      }),
    freshness.workspace_changed && t("review.outOfDateWorkspace"),
  ].filter(Boolean);
  return (
    <p className="sub">
      {changed.join(" ")} {t("review.outOfDateRefresh")}
    </p>
  );
}
