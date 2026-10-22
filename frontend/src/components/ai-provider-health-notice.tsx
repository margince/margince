// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { formatRelativeTime } from "../format/relativetime";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

export type ProviderHealthEntry =
  components["schemas"]["AiProviderHealthEntry"];
type Health = ProviderHealthEntry["health"];

// Keyed on the union so a health value added to the contract is a compile
// error here rather than a blank badge.
const COPY = {
  degraded: {
    label: "aiProviderHealth.label.degraded",
    reason: "aiProviderHealth.reason.degraded",
    fix: "aiProviderHealth.fix.degraded",
    tone: "warning",
  },
  down: {
    label: "aiProviderHealth.label.down",
    reason: "aiProviderHealth.reason.down",
    fix: "aiProviderHealth.fix.down",
    tone: "danger",
  },
  out_of_credit: {
    label: "aiProviderHealth.label.outOfCredit",
    reason: "aiProviderHealth.reason.outOfCredit",
    fix: "aiProviderHealth.fix.outOfCredit",
    tone: "danger",
  },
  unauthorized: {
    label: "aiProviderHealth.label.unauthorized",
    reason: "aiProviderHealth.reason.unauthorized",
    fix: "aiProviderHealth.fix.unauthorized",
    tone: "danger",
  },
} as const satisfies Record<
  Health,
  {
    label: MessageKey;
    reason: MessageKey;
    fix: MessageKey;
    tone: "warning" | "danger";
  }
>;

/** The health badge alone, for a row that has its own room for the rest. */
export function ProviderHealthBadge({ health }: Readonly<{ health: Health }>) {
  const t = useT();
  const copy = COPY[health];
  return <Badge tone={copy.tone}>{t(copy.label)}</Badge>;
}

/** Why a provider is not answering, since when, and who can fix it. */
export function ProviderHealthNotice({
  entry,
  now,
}: Readonly<{ entry: ProviderHealthEntry; now?: Date }>) {
  const t = useT();
  const { locale } = useLocale();
  const copy = COPY[entry.health];
  const at = now ?? new Date();
  const retry = entry.retry_after;
  let nextCheck: string | null = null;
  if (retry) {
    nextCheck =
      new Date(retry) > at
        ? t("aiProviderHealth.nextCheck", {
            when: formatRelativeTime(retry, locale, at),
          })
        : t("aiProviderHealth.nextCheckDue");
  }
  return (
    <div
      className="ai-provider-health"
      data-testid={`ai-provider-health-${entry.provider}`}
    >
      <ProviderHealthBadge health={entry.health} />
      <p className="t-caption">
        {t(copy.reason)}{" "}
        {t("aiProviderHealth.since", {
          when: formatRelativeTime(entry.since, locale, at),
        })}
        {nextCheck && ` · ${nextCheck}`}
      </p>
      <p className="t-caption">{t(copy.fix)}</p>
    </div>
  );
}
