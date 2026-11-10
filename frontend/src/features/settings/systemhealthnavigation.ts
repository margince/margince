// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { routeHash } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { hashWithParams } from "../../lib/urlstate";
import { settingsSectionKeys } from "./settingssections";

export function healthSectionKey(section: string | undefined, t: Translator) {
  return settingsSectionKeys("system-health").find(
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

/** The address of System health's diagnostics, opened on one section. */
export function settingsHealthHref(section: MessageKey | undefined): string {
  return hashWithParams(
    routeHash({ ...settingsHref("system-health"), id2: "diagnostics" }),
    new Map([["section", section ?? ""]]),
  );
}
