// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCan } from "../app/capability";
import { EmptyState } from "../design-system/atoms";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useT } from "../i18n";
import { useProviderHealth } from "./ai-provider-health";
import { ProviderHealthNotice } from "./ai-provider-health-notice";
import { providerName } from "./ai-provider-names";
import { HealthCard } from "./healthcard";
import "./ai-settings.css";

// Settings → System health: every AI provider that is not answering. The
// Providers list shows the same entries per row; both read useProviderHealth.
export function ProviderHealthCard() {
  const t = useT();
  // `ai_diagnostics:read`, the grant /ai/health and so this endpoint ask for.
  const canSee = useCan("ai_diagnostics", "read");
  const query = useProviderHealth(canSee);
  return (
    <HealthCard
      title={t("settings.providerHealth")}
      sub={t("settings.providerHealthSub")}
      withheld={t("providerHealth.adminOnly")}
      canSee={canSee}
      query={query}
      footer={() => null}
    >
      {({ providers }) =>
        providers.length === 0 ? (
          // A finding, not an absence: the empty list is the endpoint's way of
          // saying every provider answered.
          <EmptyState>{t("providerHealth.healthy")}</EmptyState>
        ) : (
          <SettingList>
            {providers.map((entry) => (
              <SettingRow
                key={entry.provider}
                label={providerName(entry.provider, t)}
                layout="stack"
                control={<ProviderHealthNotice entry={entry} />}
              />
            ))}
          </SettingList>
        )
      }
    </HealthCard>
  );
}
