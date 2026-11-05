// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ProviderHealthNotice } from "../../components/ai-provider-health-notice";
import { EmptyState } from "../../design-system/atoms";
import { PanelBody, PanelRow } from "../../design-system/panel";
import { useT } from "../../i18n";
import { useProviderHealth } from "../../lib/ai-provider-health";
import { providerName } from "../../lib/ai-provider-names";
import { useCan } from "../../lib/capability";
import { HealthCard } from "./healthcard";

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
    >
      {({ providers, configured }) => {
        // The worker can report a provider this process has no binding for.
        if (providers.length === 0) {
          return (
            <PanelBody>
              <EmptyState>
                {t(
                  (configured ?? []).length === 0
                    ? "providerHealth.noneConfigured"
                    : "health.code.providers_unobserved",
                )}
              </EmptyState>
            </PanelBody>
          );
        }
        return providers.map((entry) => (
          <PanelRow record key={entry.provider}>
            <p className="t-name">{providerName(entry.provider, t)}</p>
            <ProviderHealthNotice entry={entry} />
          </PanelRow>
        ));
      }}
    </HealthCard>
  );
}
