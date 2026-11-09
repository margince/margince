// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";
import type { SettingsPageId } from "../../lib/settingscatalog";
import { settingsSectionTab } from "./settingssections";

// Words that name one TAB rather than its page: the old names of pages that
// became tabs, and nouns only that tab answers. A hit through one opens the tab.
const TAB_ALIASES: Partial<
  Record<SettingsPageId, Readonly<Record<string, readonly string[]>>>
> = {
  connections: {
    activity: [
      "capture activity",
      "capture log",
      "held",
      "judgement",
      "why",
      "trace",
    ],
  },
  deals: {
    sources: ["acquisition"],
    loss: ["lost reasons"],
    reviews: ["review questions"],
  },
  records: { roles: ["record roles"] },
};

/** The tab aliases of `page` this reader may open, as `[tab, word]` pairs. */
export function tabAliases(
  page: SettingsPageId,
  opensTab?: (page: SettingsPageId, tab: string) => boolean,
): readonly (readonly [string, string])[] {
  return Object.entries(TAB_ALIASES[page] ?? {})
    .filter(([tab]) => opensTab?.(page, tab) ?? true)
    .flatMap(([tab, words]) => words.map((word) => [tab, word] as const));
}

// The tab a hit opens: the one whose word matched, else the one drawing the
// matched setting.
export function matchedTab(
  page: SettingsPageId,
  tabWord: readonly [string, string] | undefined,
  sectionKey: MessageKey | undefined,
): string | undefined {
  if (tabWord !== undefined) return tabWord[0];
  return sectionKey === undefined
    ? undefined
    : settingsSectionTab(page, sectionKey);
}
