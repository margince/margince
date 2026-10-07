// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { DECIDE_RUNG } from "./ai-decision-labels";

type Health = components["schemas"]["AiHealth"];

/** The rung a task starts on: the decision model's, or its leading tier's. */
export function laneRung(health: Health, tier: string, decisionFirst: boolean) {
  return health.rungs.find(
    (r) => r.tier === (decisionFirst ? DECIDE_RUNG : tier),
  );
}

/**
 * How the lane a task starts on is answering, as the pill the providers list
 * uses for its own state. A decision-first task starts on the decision model,
 * so that is the lane it reports; every other task reports its leading tier.
 *
 * Silent when the reader has no health read, since an absent pill is not
 * "healthy", and grey, not green, for a lane that took no calls in the window.
 */
export function TaskState({
  health,
  tier,
  decisionFirst,
}: Readonly<{
  health: Health | undefined;
  tier: string;
  decisionFirst: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (!health) return null;
  const rung = laneRung(health, tier, decisionFirst);
  if (!rung) {
    return (
      <Badge>
        {t("aiHealth.noCalls", {
          hours: formatNumber(health.window_hours, locale),
        })}
      </Badge>
    );
  }
  return rung.healthy ? (
    <Badge tone="success">{t("aiHealth.answering")}</Badge>
  ) : (
    <Badge tone="danger">{t("aiHealth.notAnswering")}</Badge>
  );
}
