// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Suspense } from "react";
import { Heading } from "../design-system/heading";
import { SettingsScreen, settingsAddress } from "../features/settings/settings";
import { useT } from "../i18n";
import { useRoute } from "../lib/router";

export function HealthSessionPage() {
  const route = useRoute();
  const t = useT();
  return (
    <main>
      <header className="wrap">
        <div className="settings-stack">
          <Heading size="xlarge" as="h1">
            {t("settings.tab.system-health")}
          </Heading>
        </div>
      </header>
      <Suspense fallback={null}>
        <SettingsScreen
          route={
            route.screen === "settings"
              ? route
              : { ...settingsAddress("system-health"), id2: route.id2 }
          }
        />
      </Suspense>
    </main>
  );
}
