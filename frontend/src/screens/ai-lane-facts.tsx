// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import { Badge } from "../design-system/atoms";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { useAiStatus } from "./ai-admin";
import { useAiHealth } from "./ai-health";

// The line under a Model tiers row: how many tasks lead with this tier, and
// whether it answered in the health window.
//
// Both are joins against reads with their own grants, and each half stays
// silent when its read is not this reader's — an absent count is not "unused",
// and an absent health line is not "healthy".

type Health = components["schemas"]["AiHealth"];
type Feature = components["schemas"]["AiFeatureRoute"];

export type LaneFactsSource = Readonly<{
  health: Health | undefined;
  features: readonly Feature[] | undefined;
}>;

export function useLaneFactsSource(): LaneFactsSource {
  const canDiagnose = useCan("ai_diagnostics", "read");
  const canBudget = useCan("ai_budget", "read");
  const health = useAiHealth(canDiagnose);
  const status = useAiStatus(canDiagnose && canBudget);
  return { health: health.data, features: status.data?.features };
}

export function TierFacts({
  tier,
  source,
}: Readonly<{ tier: string; source: LaneFactsSource }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const tasks = source.features?.filter((f) => f.leading_tier === tier).length;
  const rung = source.health?.rungs.find((r) => r.tier === tier);
  if (tasks === undefined && source.health === undefined) return null;
  return (
    <p className="t-sub ai-lane-facts">
      {tasks !== undefined &&
        plural("aiRouting.taskCount", tasks, {
          count: formatNumber(tasks, locale),
        })}
      {source.health && !rung && (
        <span>
          {t("aiHealth.noCalls", {
            hours: formatNumber(source.health.window_hours, locale),
          })}
        </span>
      )}
      {source.health && rung && (
        <>
          {rung.healthy ? (
            <Badge tone="success">{t("aiHealth.answering")}</Badge>
          ) : (
            <Badge tone="danger">{t("aiHealth.notAnswering")}</Badge>
          )}
          <span>
            {plural("aiHealth.callCounts", rung.calls, {
              count: formatNumber(rung.calls, locale),
              failures: formatNumber(rung.failures, locale),
            })}
          </span>
          <span>
            {t("aiRouting.median", {
              ms: formatNumber(rung.median_latency_ms, locale),
            })}
          </span>
        </>
      )}
    </p>
  );
}

// The embedder and the decision model are not rungs of the ladder, and the
// health read counts rungs only — so these rows say that, rather than printing
// nothing a reader could take for "no calls".
export function UntrackedFacts() {
  const t = useT();
  return <p className="t-sub ai-lane-facts">{t("aiRouting.untracked")}</p>;
}
