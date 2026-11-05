// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { holdsEveryWord, searchWords } from "../../format/collate";
import { unwrap } from "../../lib/problem";

export type ConfigurationEntry = components["schemas"]["ConfigurationEntry"];

/** The two names an operator holds for a setting: its key and its variable. */
export type SettingName = Pick<ConfigurationEntry, "key" | "env">;

/** The address dial the Configuration page's filter lives in. */
export const CONFIGURATION_FILTER_PARAM = "q";

export const configurationQueryKey = ["installation-configuration"] as const;

/**
 * The installation's settings, as the Configuration page lists them.
 *
 * The page and settings search read this one query, so a reader who picks a
 * search hit lands on the rows that search matched. `enabled` is false for a
 * reader who may not open the page: the endpoint refuses them.
 */
export function useConfiguration(enabled = true) {
  return useQuery({
    queryKey: configurationQueryKey,
    queryFn: async () => unwrap(await api.GET("/installation/configuration")),
    enabled,
  });
}

/** Whether the setting's key or variable holds `filter`, as the page filters. */
export function matchesConfigurationFilter(
  setting: SettingName,
  filter: string,
): boolean {
  return holdsEveryWord(searchWords(filter), [setting.key, setting.env]);
}
