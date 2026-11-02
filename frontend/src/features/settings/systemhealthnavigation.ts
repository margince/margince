// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { Route } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import type { SettingsHit } from "./settingssearch";
import { SETTINGS_SECTIONS } from "./settingssections";

export function settingsSearchRoute(hit: SettingsHit, t: Translator): Route {
  const route = settingsHref(hit.page.id);
  if (hit.page.id !== "system-health" || !hit.section) return route;
  const key = SETTINGS_SECTIONS["system-health"].find(
    (key) => t(key) === hit.section,
  );
  return {
    ...route,
    id2: key?.startsWith("operations.") ? "configuration" : "diagnostics",
  };
}

export function healthSectionKey(section: string | undefined, t: Translator) {
  return SETTINGS_SECTIONS["system-health"].find(
    (key) => key === section || t(key) === section,
  );
}

const healthSections: Readonly<Record<string, MessageKey>> = {
  jobs: "settings.jobs",
  capture: "settings.captureHealth",
  intake: "settings.extIngest",
  recovery: "settings.recoveryHealth",
  automatic: "settings.automaticWork",
  providers: "settings.providerHealth",
  updates: "dataTasks.title",
  search: "embedreindex.title",
};

export function healthSectionForSource(source: string): MessageKey | undefined {
  return healthSections[source];
}

export function healthSourceForSection(
  section: string | undefined,
): string | undefined {
  if (section === "health.history") return "jobs";
  if (section?.startsWith("embedreindex.")) return "search";
  if (section?.startsWith("recoveryHealth.")) return "recovery";
  return Object.entries(healthSections).find(([, key]) => key === section)?.[0];
}
